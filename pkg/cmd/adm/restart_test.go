package adm

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	clicontext "github.com/kubesaw/ksctl/pkg/context"
	. "github.com/kubesaw/ksctl/pkg/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierror "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/client-go/rest/fake"
	cgtesting "k8s.io/client-go/testing"
	cmdtesting "k8s.io/kubectl/pkg/cmd/testing"
	"k8s.io/kubectl/pkg/scheme"
)

func TestKubectlRolloutFunctionality(t *testing.T) {

	HostNamespacedName := types.NamespacedName{
		Namespace: "toolchain-host-operator",
		Name:      "host-operator-controller-manager",
	}
	RegNamespacedName := types.NamespacedName{
		Namespace: "toolchain-host-operator",
		Name:      "registration-service",
	}
	var rolloutGroupVersionEncoder = schema.GroupVersion{Group: "apps", Version: "v1"}
	hostDep := newDeployment(HostNamespacedName, 1)
	regDep := newDeployment(RegNamespacedName, 1)
	ns := scheme.Codecs.WithoutConversion()
	tf := cmdtesting.NewTestFactory().WithNamespace(HostNamespacedName.Namespace)
	tf.ClientConfigVal = cmdtesting.DefaultClientConfig()
	info, _ := runtime.SerializerInfoForMediaType(ns.SupportedMediaTypes(), runtime.ContentTypeJSON)
	encoder := ns.EncoderForVersion(info.Serializer, rolloutGroupVersionEncoder)
	tf.Client = &fake.RESTClient{
		GroupVersion:         rolloutGroupVersionEncoder,
		NegotiatedSerializer: ns,
		Client: fake.CreateHTTPClient(func(req *http.Request) (*http.Response, error) {
			body := io.NopCloser(bytes.NewReader([]byte(runtime.EncodeOrDie(encoder, hostDep))))
			return &http.Response{StatusCode: http.StatusOK, Header: cmdtesting.DefaultHeader(), Body: body}, nil
		}),
	}
	csCalls := 0
	tf.FakeDynamicClient.WatchReactionChain = nil
	tf.FakeDynamicClient.AddWatchReactor("*", func(action cgtesting.Action) (handled bool, ret watch.Interface, err error) {
		csCalls++
		fw := watch.NewFake()
		hostDep.Status = appsv1.DeploymentStatus{
			Replicas:            1,
			UpdatedReplicas:     1,
			ReadyReplicas:       1,
			AvailableReplicas:   1,
			UnavailableReplicas: 0,
			Conditions: []appsv1.DeploymentCondition{{
				Type: appsv1.DeploymentAvailable,
			}},
		}
		c, err := runtime.DefaultUnstructuredConverter.ToUnstructured(hostDep.DeepCopyObject())
		if err != nil {
			t.Errorf("unexpected err %s", err)
		}
		u := &unstructured.Unstructured{}
		u.SetUnstructuredContent(c)
		go fw.Add(u)
		return true, fw, nil
	})

	streams, _, buf, _ := genericiooptions.NewTestIOStreams()
	term := NewFakeTerminalWithResponse("Y")
	pod := newPod(test.NamespacedName(hostDep.Namespace, hostDep.Name))
	hostDep.Labels = map[string]string{"kubesaw-control-plane": "kubesaw-controller-manager"}
	regDep.Labels = map[string]string{"toolchain.dev.openshift.com/provider": "codeready-toolchain"}

	t.Run("Rollout Restart and Rollout Status works successfully", func(t *testing.T) {
		csCalls = 0
		newClient, fakeClient := NewFakeClients(t, hostDep, regDep, pod)
		ctx := clicontext.NewCommandContext(term, newClient)

		//when
		err := restartDeployments(ctx, fakeClient, HostNamespacedName.Namespace, func(ctx *clicontext.CommandContext, deployment appsv1.Deployment) error {
			return checkRolloutStatus(ctx, tf, streams, *hostDep)
		}, func(ctx *clicontext.CommandContext, deployment appsv1.Deployment) error {
			return restartNonOlmDeployments(ctx, deployment, tf, streams)
		})

		//then
		require.NoError(t, err)
		require.Contains(t, term.Output(), "Checking the status of the deleted pod's deployment")
		//checking the output from kubectl for rolloutstatus
		require.Contains(t, buf.String(), "deployment.apps/host-operator-controller-manager restarted\n")
		//checking the flow for non-operator deployments
		require.Contains(t, term.Output(), "Proceeding to restart the non-olm deployment")
		require.Contains(t, term.Output(), "Running the rollout restart command for non-Olm deployment")
		actual := &appsv1.Deployment{}
		AssertObjectHasContent(t, fakeClient, HostNamespacedName, actual, func() {
			require.NotNil(t, actual.Spec.Replicas)
			assert.Equal(t, int32(1), *actual.Spec.Replicas)
			require.NotNil(t, actual.Annotations["restartedAt"])
		})
		assert.Equal(t, 2, csCalls)
		require.Contains(t, term.Output(), "Checking the status of the rolled out deployment")
		require.Contains(t, term.Output(), "Running the Rollout status to check the status of the deployment")

	})

	t.Run("Error No OLM deployment", func(t *testing.T) {
		csCalls = 0
		newClient, fakeClient := NewFakeClients(t, regDep)
		ctx := clicontext.NewCommandContext(term, newClient)

		//when
		err := restartDeployments(ctx, fakeClient, HostNamespacedName.Namespace, func(ctx *clicontext.CommandContext, deployment appsv1.Deployment) error {
			return checkRolloutStatus(ctx, tf, streams, *hostDep)
		}, func(ctx *clicontext.CommandContext, deployment appsv1.Deployment) error {
			return restartNonOlmDeployments(ctx, deployment, tf, streams)
		})

		//then
		require.Error(t, err, "no operator based deployment found in namespace toolchain-host-operator , hence no restart happened")
		assert.Equal(t, 0, csCalls)

	})
	t.Run("No Non-OLM deployment", func(t *testing.T) {
		csCalls = 0
		newClient, fakeClient := NewFakeClients(t, hostDep, pod)
		ctx := clicontext.NewCommandContext(term, newClient)

		//when
		err := restartDeployments(ctx, fakeClient, HostNamespacedName.Namespace, func(ctx *clicontext.CommandContext, deployment appsv1.Deployment) error {
			return checkRolloutStatus(ctx, tf, streams, *hostDep)
		}, func(ctx *clicontext.CommandContext, deployment appsv1.Deployment) error {
			return restartNonOlmDeployments(ctx, deployment, tf, streams)
		})

		//then
		require.NoError(t, err)
		//checking the logic when only operator based deployment is there and no non-operator based
		require.Contains(t, term.Output(), "No Non-OLM deployment found in namespace toolchain-host-operator, hence no restart happened")
		assert.Equal(t, 1, csCalls)

	})

}
func TestRestartDeployment(t *testing.T) {
	//given
	SetFileConfig(t, Host(), Member())

	//OLM-deployments
	//host
	hostDeployment := newDeployment(test.NamespacedName("toolchain-host-operator", "host-operator-controller-manager"), 1)
	hostDeployment.Labels = map[string]string{"kubesaw-control-plane": "kubesaw-controller-manager"}
	hostPod := newPod(test.NamespacedName("toolchain-host-operator", "host-operator-controller-manager"))
	extraPod := newPod(test.NamespacedName("toolchain-host-operator", "extra"))

	//Non-OLM deployments
	//reg-svc
	regServDeployment := newDeployment(test.NamespacedName("toolchain-host-operator", "registration-service"), 1)
	regServDeployment.Labels = map[string]string{"toolchain.dev.openshift.com/provider": "codeready-toolchain"}

	actualPod := &corev1.Pod{}
	term := NewFakeTerminalWithResponse("Y")

	t.Run("restart deployment returns an error if no operator based deployment found", func(t *testing.T) {
		//given
		newClient, fakeClient := NewFakeClients(t, regServDeployment)
		ctx := clicontext.NewCommandContext(term, newClient)

		//when
		err := restartDeployments(ctx, fakeClient, "toolchain-host-operator",
			func(ctx *clicontext.CommandContext, deployment appsv1.Deployment) error {
				require.Equal(t, "host-operator-controller-manager", deployment.Name)
				return nil
			}, func(ctx *clicontext.CommandContext, deployment appsv1.Deployment) error {
				require.Equal(t, regServDeployment, deployment)
				return nil
			})

		//then
		require.Error(t, err, "no operator based deployment found in namespace toolchain-host-operator , it is required for the operator deployment to be running so the command can proceed with restarting the KubeSaw components")
	})

	t.Run("restart deployment works successfully with whole operator(operator, non operator)", func(t *testing.T) {
		//given
		newClient, fakeClient := NewFakeClients(t, hostDeployment, hostPod, regServDeployment, extraPod)
		ctx := clicontext.NewCommandContext(term, newClient)

		//when
		err := restartDeployments(ctx, fakeClient, "toolchain-host-operator",
			func(ctx *clicontext.CommandContext, deployment appsv1.Deployment) error {
				return nil
			}, func(ctx *clicontext.CommandContext, deployment appsv1.Deployment) error {
				return nil
			})

		//then
		require.NoError(t, err)
		//checking the flow for operator deployments
		require.Contains(t, term.Output(), "Fetching the current OLM and non-OLM deployments of the operator in toolchain-host-operator namespace")
		require.Contains(t, term.Output(), "Proceeding to delete the Pods of")
		require.Contains(t, term.Output(), "Deleting pod: host-operator-controller-manager")
		err = fakeClient.Get(ctx, test.NamespacedName("toolchain-host-operator", "host-operator-controller-manager"), actualPod)
		//pods are actually deleted
		require.True(t, apierror.IsNotFound(err))
		require.Contains(t, term.Output(), "Checking the status of the deleted pod's deployment")
		//checking the flow for non-operator deployments
		require.Contains(t, term.Output(), "Proceeding to restart the non-olm deployment")
		require.Contains(t, term.Output(), "Checking the status of the rolled out deployment")
	})

	t.Run("restart deployment works successfully when only operator based deployment", func(t *testing.T) {
		//given
		newClient, fakeClient := NewFakeClients(t, hostDeployment, hostPod)
		ctx := clicontext.NewCommandContext(term, newClient)

		//when
		err := restartDeployments(ctx, fakeClient, "toolchain-host-operator",
			func(ctx *clicontext.CommandContext, deployment appsv1.Deployment) error {
				return nil
			}, func(ctx *clicontext.CommandContext, deployment appsv1.Deployment) error {
				return nil
			})

		//then
		require.NoError(t, err)
		require.Contains(t, term.Output(), "No Non-OLM deployment found in namespace toolchain-host-operator, hence no restart happened")
	})

	t.Run("rollout restart returns an error", func(t *testing.T) {
		//given
		newClient, fakeClient := NewFakeClients(t, hostDeployment, regServDeployment, hostPod)
		ctx := clicontext.NewCommandContext(term, newClient)
		expectedErr := fmt.Errorf("Could not do rollout restart of the deployment")
		//when
		err := restartDeployments(ctx, fakeClient, "toolchain-host-operator",
			func(ctx *clicontext.CommandContext, deployment appsv1.Deployment) error {
				return nil
			}, func(ctx *clicontext.CommandContext, deployment appsv1.Deployment) error {
				return expectedErr
			})

		//then
		require.EqualError(t, err, expectedErr.Error())
	})

	t.Run("rollout status for the deleted pods(operator) works", func(t *testing.T) {
		//given
		newClient, fakeClient := NewFakeClients(t, hostDeployment)
		ctx := clicontext.NewCommandContext(term, newClient)

		//when
		err := restartDeployments(ctx, fakeClient, "toolchain-host-operator",
			func(ctx *clicontext.CommandContext, deployment appsv1.Deployment) error {
				return nil
			}, nil)

		//then
		require.NoError(t, err)
	})

	t.Run("error in rollout status of the deleted pods(operator)", func(t *testing.T) {
		//given
		newClient, fakeClient := NewFakeClients(t, hostDeployment)
		ctx := clicontext.NewCommandContext(term, newClient)
		expectedErr := fmt.Errorf("Could not check the status of the deployment")
		//when
		err := restartDeployments(ctx, fakeClient, "toolchain-host-operator",
			func(ctx *clicontext.CommandContext, deployment appsv1.Deployment) error {
				return expectedErr
			}, nil)

		//then
		require.EqualError(t, err, expectedErr.Error())
	})

}

func TestRestartAutoScalerDeployment(t *testing.T) {
	//given
	SetFileConfig(t, Host(), Member())

	//OLM-deployments
	//member
	memberDeployment := newDeployment(test.NamespacedName("toolchain-member-operator", "member-operator-controller-manager"), 1)
	memberDeployment.Labels = map[string]string{"kubesaw-control-plane": "kubesaw-controller-manager"}

	//Non-OLM deployments
	//autoscaler
	autoscalerDeployment := newDeployment(test.NamespacedName("toolchain-member-operator", "autoscaling-buffer"), 1)
	autoscalerDeployment.Labels = map[string]string{"toolchain.dev.openshift.com/provider": "codeready-toolchain"}

	term := NewFakeTerminalWithResponse("Y")

	t.Run("autoscalling deployment should not restart", func(t *testing.T) {
		//given
		newClient, fakeClient := NewFakeClients(t, memberDeployment, autoscalerDeployment)
		ctx := clicontext.NewCommandContext(term, newClient)
		//when
		err := restartDeployments(ctx, fakeClient, "toolchain-member-operator",
			func(ctx *clicontext.CommandContext, deployment appsv1.Deployment) error {
				return nil
			}, mockRolloutRestartInterceptor())

		//then
		require.NoError(t, err)
		require.Contains(t, term.Output(), "Found only autoscaling-buffer deployment in namespace toolchain-member-operator , which is not required to be restarted")
		require.NotContains(t, term.Output(), "Proceeding to restart the non-olm deployment")
	})
}

func TestRestart(t *testing.T) {
	//given
	SetFileConfig(t, Host(), Member())

	t.Run("No restart when users says NO in confirmaion of restart", func(t *testing.T) {
		term := NewFakeTerminalWithResponse("N")
		//given
		newClient, _ := NewFakeClients(t)
		ctx := clicontext.NewCommandContext(term, newClient)
		//when
		err := restart(ctx, "host", getConfigFlagsAndClient)

		//then
		require.NoError(t, err)
		require.NotContains(t, term.Output(), "Fetching the current OLM and non-OLM deployments of the operator in")

	})
}

func newDeployment(namespacedName types.NamespacedName, replicas int32) *appsv1.Deployment { //nolint:unparam
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespacedName.Namespace,
			Name:      namespacedName.Name,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"dummy-key": "controller"}},
		},
	}
}

func newPod(namespacedName types.NamespacedName) *corev1.Pod { //nolint:unparam
	return &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Pod",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespacedName.Namespace,
			Name:      namespacedName.Name,
			Labels:    map[string]string{"dummy-key": "controller"},
		},
		Spec: corev1.PodSpec{},
		Status: corev1.PodStatus{
			Phase: "Running",
		},
	}
}

func mockRolloutRestartInterceptor() func(ctx *clicontext.CommandContext, deployment appsv1.Deployment) error {
	return func(ctx *clicontext.CommandContext, deployment appsv1.Deployment) error {
		if deployment.Name == "autoscaling-buffer" {
			return fmt.Errorf("autoscalling deployment found")
		}
		return nil
	}
}

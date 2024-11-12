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
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/rest/fake"
	cgtesting "k8s.io/client-go/testing"
	cmdtesting "k8s.io/kubectl/pkg/cmd/testing"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/scheme"
)

func TestRestartDeployment(t *testing.T) {
	// given
	tests := map[string]struct {
		namespace      string
		name           string
		name1          string
		labelKey       string
		labelValue     string
		labelKey1      string
		labelValue1    string
		expectedMsg    string
		labelSelector  string
		expectedOutput string
		lsKey          string
		lsValue        string
	}{
		//operator and non-operator deployments
		"OperatorAndNonOperatorHostDeployment": {
			namespace:     "toolchain-host-operator",
			name:          "host-operator-controller-manager",
			name1:         "registration-service",
			labelKey:      "kubesaw-control-plane",
			labelValue:    "kubesaw-controller-manager",
			labelKey1:     "toolchain.dev.openshift.com/provider",
			labelValue1:   "codeready-toolchain",
			expectedMsg:   "deployment \"host-operator-controller-manager\" successfully rolled out\n",
			labelSelector: "kubesaw-control-plane=kubesaw-controller-manager",
			lsKey:         "host",
			lsValue:       "operator",
		},
		//operator and non-operator deployments, checking for autoscaler deployments,
		//it should be treated as no non-operator deployment available
		"OperatorAndNonOperatorWithAutoscalerDeployment": {
			namespace:     "toolchain-member-operator",
			name:          "member-operator-controller-manager",
			name1:         "autoscaling-buffer",
			labelKey:      "kubesaw-control-plane",
			labelValue:    "kubesaw-controller-manager",
			labelKey1:     "toolchain.dev.openshift.com/provider",
			labelValue1:   "codeready-toolchain",
			expectedMsg:   "deployment \"member-operator-controller-manager\" successfully rolled out\n",
			labelSelector: "kubesaw-control-plane=kubesaw-controller-manager",
			lsKey:         "host",
			lsValue:       "operator",
		},
		//only non-operator deployment
		"NonOperatorHostDeployment": {
			namespace:      "toolchain-host-operator",
			name:           "registration-service",
			labelKey:       "toolchain.dev.openshift.com/provider",
			labelValue:     "codeready-toolchain",
			expectedMsg:    "deployment \"registration-service\" successfully rolled out\n",
			labelSelector:  "toolchain.dev.openshift.com/provider=codeready-toolchain",
			expectedOutput: "deployment.apps/registration-service restarted\n",
		},
		//only operator deployment
		"OperatorHostDeployment": {
			namespace:     "toolchain-host-operator",
			name:          "host-operator-controller-manager",
			labelKey:      "kubesaw-control-plane",
			labelValue:    "kubesaw-controller-manager",
			expectedMsg:   "deployment \"host-operator-controller-manager\" successfully rolled out\n",
			labelSelector: "kubesaw-control-plane=kubesaw-controller-manager",
			lsKey:         "host",
			lsValue:       "operator",
		},
	}
	for k, tc := range tests {
		t.Run(k, func(t *testing.T) {
			//given
			namespacedName := types.NamespacedName{
				Namespace: tc.namespace,
				Name:      tc.name,
			}
			namespacedName1 := types.NamespacedName{
				Namespace: tc.namespace,
				Name:      tc.name1,
			}
			var rolloutGroupVersionEncoder = schema.GroupVersion{Group: "apps", Version: "v1"}
			deployment1 := newDeployment(namespacedName, 1)
			deployment2 := newDeployment(namespacedName1, 1)
			ns := scheme.Codecs.WithoutConversion()
			tf := cmdtesting.NewTestFactory().WithNamespace(namespacedName.Namespace)
			tf.ClientConfigVal = cmdtesting.DefaultClientConfig()
			info, _ := runtime.SerializerInfoForMediaType(ns.SupportedMediaTypes(), runtime.ContentTypeJSON)
			encoder := ns.EncoderForVersion(info.Serializer, rolloutGroupVersionEncoder)
			tf.Client = &fake.RESTClient{
				GroupVersion:         rolloutGroupVersionEncoder,
				NegotiatedSerializer: ns,
				Client: fake.CreateHTTPClient(func(req *http.Request) (*http.Response, error) {
					body := io.NopCloser(bytes.NewReader([]byte(runtime.EncodeOrDie(encoder, deployment1))))
					return &http.Response{StatusCode: http.StatusOK, Header: cmdtesting.DefaultHeader(), Body: body}, nil
				}),
			}
			csCalls := 0
			tf.FakeDynamicClient.WatchReactionChain = nil
			tf.FakeDynamicClient.AddWatchReactor("*", func(action cgtesting.Action) (handled bool, ret watch.Interface, err error) {
				csCalls++
				fw := watch.NewFake()
				deployment1.Status = appsv1.DeploymentStatus{
					Replicas:            1,
					UpdatedReplicas:     1,
					ReadyReplicas:       1,
					AvailableReplicas:   1,
					UnavailableReplicas: 0,
					Conditions: []appsv1.DeploymentCondition{{
						Type: appsv1.DeploymentAvailable,
					}},
				}
				c, err := runtime.DefaultUnstructuredConverter.ToUnstructured(deployment1.DeepCopyObject())
				if err != nil {
					t.Errorf("unexpected err %s", err)
				}
				u := &unstructured.Unstructured{}
				u.SetUnstructuredContent(c)
				go fw.Add(u)
				return true, fw, nil
			})

			streams, _, buf, _ := genericclioptions.NewTestIOStreams()
			term := NewFakeTerminalWithResponse("Y")
			pod := newPod(test.NamespacedName(namespacedName.Namespace, namespacedName.Name))
			deployment1.Labels = make(map[string]string)
			deployment1.Labels[tc.labelKey] = tc.labelValue
			deployment2.Labels = make(map[string]string)
			deployment2.Labels[tc.labelKey1] = tc.labelValue1
			newClient, fakeClient := NewFakeClients(t, deployment1, deployment2, pod)
			ctx := clicontext.NewCommandContext(term, newClient)

			//when
			err := restartDeployment(ctx, fakeClient, namespacedName.Namespace, tf, streams, checkRolloutStatus, restartNonOperatorDeployments)

			//then

			//checking the whole flow(starting with operator deployments & then to non operator deployments)
			if tc.labelValue == "kubesaw-controller-manager" && tc.labelValue1 == "codeready-toolchain" && tc.name1 != "autoscaling-buffer" {
				require.Contains(t, term.Output(), "Checking the status of the deleted pod's deployment")
				//checking the output from kubectl for rolloutstatus
				require.Contains(t, buf.String(), tc.expectedOutput)
				//checking the flow for non-operator deployments
				require.Contains(t, term.Output(), "Proceeding to restart the non-operator deployment")
				require.Contains(t, term.Output(), "Running the rollout restart command for non-Operator deployment")
				actual := &appsv1.Deployment{}
				AssertObjectHasContent(t, fakeClient, namespacedName, actual, func() {
					require.NotNil(t, actual.Spec.Replicas)
					assert.Equal(t, int32(1), *actual.Spec.Replicas)
					require.NotNil(t, actual.Annotations["restartedAt"])
				})
				assert.Equal(t, 2, csCalls)
				require.Contains(t, term.Output(), "Checking the status of the rolled out deployment")
				require.Contains(t, term.Output(), "Running the Rollout status to check the status of the deployment")
			} else if tc.labelValue == "codeready-toolchain" {
				//Checking the logic where no operator deployments are there
				require.Error(t, err, "no operator based deployment found in namespace toolchain-host-operator , hence no restart happened")
				assert.Equal(t, 0, csCalls)
			} else if tc.labelValue == "kubesaw-controller-manager" && tc.name1 != "autoscaling-buffer" {
				//checking the logic when only operator based deployment is there and no non-operator based
				require.Contains(t, term.Output(), "No Non-operator deployment found in namespace", tc.namespace, ", hence no restart happened")
				assert.Equal(t, 1, csCalls)
			} else if tc.name1 == "autoscaling-buffer" {
				require.Contains(t, term.Output(), "Found only autoscaling-buffer deployment in namespace toolchain-member-operator , which is not required to be restarted")
				assert.Equal(t, 1, csCalls)
			}

		})
	}
}

func TestOperator(t *testing.T) {
	//given
	testIOStreams := genericclioptions.NewTestIOStreamsDiscard()
	SetFileConfig(t, Host())
	toolchainCluster := NewToolchainCluster(ToolchainClusterName("host"))
	hostDeployment := newDeployment(test.NamespacedName("toolchain-host-operator", "host-operator-controller-manager"), 1)
	hostDeployment.Labels = make(map[string]string)
	hostDeployment.Labels["kubesaw-control-plane"] = "kubesaw-controller-manager"
	regServDeployment := newDeployment(test.NamespacedName("toolchain-host-operator", "registration-service"), 1)
	regServDeployment.Labels = make(map[string]string)
	regServDeployment.Labels["toolchain.dev.openshift.com/provider"] = "codeready-toolchain"
	hostPod := newPod(test.NamespacedName("toolchain-host-operator", "host-operator-controller-manager"))
	memberDeployment := newDeployment(test.NamespacedName("toolchain-member-operator", "member-operator-controller-manager"), 1)
	memberDeployment.Labels = make(map[string]string)
	memberDeployment.Labels["kubesaw-control-plane"] = "kubesaw-controller-manager"
	autoscalarDeployment := newDeployment(test.NamespacedName("toolchain-member-operator", "autoscaling-buffer"), 1)
	autoscalarDeployment.Labels = make(map[string]string)
	autoscalarDeployment.Labels["toolchain.dev.openshift.com/provider"] = "codeready-toolchain"
	actualPod := &corev1.Pod{}
	term := NewFakeTerminalWithResponse("Y")

	t.Run("restart deployment returns an error if no operator based deployment found", func(t *testing.T) {
		//given
		newClient, fakeClient := NewFakeClients(t, toolchainCluster, regServDeployment)
		ctx := clicontext.NewCommandContext(term, newClient)

		//when
		err := restartDeployment(ctx, fakeClient, "toolchain-host-operator", nil, testIOStreams,
			func(ctx *clicontext.CommandContext, f cmdutil.Factory, ioStreams genericclioptions.IOStreams, labelSelector string) error {
				require.Equal(t, testIOStreams, ioStreams)
				require.Nil(t, f)
				require.Equal(t, "toolchain.dev.openshift.com/provider=codeready-toolchain", labelSelector)
				return nil
			}, func(ctx *clicontext.CommandContext, deployment appsv1.Deployment, f cmdutil.Factory, ioStreams genericclioptions.IOStreams) error {
				require.Equal(t, regServDeployment, deployment)
				require.Equal(t, testIOStreams, ioStreams)
				require.Nil(t, f)
				return nil
			})

		//then
		require.Error(t, err, "no operator based deployment found in namespace toolchain-host-operator , hence no restart happened")
	})
	t.Run("restart deployment works successfully with whole operator(operator, non operator)", func(t *testing.T) {
		//given
		newClient, fakeClient := NewFakeClients(t, toolchainCluster, hostDeployment, hostPod, regServDeployment)
		ctx := clicontext.NewCommandContext(term, newClient)

		//when
		err := restartDeployment(ctx, fakeClient, "toolchain-host-operator", nil, testIOStreams,
			func(ctx *clicontext.CommandContext, f cmdutil.Factory, ioStreams genericclioptions.IOStreams, labelSelector string) error {
				require.Equal(t, testIOStreams, ioStreams)
				require.Nil(t, f)
				return nil
			}, func(ctx *clicontext.CommandContext, deployment appsv1.Deployment, f cmdutil.Factory, ioStreams genericclioptions.IOStreams) error {
				require.Equal(t, testIOStreams, ioStreams)
				require.Nil(t, f)
				return nil
			})

		//then
		require.NoError(t, err)
		//checking the flow for operator deployments
		require.Contains(t, term.Output(), "Fetching the current Operator and non-Operator deployments of the operator in")
		require.Contains(t, term.Output(), "Proceeding to delete the Pods of")
		require.Contains(t, term.Output(), "Listing the pods to be deleted")
		require.Contains(t, term.Output(), "Starting to delete the pods")
		err = fakeClient.Get(ctx, test.NamespacedName("toolchain-host-operator", "host-operator-controller-manager"), actualPod)
		//pods are actually deleted
		require.True(t, apierror.IsNotFound(err))
		require.Contains(t, term.Output(), "Checking the status of the deleted pod's deployment")
		//checking the flow for non-operator deployments
		require.Contains(t, term.Output(), "Proceeding to restart the non-operator deployment")
		require.Contains(t, term.Output(), "Checking the status of the rolled out deployment")
	})

	t.Run("restart deployment works successfully when only operator based deployment", func(t *testing.T) {
		//given
		newClient, fakeClient := NewFakeClients(t, toolchainCluster, hostDeployment, hostPod)
		ctx := clicontext.NewCommandContext(term, newClient)

		//when
		err := restartDeployment(ctx, fakeClient, "toolchain-host-operator", nil, testIOStreams,
			func(ctx *clicontext.CommandContext, f cmdutil.Factory, ioStreams genericclioptions.IOStreams, labelSelector string) error {
				require.Equal(t, testIOStreams, ioStreams)
				require.Equal(t, nil, f)
				return nil
			}, func(ctx *clicontext.CommandContext, deployment appsv1.Deployment, f cmdutil.Factory, ioStreams genericclioptions.IOStreams) error {
				require.Equal(t, testIOStreams, ioStreams)
				require.Equal(t, nil, f)
				return nil
			})

		//then
		require.NoError(t, err)
		require.Contains(t, term.Output(), "No Non-operator deployment found in namespace toolchain-host-operator, hence no restart happened")
	})
	t.Run("rollout restart returns an error", func(t *testing.T) {
		//given
		newClient, fakeClient := NewFakeClients(t, toolchainCluster, hostDeployment, regServDeployment, hostPod)
		ctx := clicontext.NewCommandContext(term, newClient)
		expectedErr := fmt.Errorf("Could not do rollout restart of the deployment")
		//when
		err := restartDeployment(ctx, fakeClient, "toolchain-host-operator", nil, testIOStreams,
			func(ctx *clicontext.CommandContext, f cmdutil.Factory, ioStreams genericclioptions.IOStreams, labelSelector string) error {
				require.Equal(t, testIOStreams, ioStreams)
				require.Equal(t, nil, f)
				return nil
			}, func(ctx *clicontext.CommandContext, deployment appsv1.Deployment, f cmdutil.Factory, ioStreams genericclioptions.IOStreams) error {
				require.Equal(t, testIOStreams, ioStreams)
				require.Equal(t, nil, f)
				return expectedErr
			})

		//then
		require.EqualError(t, err, expectedErr.Error())
	})

	t.Run("rollout status for the deleted pods(operator) works", func(t *testing.T) {
		//given
		newClient, fakeClient := NewFakeClients(t, toolchainCluster, hostDeployment)
		ctx := clicontext.NewCommandContext(term, newClient)

		//when
		err := restartDeployment(ctx, fakeClient, "toolchain-host-operator", nil, testIOStreams,
			func(ctx *clicontext.CommandContext, f cmdutil.Factory, ioStreams genericclioptions.IOStreams, labelSelector string) error {
				require.Equal(t, testIOStreams, ioStreams)
				return nil
			}, nil)

		//then
		require.NoError(t, err)
	})

	t.Run("error in rollout status of the deleted pods(operator)", func(t *testing.T) {
		//given
		newClient, fakeClient := NewFakeClients(t, toolchainCluster, hostDeployment)
		ctx := clicontext.NewCommandContext(term, newClient)
		expectedErr := fmt.Errorf("Could not check the status of the deployment")
		//when
		err := restartDeployment(ctx, fakeClient, "toolchain-host-operator", nil, genericclioptions.NewTestIOStreamsDiscard(),
			func(ctx *clicontext.CommandContext, f cmdutil.Factory, ioStreams genericclioptions.IOStreams, labelSelector string) error {
				return expectedErr
			}, nil)

		//then
		require.EqualError(t, err, expectedErr.Error())
	})

	t.Run("autoscalling deployment should not restart", func(t *testing.T) {
		//given
		newClient, fakeClient := NewFakeClients(t, toolchainCluster, memberDeployment, autoscalarDeployment)
		ctx := clicontext.NewCommandContext(term, newClient)
		//when
		err := restartDeployment(ctx, fakeClient, "toolchain-member-operator", nil, genericclioptions.NewTestIOStreamsDiscard(),
			func(ctx *clicontext.CommandContext, f cmdutil.Factory, ioStreams genericclioptions.IOStreams, labelSelector string) error {
				return nil
			}, nil)

		//then
		require.NoError(t, err)
		require.Contains(t, term.Output(), "Found only autoscaling-buffer deployment in namespace toolchain-member-operator , which is not required to be restarted")
	})

}

func TestRestart(t *testing.T) {

	t.Run("restart should start with y response", func(t *testing.T) {
		//given
		SetFileConfig(t, Host())
		toolchainCluster := NewToolchainCluster(ToolchainClusterName("host"))
		deployment := newDeployment(test.NamespacedName("toolchain-host-operator", "host-operator-controller-manager"), 1)
		deployment.Labels = make(map[string]string)
		deployment.Labels["kubesaw-control-plane"] = "kubesaw-controller-manager"
		term := NewFakeTerminalWithResponse("Y")
		newClient, _ := NewFakeClients(t, toolchainCluster, deployment)
		ctx := clicontext.NewCommandContext(term, newClient)

		//when
		err := restart(ctx, "host")

		//then
		require.ErrorContains(t, err, "no such host") //we expect an error as we have not set up any http client , just checking that it passes the cmd phase and restartdeployment method is called
		require.Contains(t, term.Output(), "Fetching the current Operator and non-Operator deployments of the operator in")
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
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"host": "controller"}},
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
			Labels:    map[string]string{"host": "controller"},
		},
		Spec: corev1.PodSpec{},
		Status: corev1.PodStatus{
			Phase: "Running",
		},
	}
}

func checkDeploymentBeingUpdated(t *testing.T, fakeClient *test.FakeClient, namespacedName types.NamespacedName, currentReplicas int32, numberOfUpdateCalls *int, deployment *appsv1.Deployment) {
	// on the first call, we should have a deployment with 3 replicas ("current") and request to delete to 0 ("requested")
	if *numberOfUpdateCalls == 0 {
		// check the current deployment's replicas field
		AssertDeploymentHasReplicas(t, fakeClient, namespacedName, currentReplicas)
		// check the requested deployment's replicas field
		assert.Equal(t, int32(0), *deployment.Spec.Replicas)
	} else {
		// check the current deployment's replicas field
		AssertDeploymentHasReplicas(t, fakeClient, namespacedName, 0)
		// check the requested deployment's replicas field
		assert.Equal(t, currentReplicas, *deployment.Spec.Replicas)
	}
	*numberOfUpdateCalls++
}

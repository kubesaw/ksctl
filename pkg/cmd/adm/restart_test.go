package adm

import (
	"bytes"
	"io"
	"net/http"
	"testing"

	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	clicontext "github.com/kubesaw/ksctl/pkg/context"
	. "github.com/kubesaw/ksctl/pkg/test"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
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
		"NonOperatorHostDeployment": {
			namespace:      "toolchain-host-operator",
			name:           "registration-service",
			labelKey:       "toolchain.dev.openshift.com/provider",
			labelValue:     "codeready-toolchain",
			expectedMsg:    "deployment \"registration-service\" successfully rolled out\n",
			labelSelector:  "toolchain.dev.openshift.com/provider=codeready-toolchain",
			expectedOutput: "deployment.apps/registration-service restarted\n",
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
			tf.Client = &RolloutRestartRESTClient{
				RESTClient: &fake.RESTClient{
					GroupVersion:         rolloutGroupVersionEncoder,
					NegotiatedSerializer: ns,
					Client: fake.CreateHTTPClient(func(req *http.Request) (*http.Response, error) {
						responseDeployment1 := appsv1.Deployment{}
						responseDeployment1.Name = deployment1.Name
						responseDeployment1.Labels = make(map[string]string)
						responseDeployment1.Labels[tc.labelKey] = tc.labelValue
						responseDeployment2 := appsv1.Deployment{}
						responseDeployment2.Name = deployment2.Name
						responseDeployment2.Labels = make(map[string]string)
						responseDeployment2.Labels[tc.labelKey1] = tc.labelValue1
						body := io.NopCloser(bytes.NewReader([]byte(runtime.EncodeOrDie(encoder, &responseDeployment1))))
						return &http.Response{StatusCode: http.StatusOK, Header: cmdtesting.DefaultHeader(), Body: body}, nil
					}),
				},
			}
			tf.FakeDynamicClient.WatchReactionChain = nil
			tf.FakeDynamicClient.AddWatchReactor("*", func(action cgtesting.Action) (handled bool, ret watch.Interface, err error) {
				fw := watch.NewFake()
				dep := &appsv1.Deployment{}
				dep.Name = deployment1.Name
				dep.Status = appsv1.DeploymentStatus{
					Replicas:            1,
					UpdatedReplicas:     1,
					ReadyReplicas:       1,
					AvailableReplicas:   1,
					UnavailableReplicas: 0,
					Conditions: []appsv1.DeploymentCondition{{
						Type: appsv1.DeploymentAvailable,
					}},
				}
				dep.Labels = make(map[string]string)
				dep.Labels[tc.labelKey] = tc.labelValue
				c, err := runtime.DefaultUnstructuredConverter.ToUnstructured(dep.DeepCopyObject())
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
			err := restartDeployment(ctx, fakeClient, namespacedName.Namespace, tf, streams)
			if tc.labelValue == "kubesaw-controller-manager" {
				require.NoError(t, err)
				require.Contains(t, term.Output(), "Fetching the current OLM and non-OLM deployments of the operator in")
				require.Contains(t, term.Output(), "Proceeding to delete the Pods of")
				require.Contains(t, term.Output(), "Listing the pods to be deleted")
				require.Contains(t, term.Output(), "Starting to delete the pods")
				require.Contains(t, term.Output(), "Checking the status of the deleted pod's deployment")
				//checking the output from kubectl for rolloutstatus
				require.Contains(t, buf.String(), tc.expectedOutput)
			} else if tc.labelValue == "codeready-toolchain" {
				require.Error(t, err)
			}

		})
	}
}

func TestRestart(t *testing.T) {

	t.Run("restart should start with y response", func(t *testing.T) {
		//given
		SetFileConfig(t, Host())
		toolchainCluster := NewToolchainCluster(ToolchainClusterName("host"))
		deployment := newDeployment(test.NamespacedName("toolchain-host-operator", "host-operator-controller-manager"), 1)
		term := NewFakeTerminalWithResponse("Y")
		newClient, _ := NewFakeClients(t, toolchainCluster, deployment)
		ctx := clicontext.NewCommandContext(term, newClient)

		//when
		restart(ctx, "host")

		//then
		require.Contains(t, term.Output(), "Fetching the current OLM and non-OLM deployments of the operator in")
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

type RolloutRestartRESTClient struct {
	*fake.RESTClient
}

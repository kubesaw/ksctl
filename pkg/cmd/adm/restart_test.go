package adm

import (
	"bytes"
	"io"
	"net/http"
	"testing"

	clicontext "github.com/kubesaw/ksctl/pkg/context"
	. "github.com/kubesaw/ksctl/pkg/test"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
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

func TestRestart(t *testing.T) {
	// given
	tests := map[string]struct {
		namespace      string
		name           string
		labelKey       string
		labelValue     string
		expectedMsg    string
		labelSelector  string
		expectedOutput string
	}{
		"OlmHostDeployment": {
			namespace:     "toolchain-host-operator",
			name:          "host-operator-controller-manager",
			labelKey:      "kubesaw-control-plane",
			labelValue:    "kubesaw-controller-manager",
			expectedMsg:   "deployment \"host-operator-controller-manager\" successfully rolled out\n",
			labelSelector: "kubesaw-control-plane=kubesaw-controller-manager",
		},
		"NonOlmHostDeployment": {
			namespace:      "toolchain-host-operator",
			name:           "registration-service",
			labelKey:       "provider",
			labelValue:     "codeready-toolchain",
			expectedMsg:    "deployment \"registration-service\" successfully rolled out\n",
			labelSelector:  "provider=codeready-toolchain",
			expectedOutput: "deployment.apps/registration-service restarted\n",
		},
		"OlmMemberDeployment": {
			namespace:     "toolchain-member-operator",
			name:          "member-operator-controller-manager",
			labelKey:      "kubesaw-control-plane",
			labelValue:    "kubesaw-controller-manager",
			expectedMsg:   "deployment \"member-operator-controller-manager\" successfully rolled out\n",
			labelSelector: "kubesaw-control-plane=kubesaw-controller-manager",
		},
		"NonOlmMemberDeployment": {
			namespace:      "toolchain-member-operator",
			name:           "member-webhooks",
			labelKey:       "provider",
			labelValue:     "codeready-toolchain",
			expectedMsg:    "deployment \"member-webhooks\" successfully rolled out\n",
			labelSelector:  "provider=codeready-toolchain",
			expectedOutput: "deployment.apps/member-webhooks restarted\n",
		},
	}
	for k, tc := range tests {
		t.Run(k, func(t *testing.T) {
			//given
			namespacedName := types.NamespacedName{
				Namespace: tc.namespace,
				Name:      tc.name,
			}
			var rolloutGroupVersionEncoder = schema.GroupVersion{Group: "apps", Version: "v1"}
			deployment1 := newDeployment(namespacedName, 1)
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
						responseDeployment := &appsv1.Deployment{}
						responseDeployment.Name = deployment1.Name
						responseDeployment.Labels = make(map[string]string)
						responseDeployment.Labels[tc.labelKey] = tc.labelValue
						body := io.NopCloser(bytes.NewReader([]byte(runtime.EncodeOrDie(encoder, responseDeployment))))
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
			deployment := newDeployment(namespacedName, 1)
			deployment.Labels = map[string]string{tc.labelKey: tc.labelValue}
			term := NewFakeTerminalWithResponse("Y")
			newClient, fakeClient := NewFakeClients(t, deployment)
			ctx := clicontext.NewCommandContext(term, newClient)

			//when
			err := restartDeployment(ctx, fakeClient, namespacedName.Namespace, tf, streams)
			if tc.labelValue == "kubesaw-controller-manager" {
				require.NoError(t, err, "non-OLM based deployment not found in")
			} else if tc.labelValue == "codeready-toolchain" {
				require.NoError(t, err, "OLM based deployment not found in")
				err := restartNonOlmDeployments(*deployment1, tf, streams)
				require.NoError(t, err)
				//checking the output from kubectl
				require.Contains(t, buf.String(), tc.expectedOutput)
			}
			err1 := checkRolloutStatus(tf, streams, tc.labelSelector)
			require.NoError(t, err1)
			//checking the output from kubectl
			require.Contains(t, buf.String(), tc.expectedMsg)

		})
	}
}

func newDeployment(namespacedName types.NamespacedName, replicas int32) *appsv1.Deployment { //nolint:unparam
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespacedName.Namespace,
			Name:      namespacedName.Name,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
		},
	}
}

type RolloutRestartRESTClient struct {
	*fake.RESTClient
}

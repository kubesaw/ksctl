package adm

import (
	"bytes"
	"io"
	"net/http"
	"testing"

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
	SetFileConfig(t, Host())
	namespacedName := types.NamespacedName{
		Namespace: "toolchain-host-operator",
		Name:      "host-operator-controller-manager",
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
				responseDeployment.Labels["kubesaw-control-plane"] = "kubesaw-controller-manager"
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
		dep.Labels["kubesaw-control-plane"] = "kubesaw-controller-manager"
		c, err := runtime.DefaultUnstructuredConverter.ToUnstructured(dep.DeepCopyObject())
		if err != nil {
			t.Errorf("unexpected err %s", err)
		}
		u := &unstructured.Unstructured{}
		u.SetUnstructuredContent(c)
		go fw.Add(u)
		return true, fw, nil
	})

	//add comments that it is checking the output from kubectl
	streams, _, buf, _ := genericclioptions.NewTestIOStreams()
	t.Run("Rollout restart of non-olm deployments is successful", func(t *testing.T) {
		// given

		err := restartNonOlmDeployments(*deployment1, tf, streams)
		expectedOutput := "deployment.apps/" + deployment1.Name + " restarted\n"
		require.NoError(t, err)
		require.Contains(t, buf.String(), expectedOutput)

	})

	t.Run("check rollout status of deployments is successful", func(t *testing.T) {
		//when
		err := checkRolloutStatus(tf, streams, "kubesaw-control-plane=kubesaw-controller-manager")

		//then
		require.NoError(t, err)

		expectedMsg := "deployment \"host-operator-controller-manager\" successfully rolled out\n"
		require.Contains(t, buf.String(), expectedMsg)

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
		},
	}
}

type RolloutRestartRESTClient struct {
	*fake.RESTClient
}

package cmd_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kubesaw/ksctl/pkg/cmd"
	"github.com/kubesaw/ksctl/pkg/configuration"
	. "github.com/kubesaw/ksctl/pkg/test"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestLogs(t *testing.T) {

	// given
	server := NewLogsServer(t)
	t.Logf("server URL: %s", server.URL)
	defer server.Close()
	SetFileConfig(t, Host(ServerAPI(server.URL)))

	wasVerbose := configuration.Verbose
	configuration.Verbose = true
	t.Cleanup(func() {
		configuration.Verbose = wasVerbose
	})

	t.Run("logs with with long-hand target cluster and namespace flags", func(t *testing.T) {
		// given
		logsCmd := cmd.NewLogsCmd()
		logsCmd.SetArgs([]string{
			"--target-cluster=host",
			"--namespace=toolchain-host-operator",
			"--insecure-skip-tls-verify=true",
			"cheesecake",
		})

		// when
		_, err := logsCmd.ExecuteC()

		// then
		require.NoError(t, err)
	})

	t.Run("logs with with short-hand target cluster and namespace flags", func(t *testing.T) {
		// given
		logsCmd := cmd.NewLogsCmd()
		logsCmd.SetArgs([]string{
			"-t=host",
			"-n=toolchain-host-operator",
			"--insecure-skip-tls-verify=true",
			"cheesecake",
		})

		// when
		_, err := logsCmd.ExecuteC()

		// then
		require.NoError(t, err)
	})

	t.Run("logs with default namespace", func(t *testing.T) {
		// given
		logsCmd := cmd.NewLogsCmd()
		logsCmd.SetArgs([]string{
			"--target-cluster=host",
			// "--namespace=...", // will default to `toolchain-host-operator`
			"--insecure-skip-tls-verify=true",
			"cheesecake",
		})

		// when
		_, err := logsCmd.ExecuteC()

		// then
		require.NoError(t, err)
	})

	t.Run("missing '--cluster' flag", func(t *testing.T) {
		// given
		logsCmd := cmd.NewLogsCmd()
		logsCmd.SetArgs([]string{
			"--namespace=toolchain-host-operator",
			"--insecure-skip-tls-verify=true",
			"cheesecake",
		})

		// when
		_, err := logsCmd.ExecuteC()

		// then
		require.Error(t, err)
		require.Error(t, err, "you must specify the target cluster")
	})
}

// NewLogsServer returns a new HTTP Server which supports:
// - calls to `/api`
// - calls to `/apis`
// - calls on some predefined resources
// - 404 responses otherwise
// see https://github.com/kubernetes/client-go/blob/master/discovery/discovery_client_test.go
func NewLogsServer(t *testing.T) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var response interface{}
		switch req.Method {
		case "GET":
			switch req.URL.Path {
			case "/api/v1":
				response = &metav1.APIResourceList{
					GroupVersion: "v1",
					APIResources: []metav1.APIResource{
						{
							Name:         "pods",
							SingularName: "pod",
							ShortNames:   []string{"po"},
							Namespaced:   true,
							Kind:         "Pod",
						},
					},
				}
			case "/api":
				response = &metav1.APIVersions{
					Versions: []string{
						"v1",
					},
				}
			case "/apis":
				response = &metav1.APIGroupList{
					Groups: []metav1.APIGroup{},
				}

			case "/api/v1/namespaces/toolchain-host-operator/pods/cheesecake":
				response = corev1.Pod{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "v1",
						Kind:       "Pod",
					},
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "toolchain-host-operator",
						Name:      "cheesecake",
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name: "default",
							},
						},
					},
					Status: corev1.PodStatus{
						Phase: "Running",
					},
				}
			case "/api/v1/namespaces/toolchain-host-operator/pods/cheesecake/log":
				response = "some content"
			default:
				t.Errorf("not found: %s %s\n", req.Method, req.URL)
				w.WriteHeader(http.StatusNotFound)
				return
			}
		default:
			t.Errorf("unexpected request: %s %s\n", req.Method, req.URL)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		output, err := json.Marshal(response)
		if err != nil {
			t.Errorf("unexpected encoding error: %v", err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(output) // nolint: errcheck
	}))
}

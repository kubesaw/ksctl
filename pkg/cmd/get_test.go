package cmd_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/hash"
	"github.com/kubesaw/ksctl/pkg/cmd"
	. "github.com/kubesaw/ksctl/pkg/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGet(t *testing.T) {

	// given
	server := NewGetServer(t)
	t.Logf("server URL: %s", server.URL)
	defer server.Close()
	SetFileConfig(t, Host(ServerAPI(server.URL)))

	t.Run("get pods with long-hand target cluster and namespace flags", func(t *testing.T) {
		// given
		getCmd := cmd.NewGetCmd()
		getCmd.SetArgs([]string{
			"--target-cluster=host",
			"--namespace=toolchain-host-operator",
			"--insecure-skip-tls-verify=true",
			"pods",
		})

		// when
		_, err := getCmd.ExecuteC()

		// then
		require.NoError(t, err)
	})

	t.Run("get pods with short-hand target cluster and namespace flags", func(t *testing.T) {
		// given
		getCmd := cmd.NewGetCmd()
		getCmd.SetArgs([]string{
			"-t=host",
			"-n=toolchain-host-operator",
			"--insecure-skip-tls-verify=true",
			"pods",
		})

		// when
		_, err := getCmd.ExecuteC()

		// then
		require.NoError(t, err)
	})

	t.Run("get pods with default namespace", func(t *testing.T) {
		// given
		getCmd := cmd.NewGetCmd()
		getCmd.SetArgs([]string{
			"--target-cluster=host",
			// "--namespace=...", // will default to `toolchain-host-operator`
			"--insecure-skip-tls-verify=true",
			"pods",
		})

		// when
		_, err := getCmd.ExecuteC()

		// then
		require.NoError(t, err)
	})

	t.Run("missing 'cluster' flag", func(t *testing.T) {
		// given
		getCmd := cmd.NewGetCmd()
		getCmd.SetArgs([]string{
			"--namespace=toolchain-host-operator",
			"--insecure-skip-tls-verify=true",
			"pods",
		})

		// when
		_, err := getCmd.ExecuteC()

		// then
		require.Error(t, err)
		require.Error(t, err, "you must specify the target cluster")
	})
}

func TestGetWithEmailFlag(t *testing.T) {
	email := "user@example.com"
	emailHash := hash.EncodeString(email)
	expectedSelector := toolchainv1alpha1.UserSignupUserEmailHashLabelKey + "=" + emailHash

	t.Run("get usersignups with email flag applies label selector", func(t *testing.T) {
		// given
		var receivedSelector string
		server := NewGetServerWithUserSignups(t, func(selector string) {
			receivedSelector = selector
		})
		defer server.Close()
		SetFileConfig(t, Host(ServerAPI(server.URL)))

		getCmd := cmd.NewGetCmd()
		getCmd.SetArgs([]string{
			"--target-cluster=host",
			"--namespace=toolchain-host-operator",
			"--insecure-skip-tls-verify=true",
			"--email=" + email,
			"usersignups",
		})

		// when
		_, err := getCmd.ExecuteC()

		// then
		require.NoError(t, err)
		assert.Equal(t, expectedSelector, receivedSelector)
	})

	t.Run("get usersignups without email flag sends no label selector", func(t *testing.T) {
		// given
		var receivedSelector string
		server := NewGetServerWithUserSignups(t, func(selector string) {
			receivedSelector = selector
		})
		defer server.Close()
		SetFileConfig(t, Host(ServerAPI(server.URL)))

		getCmd := cmd.NewGetCmd()
		getCmd.SetArgs([]string{
			"--target-cluster=host",
			"--namespace=toolchain-host-operator",
			"--insecure-skip-tls-verify=true",
			"usersignups",
		})

		// when
		_, err := getCmd.ExecuteC()

		// then
		require.NoError(t, err)
		assert.Empty(t, receivedSelector)
	})

	t.Run("get usersignups with email flag preserves existing selector", func(t *testing.T) {
		// given
		var receivedSelector string
		server := NewGetServerWithUserSignups(t, func(selector string) {
			receivedSelector = selector
		})
		defer server.Close()
		SetFileConfig(t, Host(ServerAPI(server.URL)))

		existingSelector := "app=myapp"
		getCmd := cmd.NewGetCmd()
		getCmd.SetArgs([]string{
			"--target-cluster=host",
			"--namespace=toolchain-host-operator",
			"--insecure-skip-tls-verify=true",
			"--email=" + email,
			"--selector=" + existingSelector,
			"usersignups",
		})

		// when
		_, err := getCmd.ExecuteC()

		// then
		require.NoError(t, err)
		assert.Contains(t, receivedSelector, existingSelector)
		assert.Contains(t, receivedSelector, toolchainv1alpha1.UserSignupUserEmailHashLabelKey+"="+emailHash)
	})

	t.Run("email flag is rejected when not querying usersignups", func(t *testing.T) {
		// given
		server := NewGetServer(t)
		defer server.Close()
		SetFileConfig(t, Host(ServerAPI(server.URL)))

		getCmd := cmd.NewGetCmd()
		getCmd.SetArgs([]string{
			"--target-cluster=host",
			"--namespace=toolchain-host-operator",
			"--insecure-skip-tls-verify=true",
			"--email=" + email,
			"pods",
		})

		// when
		_, err := getCmd.ExecuteC()

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), `"--email" flag can only be used to filter UserSignups`)
	})

	t.Run("email flag works with mixed-case resource name", func(t *testing.T) {
		// given
		var receivedSelector string
		server := NewGetServerWithUserSignups(t, func(selector string) {
			receivedSelector = selector
		})
		defer server.Close()
		SetFileConfig(t, Host(ServerAPI(server.URL)))

		getCmd := cmd.NewGetCmd()
		getCmd.SetArgs([]string{
			"--target-cluster=host",
			"--namespace=toolchain-host-operator",
			"--insecure-skip-tls-verify=true",
			"--email=" + email,
			"UserSignups",
		})

		// when
		_, err := getCmd.ExecuteC()

		// then
		require.NoError(t, err)
		assert.Equal(t, expectedSelector, receivedSelector)
	})

	t.Run("email flag works with singular resource name", func(t *testing.T) {
		// given
		var receivedSelector string
		server := NewGetServerWithUserSignups(t, func(selector string) {
			receivedSelector = selector
		})
		defer server.Close()
		SetFileConfig(t, Host(ServerAPI(server.URL)))

		getCmd := cmd.NewGetCmd()
		getCmd.SetArgs([]string{
			"--target-cluster=host",
			"--namespace=toolchain-host-operator",
			"--insecure-skip-tls-verify=true",
			"--email=" + email,
			"usersignup",
		})

		// when
		_, err := getCmd.ExecuteC()

		// then
		require.NoError(t, err)
		assert.Equal(t, expectedSelector, receivedSelector)
	})

	t.Run("different emails produce different selectors", func(t *testing.T) {
		// given
		otherEmail := "other@example.com"
		otherHash := hash.EncodeString(otherEmail)

		var receivedSelector string
		server := NewGetServerWithUserSignups(t, func(selector string) {
			receivedSelector = selector
		})
		defer server.Close()
		SetFileConfig(t, Host(ServerAPI(server.URL)))

		getCmd := cmd.NewGetCmd()
		getCmd.SetArgs([]string{
			"--target-cluster=host",
			"--namespace=toolchain-host-operator",
			"--insecure-skip-tls-verify=true",
			"--email=" + otherEmail,
			"usersignups",
		})

		// when
		_, err := getCmd.ExecuteC()

		// then
		require.NoError(t, err)
		assert.NotEqual(t, expectedSelector, receivedSelector)
		assert.Contains(t, receivedSelector, otherHash)
	})

	t.Run("email flag is rejected with comma-separated multi-resource query", func(t *testing.T) {
		// given
		server := NewGetServerWithUserSignups(t, func(_ string) {})
		defer server.Close()
		SetFileConfig(t, Host(ServerAPI(server.URL)))

		getCmd := cmd.NewGetCmd()
		getCmd.SetArgs([]string{
			"--target-cluster=host",
			"--namespace=toolchain-host-operator",
			"--insecure-skip-tls-verify=true",
			"--email=" + email,
			"pods,usersignups",
		})

		// when
		_, err := getCmd.ExecuteC()

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), `"--email" flag can only be used to filter UserSignups`)
	})

	t.Run("email flag is rejected when no resource type is specified", func(t *testing.T) {
		// given
		server := NewGetServer(t)
		defer server.Close()
		SetFileConfig(t, Host(ServerAPI(server.URL)))

		getCmd := cmd.NewGetCmd()
		getCmd.SetArgs([]string{
			"--target-cluster=host",
			"--namespace=toolchain-host-operator",
			"--insecure-skip-tls-verify=true",
			"--email=" + email,
		})

		// when
		_, err := getCmd.ExecuteC()

		// then
		require.Error(t, err)
	})

	t.Run("email flag works with fully-qualified resource name", func(t *testing.T) {
		// given
		var receivedSelector string
		server := NewGetServerWithUserSignups(t, func(selector string) {
			receivedSelector = selector
		})
		defer server.Close()
		SetFileConfig(t, Host(ServerAPI(server.URL)))

		getCmd := cmd.NewGetCmd()
		getCmd.SetArgs([]string{
			"--target-cluster=host",
			"--namespace=toolchain-host-operator",
			"--insecure-skip-tls-verify=true",
			"--email=" + email,
			"usersignups.v1alpha1.toolchain.dev.openshift.com",
		})

		// when
		_, err := getCmd.ExecuteC()

		// then
		require.NoError(t, err)
		assert.Equal(t, expectedSelector, receivedSelector)
	})

	t.Run("empty email flag does not inject a selector", func(t *testing.T) {
		// given
		var receivedSelector string
		server := NewGetServerWithUserSignups(t, func(selector string) {
			receivedSelector = selector
		})
		defer server.Close()
		SetFileConfig(t, Host(ServerAPI(server.URL)))

		getCmd := cmd.NewGetCmd()
		getCmd.SetArgs([]string{
			"--target-cluster=host",
			"--namespace=toolchain-host-operator",
			"--insecure-skip-tls-verify=true",
			"--email=",
			"usersignups",
		})

		// when
		_, err := getCmd.ExecuteC()

		// then
		require.NoError(t, err)
		assert.Empty(t, receivedSelector)
	})
}

// NewGetServer returns a new HTTP Server which supports:
// - calls to `/api`
// - calls to `/apis`
// - calls on some predefined resources
// - 404 responses otherwise
// see https://github.com/kubernetes/client-go/blob/master/discovery/discovery_client_test.go
func NewGetServer(t *testing.T) *httptest.Server {
	return newGetServer(t, nil)
}

// NewGetServerWithUserSignups is like NewGetServer but also registers the
// toolchain.dev.openshift.com API group with the usersignups resource.
// The onSelector callback receives the labelSelector query parameter value
// whenever the usersignups endpoint is called.
func NewGetServerWithUserSignups(t *testing.T, onSelector func(string)) *httptest.Server {
	return newGetServer(t, onSelector)
}

func newGetServer(t *testing.T, onSelector func(string)) *httptest.Server {
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
				groups := []metav1.APIGroup{}
				if onSelector != nil {
					groups = append(groups, metav1.APIGroup{
						Name: "toolchain.dev.openshift.com",
						Versions: []metav1.GroupVersionForDiscovery{
							{
								GroupVersion: "toolchain.dev.openshift.com/v1alpha1",
								Version:      "v1alpha1",
							},
						},
						PreferredVersion: metav1.GroupVersionForDiscovery{
							GroupVersion: "toolchain.dev.openshift.com/v1alpha1",
							Version:      "v1alpha1",
						},
					})
				}
				response = &metav1.APIGroupList{
					Groups: groups,
				}

			case "/apis/toolchain.dev.openshift.com/v1alpha1":
				response = &metav1.APIResourceList{
					GroupVersion: "toolchain.dev.openshift.com/v1alpha1",
					APIResources: []metav1.APIResource{
						{
							Name:         "usersignups",
							SingularName: "usersignup",
							Namespaced:   true,
							Kind:         "UserSignup",
							Verbs:        metav1.Verbs{"get", "list"},
						},
					},
				}

			case "/apis/toolchain.dev.openshift.com/v1alpha1/namespaces/toolchain-host-operator/usersignups":
				if onSelector != nil {
					onSelector(req.URL.Query().Get("labelSelector"))
				}
				response = map[string]interface{}{
					"apiVersion": "toolchain.dev.openshift.com/v1alpha1",
					"kind":       "UserSignupList",
					"metadata":   map[string]interface{}{},
					"items":      []interface{}{},
				}

			case "/api/v1/namespaces/toolchain-host-operator/pods":
				response = corev1.PodList{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "v1",
						Kind:       "Pod",
					},
					ListMeta: metav1.ListMeta{},
					Items: []corev1.Pod{
						{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "toolchain-host-operator",
								Name:      "cheesecake",
							},
							Spec: corev1.PodSpec{},
							Status: corev1.PodStatus{
								Phase: "Running",
							},
						},
					},
				}

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

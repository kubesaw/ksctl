package adm_test

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/h2non/gock"
	"github.com/kubesaw/ksctl/pkg/cmd/adm"
	. "github.com/kubesaw/ksctl/pkg/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	authv1 "k8s.io/api/authentication/v1"
	restclient "k8s.io/client-go/rest"
	"k8s.io/kubectl/pkg/scheme"
)

func TestMustGatherNamespaceCmd(t *testing.T) {

	// given
	t.Cleanup(gock.OffAll)
	apiServerURL := "https://api.example.com"
	newAPIServer(apiServerURL)
	kubeconfig := &restclient.Config{
		Host: apiServerURL,
		ContentConfig: restclient.ContentConfig{
			GroupVersion:         &authv1.SchemeGroupVersion,
			NegotiatedSerializer: scheme.Codecs,
		},
	}
	term := NewFakeTerminalWithResponse("Y")
	term.Tee(os.Stdout)

	t.Run("ok", func(t *testing.T) {
		t.Run("create the dest-dir on-the-fly", func(t *testing.T) {
			// given
			baseDir, err := os.MkdirTemp("", "ksctl-out-")
			require.NoError(t, err)
			destDir := filepath.Join(baseDir, "test-dev")

			// when
			err = adm.MustGatherNamespace(term, kubeconfig, "test-dev", destDir)

			// then
			require.NoError(t, err)
			// verify that the files exist
			assertFileContents(t, destDir, fileContents)
		})

		t.Run("dest-dir already exists and is empty", func(t *testing.T) {
			// given
			baseDir, err := os.MkdirTemp("", "ksctl-out-")
			require.NoError(t, err)
			destDir := filepath.Join(baseDir, "test-dev")
			err = os.Mkdir(destDir, 0755)
			require.NoError(t, err)

			// when
			err = adm.MustGatherNamespace(term, kubeconfig, "test-dev", destDir)

			// then
			require.NoError(t, err)
			// verify that the files exist
			assertFileContents(t, destDir, fileContents)
		})
	})

	t.Run("failure", func(t *testing.T) {

		t.Run("dest-dir already exists but is not empty", func(t *testing.T) {
			// given
			baseDir, err := os.MkdirTemp("", "ksctl-out-")
			require.NoError(t, err)
			destDir := filepath.Join(baseDir, "test-dev")
			err = os.Mkdir(destDir, 0755)
			require.NoError(t, err)
			// put some contents
			err = os.WriteFile(filepath.Join(destDir, "test.yaml"), []byte("apiVersion; v1"), 0600)
			require.NoError(t, err)

			// when
			err = adm.MustGatherNamespace(term, kubeconfig, "test-dev", destDir)

			// then
			require.NoError(t, err) // no error occurred, but command aborted
			assert.Contains(t, term.Output(), fmt.Sprintf("The '%s' dest-dir is not empty. Aborting.", destDir))
		})
	})

}

func assertFileContents(t *testing.T, destDir string, fileContents map[string]string) {
	for filename, expectedContents := range fileContents {
		actualContents, err := os.ReadFile(filepath.Join(destDir, filename))
		require.NoError(t, err, fmt.Sprintf("'%s' is missing", filename))
		assert.Equal(t, expectedContents, string(actualContents))
	}
}

func get(path string) gock.MatchFunc {
	return func(r *http.Request, _ *gock.Request) (bool, error) {
		return r.Method == "GET" && r.URL.Path == path, nil
	}
}
func newAPIServer(uri string) {
	// gock.Observe(gock.DumpRequest)
	gock.New(uri).AddMatcher(get("/api")).Persist().Reply(200).SetHeader("Content-Type", "application/json").BodyString(
		`{
			"kind": "APIVersions",
			"versions": [
			  "v1"
			]
		}`,
	)
	gock.New(uri).AddMatcher(get("/api/v1")).Persist().Reply(200).BodyString(
		`{
			"kind": "APIResourceList",
			"groupVersion": "v1",
			"resources": [
				{
					"name": "bindings",
					"singularName": "binding",
					"namespaced": true,
					"kind": "Binding",
					"verbs": [
					  "create"
					]
				},
				{
					"name": "pods",
					"singularName": "pod",
					"namespaced": true,
					"kind": "Pod",
					"verbs": [
					  "create",
					  "delete",
					  "deletecollection",
					  "get",
					  "list",
					  "patch",
					  "update",
					  "watch"
					],
					"shortNames": [
					  "po"
					],
					"categories": [
					  "all"
					]
				},
				{
					"name": "pods/log",
					"singularName": "",
					"namespaced": true,
					"kind": "Pod",
					"verbs": [
					  "get"
					]
				},
				{
					"name": "podtemplates",
					"namespaced": true,
					"kind": "PodTemplate",
					"verbs": [
					  "create",
					  "delete",
					  "deletecollection",
					  "get",
					  "list",
					  "patch",
					  "update",
					  "watch"
					]
				}
			]
		}`,
	)
	gock.New(uri).AddMatcher(get("/apis")).Persist().Reply(200).SetHeader("Content-Type", "application/json").BodyString(
		`{
			"apiVersion": "v1",
			"kind": "APIGroupList",
			"groups": [
				{
					"name": "rbac.authorization.k8s.io",
					"versions": [
					  {
						"groupVersion": "rbac.authorization.k8s.io/v1",
						"version": "v1"
					  }
					],
					"preferredVersion": {
					  "groupVersion": "rbac.authorization.k8s.io/v1",
					  "version": "v1"
					}
				}
			]
		}`,
	)
	gock.New(uri).AddMatcher(get("/api/v1/namespaces/test-dev/pods")).Persist().Reply(200).BodyString(
		`{
			"apiVersion": "v1",
			"kind": "List",
			"items": [
				{
					"apiVersion": "v1",
					"kind": "Pod",
					"metadata": {
						"name": "pasta",
						"namespace": "test-dev"
					},
					"status": {
						"containerStatuses": [
							{
							  "containerID": "cri-o://pasta",
							  "image": "pasta1:latest",
							  "name": "container1",
							  "ready": true,
							  "started": true
							},
							{
							  "containerID": "cri-o://pasta",
							  "image": "pasta2:latest",
							  "name": "container2",
							  "ready": true,
							  "started": true
							}
						]
					}
				},
				{
					"apiVersion": "v1",
					"kind": "Pod",
					"metadata": {
						"name": "cookie",
						"namespace": "test-dev"
					}
				}
			]
		}`,
	)

	gock.New(uri).AddMatcher(get("/api/v1/namespaces/test-dev/pods/pasta/log")).MatchParam("container", "^container1$").Persist().Reply(200).BodyString(
		`pasta for life!`,
	)
	gock.New(uri).AddMatcher(get("/api/v1/namespaces/test-dev/pods/pasta/log")).MatchParam("container", "^container2$").Persist().Reply(200).BodyString(
		`pasta everyday!`,
	)

	gock.New(uri).AddMatcher(get("/apis/rbac.authorization.k8s.io/v1")).Persist().Reply(200).BodyString(
		`{
			"kind": "APIResourceList",
			"apiVersion": "v1",
			"groupVersion": "rbac.authorization.k8s.io/v1",
			"resources": [
				{
					"name": "rolebindings",
					"singularName": "rolebinding",
					"namespaced": true,
					"kind": "RoleBinding",
					"verbs": [
					  "create",
					  "delete",
					  "deletecollection",
					  "get",
					  "list",
					  "patch",
					  "update",
					  "watch"
					]
				 }
			]
		}`,
	)
	gock.New(uri).AddMatcher(get("/apis/rbac.authorization.k8s.io/v1/namespaces/test-dev/rolebindings")).Persist().Reply(200).BodyString(
		`{
			"apiVersion": "v1",
			"kind": "List",
			"items": [
				{
					"apiVersion": "authorization.openshift.io/v1",
					"kind": "RoleBinding",
					"metadata": {
						"name": "admin-foo",
						"namespace": "test-dev"
					},
					"roleRef": {
						"name": "admin"
					},
					"subjects": [
						{
							"kind": "Group",
							  "name": "foo"
						}
					]
				},
				{
					"apiVersion": "authorization.openshift.io/v1",
					"kind": "RoleBinding",
					"metadata": {
						"name": "viewer-foo",
						"namespace": "test-dev"
					},
					"roleRef": {
						"name": "viewer"
					},
					"subjects": [
						{
							"kind": "Group",
							  "name": "foo"
						}
					]
				}
			]
		}`,
	)
	gock.New(uri).AddMatcher(get("/api/v1/namespaces/test-dev/podtemplates")).Persist().Reply(403)

}

var fileContents = map[string]string{
	"pod-pasta.yaml": `apiVersion: v1
kind: Pod
metadata:
    name: pasta
    namespace: test-dev
status:
    containerStatuses:
        - containerID: cri-o://pasta
          image: pasta1:latest
          name: container1
          ready: true
          started: true
        - containerID: cri-o://pasta
          image: pasta2:latest
          name: container2
          ready: true
          started: true
`,
	"pod-pasta-container1.logs": `pasta for life!`,
	"pod-pasta-container2.logs": `pasta everyday!`,
	"pod-cookie.yaml": `apiVersion: v1
kind: Pod
metadata:
    name: cookie
    namespace: test-dev
`,
	"rolebinding-admin-foo.yaml": `apiVersion: authorization.openshift.io/v1
kind: RoleBinding
metadata:
    name: admin-foo
    namespace: test-dev
roleRef:
    name: admin
subjects:
    - kind: Group
      name: foo
`,
	"rolebinding-viewer-foo.yaml": `apiVersion: authorization.openshift.io/v1
kind: RoleBinding
metadata:
    name: viewer-foo
    namespace: test-dev
roleRef:
    name: viewer
subjects:
    - kind: Group
      name: foo
`,
}

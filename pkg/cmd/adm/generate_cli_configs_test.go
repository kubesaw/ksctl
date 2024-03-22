package adm

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"testing"

	"github.com/kubesaw/ksctl/pkg/client"
	"github.com/kubesaw/ksctl/pkg/configuration"
	. "github.com/kubesaw/ksctl/pkg/test"
	"github.com/stretchr/testify/assert"
	authv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"

	"github.com/h2non/gock"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestGenerateCliConfigs(t *testing.T) {
	// given
	require.NoError(t, client.AddToScheme())
	kubeSawAdmins := NewKubeSawAdmins(
		Clusters(HostServerAPI).
			AddMember("member1", Member1ServerAPI).
			AddMember("member2", Member2ServerAPI),
		ServiceAccounts(
			Sa("john", "",
				HostRoleBindings("toolchain-host-operator", Role("install-operator"), ClusterRole("admin")),
				MemberRoleBindings("toolchain-member-operator", Role("install-operator"), ClusterRole("admin"))),
			Sa("bob", "",
				HostRoleBindings("toolchain-host-operator", Role("restart=restart-deployment"), ClusterRole("restart=edit")),
				MemberRoleBindings("toolchain-member-operator", Role("restart=restart-deployment"), ClusterRole("restart=edit")))),
		Users())

	kubeSawAdminsContent, err := yaml.Marshal(kubeSawAdmins)
	require.NoError(t, err)
	kubeconfigFiles := createKubeconfigFiles(t, ksctlKubeconfigContent, ksctlKubeconfigContentMember2)

	setupGockForListServiceAccounts(t, HostServerAPI, configuration.Host)
	setupGockForListServiceAccounts(t, Member1ServerAPI, configuration.Member)
	setupGockForListServiceAccounts(t, Member2ServerAPI, configuration.Member)

	setupGockForServiceAccounts(t, HostServerAPI,
		newServiceAccount("sandbox-sre-host", "john"),
		newServiceAccount("sandbox-sre-host", "bob"),
	)
	setupGockForServiceAccounts(t, Member1ServerAPI,
		newServiceAccount("sandbox-sre-member", "john"),
		newServiceAccount("sandbox-sre-member", "bob"),
	)
	setupGockForServiceAccounts(t, Member2ServerAPI,
		newServiceAccount("sandbox-sre-member", "john"),
		newServiceAccount("sandbox-sre-member", "bob"),
	)
	t.Cleanup(gock.OffAll)

	configFile := createKubeSawAdminsFile(t, "kubesaw.host.openshiftapps.com", kubeSawAdminsContent)

	newExternalClient := func(config *rest.Config) (*rest.RESTClient, error) {
		return DefaultNewExternalClientFromConfig(config)
	}
	term := NewFakeTerminalWithResponse("Y")
	term.Tee(os.Stdout)

	t.Run("successful", func(t *testing.T) {
		t.Run("when there is host and two members", func(t *testing.T) {
			// given
			tempDir, err := os.MkdirTemp("", "sandbox-sre-out-")
			require.NoError(t, err)
			flags := generateFlags{kubeconfigs: kubeconfigFiles, kubeSawAdminsFile: configFile, outDir: tempDir}

			// when
			err = generate(term, flags, newExternalClient)

			// then
			require.NoError(t, err)

			verifyKsctlConfigFiles(t, tempDir, hasHost(), hasMember("member1", "member1"), hasMember("member2", "member2"))
		})

		t.Run("when there SAs are defined for host cluster only", func(t *testing.T) {
			// given
			saInHostOnly := NewKubeSawAdmins(
				Clusters(HostServerAPI).
					AddMember("member1", Member1ServerAPI).
					AddMember("member2", Member2ServerAPI),
				ServiceAccounts(
					Sa("john", "",
						HostRoleBindings("toolchain-host-operator", Role("install-operator"), ClusterRole("admin"))),
					Sa("bob", "",
						HostRoleBindings("toolchain-host-operator", Role("restart=restart-deployment"), ClusterRole("restart=edit")))),
				Users())
			kubeSawAdminsContent, err := yaml.Marshal(saInHostOnly)
			require.NoError(t, err)
			configFile := createKubeSawAdminsFile(t, "kubesaw.host.openshiftapps.com", kubeSawAdminsContent)
			tempDir, err := os.MkdirTemp("", "sandbox-sre-out-")
			require.NoError(t, err)
			flags := generateFlags{kubeconfigs: kubeconfigFiles, kubeSawAdminsFile: configFile, outDir: tempDir}

			// when
			err = generate(term, flags, newExternalClient)

			// then
			require.NoError(t, err)

			verifyKsctlConfigFiles(t, tempDir, hasHost())
		})

		t.Run("in dev mode", func(t *testing.T) {
			// given
			setupGockForListServiceAccounts(t, HostServerAPI, configuration.Member)
			setupGockForServiceAccounts(t, HostServerAPI,
				newServiceAccount("sandbox-sre-member", "john"),
				newServiceAccount("sandbox-sre-member", "bob"),
			)
			tempDir, err := os.MkdirTemp("", "sandbox-sre-out-")
			require.NoError(t, err)
			kubeconfigFiles := createKubeconfigFiles(t, ksctlKubeconfigContent)
			flags := generateFlags{kubeconfigs: kubeconfigFiles, kubeSawAdminsFile: configFile, outDir: tempDir, dev: true}

			// when
			err = generate(term, flags, newExternalClient)

			// then
			require.NoError(t, err)

			verifyKsctlConfigFiles(t, tempDir, hasHost(), hasMember("member1", "host"), hasMember("member2", "host"))
		})
	})

	t.Run("failed", func(t *testing.T) {
		t.Run("test buildClientFromKubeconfigFiles cannot build REST client", func(t *testing.T) {
			// given
			ctx := &generateContext{
				Terminal: NewFakeTerminalWithResponse("y"),
				newRESTClient: func(config *rest.Config) (*rest.RESTClient, error) {
					return nil, fmt.Errorf("some error")
				},
			}

			// when
			_, err := buildClientFromKubeconfigFiles(ctx, "https://dummy.openshift.com", kubeconfigFiles, sandboxSRENamespace(configuration.Host))

			// then
			require.Error(t, err)
			require.ErrorContains(t, err, "could not setup client from any of the provided kubeconfig files")
		})

		t.Run("test buildClientFromKubeconfigFiles cannot list service accounts", func(t *testing.T) {
			// given
			path := fmt.Sprintf("api/v1/namespaces/%s/serviceaccounts/", sandboxSRENamespace(configuration.Host))
			gock.New("https://dummy.openshift.com").Get(path).Persist().Reply(403)
			ctx := &generateContext{
				Terminal:        term,
				newRESTClient:   newExternalClient,
				kubeSawAdmins:   kubeSawAdmins,
				kubeconfigPaths: kubeconfigFiles,
			}

			// when
			_, err := buildClientFromKubeconfigFiles(ctx, "https://dummy.openshift.com", kubeconfigFiles, sandboxSRENamespace(configuration.Host))

			// then
			require.Error(t, err)
			require.ErrorContains(t, err, "could not setup client from any of the provided kubeconfig files")
		})

		t.Run("wrong kubesaw-admins.yaml file path", func(t *testing.T) {
			// given
			tempDir, err := os.MkdirTemp("", "sandbox-sre-out-")
			require.NoError(t, err)
			flags := generateFlags{kubeconfigs: kubeconfigFiles, kubeSawAdminsFile: "does/not/exist", outDir: tempDir}

			// when
			err = generate(term, flags, newExternalClient)

			// then
			require.Error(t, err)
			require.ErrorContains(t, err, "unable get kubesaw-admins.yaml file from does/not/exist")
		})

		t.Run("wrong kubeconfig file path", func(t *testing.T) {
			// given
			tempDir, err := os.MkdirTemp("", "sandbox-sre-out-")
			require.NoError(t, err)
			flags := generateFlags{kubeconfigs: []string{"does/not/exist"}, kubeSawAdminsFile: configFile, outDir: tempDir}

			// when
			err = generate(term, flags, newExternalClient)

			// then
			require.Error(t, err)
			require.ErrorContains(t, err, "could not setup client from any of the provided kubeconfig files")
		})

		t.Run("when token call is not mocked for SA", func(t *testing.T) {
			// given
			saInHostOnly := NewKubeSawAdmins(
				Clusters(HostServerAPI),
				ServiceAccounts(
					Sa("notmocked", "",
						HostRoleBindings("toolchain-host-operator", Role("install-operator"), ClusterRole("admin")))),
				Users())
			kubeSawAdminsContent, err := yaml.Marshal(saInHostOnly)
			require.NoError(t, err)
			configFile := createKubeSawAdminsFile(t, "sandbox.host.openshiftapps.com", kubeSawAdminsContent)
			tempDir, err := os.MkdirTemp("", "sandbox-sre-out-")
			require.NoError(t, err)
			flags := generateFlags{kubeconfigs: kubeconfigFiles, kubeSawAdminsFile: configFile, outDir: tempDir}

			// when
			err = generate(term, flags, newExternalClient)

			// then
			require.ErrorContains(t, err, "notmocked/token\": gock: cannot match any request")
		})
	})
}

func TestGetServiceAccountToken(t *testing.T) {

	// given
	require.NoError(t, client.AddToScheme())

	setupGockForServiceAccounts(t, "https://api.example.com", newServiceAccount("openshift-customer-monitoring", "loki"))
	t.Cleanup(gock.OffAll)
	cl, err := client.NewRESTClient("secret_token", "https://api.example.com")
	cl.Client.Transport = gock.DefaultTransport // make sure that the underlying client's request are intercepted by Gock
	// gock.Observe(gock.DumpRequest)
	require.NoError(t, err)
	// when
	actualToken, err := getServiceAccountToken(cl, types.NamespacedName{
		Namespace: "openshift-customer-monitoring",
		Name:      "loki",
	})

	// then
	require.NoError(t, err)
	assert.Equal(t, "token-secret-for-loki", actualToken) // `token-secret-for-loki` is the answered mock by Gock in `setupGockForServiceAccounts(...)`
}

func verifyKsctlConfigFiles(t *testing.T, tempDir string, clusterAssertions ...userConfigClusterAssertions) {
	tempDirInfo, err := os.ReadDir(tempDir)
	require.NoError(t, err)
	assert.Len(t, tempDirInfo, 2)
	for _, userDir := range tempDirInfo {
		require.True(t, userDir.IsDir())
		userDirInfo, err := os.ReadDir(path.Join(tempDir, userDir.Name()))
		require.NoError(t, err)

		assert.Len(t, userDirInfo, 1)
		assert.Equal(t, "ksctl.yaml", userDirInfo[0].Name())
		content, err := os.ReadFile(path.Join(tempDir, userDir.Name(), userDirInfo[0].Name()))
		require.NoError(t, err)

		ksctlConfig := configuration.KsctlConfig{}
		err = yaml.Unmarshal(content, &ksctlConfig)
		require.NoError(t, err)

		userConfig := assertKsctlConfig(t, ksctlConfig, userDir.Name()).
			hasNumberOfClusters(len(clusterAssertions))
		for _, applyAssertion := range clusterAssertions {
			applyAssertion(t, userDir.Name(), userConfig)
		}
	}
}

type userConfigClusterAssertions func(*testing.T, string, *ksctlConfigAssertion)

func hasHost() userConfigClusterAssertions {
	return func(t *testing.T, name string, assertion *ksctlConfigAssertion) {
		assertion.hasCluster("host", "host", configuration.Host)
	}
}

func hasMember(memberName, subDomain string) userConfigClusterAssertions {
	return func(t *testing.T, name string, assertion *ksctlConfigAssertion) {
		assertion.hasCluster(memberName, subDomain, configuration.Member)
	}
}

// KsctlConfig assertions

type ksctlConfigAssertion struct {
	t           *testing.T
	ksctlConfig configuration.KsctlConfig
	saBaseName  string
}

func assertKsctlConfig(t *testing.T, ksctlConfig configuration.KsctlConfig, saBaseName string) *ksctlConfigAssertion {
	require.NotNil(t, ksctlConfig)
	assert.Equal(t, saBaseName, ksctlConfig.Name)
	return &ksctlConfigAssertion{
		t:           t,
		ksctlConfig: ksctlConfig,
		saBaseName:  saBaseName,
	}
}

func (a *ksctlConfigAssertion) hasNumberOfClusters(number int) *ksctlConfigAssertion {
	require.Len(a.t, a.ksctlConfig.ClusterAccessDefinitions, number)
	return a
}

func (a *ksctlConfigAssertion) hasCluster(clusterName, subDomain string, clusterType configuration.ClusterType) {
	require.NotNil(a.t, a.ksctlConfig.ClusterAccessDefinitions[clusterName])

	assert.NotNil(a.t, a.ksctlConfig.ClusterAccessDefinitions[clusterName])
	assert.Equal(a.t, clusterType, a.ksctlConfig.ClusterAccessDefinitions[clusterName].ClusterType)
	assert.Equal(a.t, fmt.Sprintf("sandbox.%s.openshiftapps.com", subDomain), a.ksctlConfig.ClusterAccessDefinitions[clusterName].ServerName)
	assert.Equal(a.t, fmt.Sprintf("https://api.sandbox.%s.openshiftapps.com:6443", subDomain), a.ksctlConfig.ClusterAccessDefinitions[clusterName].ServerAPI)

	assert.Equal(a.t, fmt.Sprintf("token-secret-for-%s", a.saBaseName), a.ksctlConfig.ClusterAccessDefinitions[clusterName].Token)
}

func setupGockForListServiceAccounts(t *testing.T, apiEndpoint string, clusterType configuration.ClusterType) {
	resultServiceAccounts := &corev1.ServiceAccountList{
		TypeMeta: metav1.TypeMeta{},
		ListMeta: metav1.ListMeta{},
		Items: []corev1.ServiceAccount{
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: sandboxSRENamespace(clusterType),
					Name:      clusterType.String(),
				},
			},
		},
	}
	resultServiceAccountsStr, err := json.Marshal(resultServiceAccounts)
	require.NoError(t, err)
	path := fmt.Sprintf("api/v1/namespaces/%s/serviceaccounts/", sandboxSRENamespace(clusterType))
	t.Logf("mocking access to List %s/%s", apiEndpoint, path)
	gock.New(apiEndpoint).
		Get(path).
		Persist().
		Reply(200).
		BodyString(string(resultServiceAccountsStr))
}

func setupGockForServiceAccounts(t *testing.T, apiEndpoint string, sas ...*corev1.ServiceAccount) {
	for _, sa := range sas {
		expectedToken := "token-secret-for-" + sa.Name
		resultTokenRequest := &authv1.TokenRequest{
			Status: authv1.TokenRequestStatus{
				Token: expectedToken,
			},
		}
		resultTokenRequestStr, err := json.Marshal(resultTokenRequest)
		require.NoError(t, err)
		path := fmt.Sprintf("api/v1/namespaces/%s/serviceaccounts/%s/token", sa.Namespace, sa.Name)
		t.Logf("mocking access to POST %s/%s", apiEndpoint, path)
		gock.New(apiEndpoint).
			Post(path).
			Persist().
			Reply(200).
			BodyString(string(resultTokenRequestStr))
	}
}

func newServiceAccount(namespace, name string) *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
	}
}

package generate

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"testing"

	"github.com/h2non/gock"
	"github.com/kubesaw/ksctl/pkg/assets"
	"github.com/kubesaw/ksctl/pkg/client"
	"github.com/kubesaw/ksctl/pkg/configuration"
	. "github.com/kubesaw/ksctl/pkg/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
	authv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
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
				MemberRoleBindings("toolchain-member-operator", Role("install-operator"), ClusterRole("admin"))).
				WithSkippedMembers("member2"),
			Sa("jenny", "",
				HostRoleBindings("toolchain-host-operator", Role("restart-deployment"), ClusterRole("view")),
				MemberRoleBindings("toolchain-member-operator", Role("restart-deployment"), ClusterRole("view"))).
				WithSelectedMembers("member2"),
			Sa("bob", "",
				HostRoleBindings("toolchain-host-operator", Role("restart=restart-deployment"), ClusterRole("restart=edit")),
				MemberRoleBindings("toolchain-member-operator", Role("restart=restart-deployment"), ClusterRole("restart=edit")))),
		Users())
	kubeSawAdmins.DefaultServiceAccountsNamespace.Host = "kubesaw-sre-host"

	kubeSawAdminsContent, err := yaml.Marshal(kubeSawAdmins)
	require.NoError(t, err)
	kubeconfigFiles := createKubeconfigFiles(t, ksctlKubeconfigContent, ksctlKubeconfigContentMember2)

	setupGockForListServiceAccounts(t, kubeSawAdmins, HostServerAPI, configuration.Host)
	setupGockForListServiceAccounts(t, kubeSawAdmins, Member1ServerAPI, configuration.Member)
	setupGockForListServiceAccounts(t, kubeSawAdmins, Member2ServerAPI, configuration.Member)

	setupGockForServiceAccounts(t, HostServerAPI, 50,
		newServiceAccount("kubesaw-sre-host", "john"),
		newServiceAccount("kubesaw-sre-host", "bob"),
		newServiceAccount("kubesaw-sre-host", "jenny"),
	)
	setupGockForServiceAccounts(t, Member1ServerAPI, 50,
		newServiceAccount("kubesaw-admins-member", "john"),
		newServiceAccount("kubesaw-admins-member", "bob"),
	)
	setupGockForServiceAccounts(t, Member2ServerAPI, 50,
		newServiceAccount("kubesaw-admins-member", "bob"),
		newServiceAccount("kubesaw-admins-member", "jenny"),
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
			tempDir, err := os.MkdirTemp("", "ksctl-out-")
			require.NoError(t, err)
			flags := generateFlags{kubeconfigs: kubeconfigFiles, kubeSawAdminsFile: configFile, outDir: tempDir, tokenExpirationDays: 50}

			// when
			err = generate(term, flags, newExternalClient)

			// then
			require.NoError(t, err)

			verifyKsctlConfigFiles(t, tempDir,
				cliConfigForUser("john", hasHost(), hasMember("member1", "member1")),
				cliConfigForUser("bob", hasHost(), hasMember("member1", "member1"), hasMember("member2", "member2")),
				cliConfigForUser("jenny", hasHost(), hasMember("member2", "member2")))
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
			saInHostOnly.DefaultServiceAccountsNamespace.Host = "kubesaw-sre-host"
			kubeSawAdminsContent, err := yaml.Marshal(saInHostOnly)
			require.NoError(t, err)
			configFile := createKubeSawAdminsFile(t, "kubesaw.host.openshiftapps.com", kubeSawAdminsContent)
			tempDir, err := os.MkdirTemp("", "ksctl-out-")
			require.NoError(t, err)
			flags := generateFlags{kubeconfigs: kubeconfigFiles, kubeSawAdminsFile: configFile, outDir: tempDir, tokenExpirationDays: 50}

			// when
			err = generate(term, flags, newExternalClient)

			// then
			require.NoError(t, err)

			verifyKsctlConfigFiles(t, tempDir,
				cliConfigForUser("john", hasHost()),
				cliConfigForUser("bob", hasHost()))
		})

		t.Run("in dev mode", func(t *testing.T) {
			// given
			setupGockForListServiceAccounts(t, kubeSawAdmins, HostServerAPI, configuration.Member)
			setupGockForServiceAccounts(t, HostServerAPI, 50,
				newServiceAccount("kubesaw-admins-member", "john"),
				newServiceAccount("kubesaw-admins-member", "bob"),
				newServiceAccount("kubesaw-admins-member", "jenny"),
			)
			tempDir, err := os.MkdirTemp("", "ksctl-out-")
			require.NoError(t, err)
			kubeconfigFiles := createKubeconfigFiles(t, ksctlKubeconfigContent)
			flags := generateFlags{kubeconfigs: kubeconfigFiles, kubeSawAdminsFile: configFile, outDir: tempDir, dev: true, tokenExpirationDays: 50}

			// when
			err = generate(term, flags, newExternalClient)

			// then
			require.NoError(t, err)

			verifyKsctlConfigFiles(t, tempDir,
				cliConfigForUser("john", hasHost(), hasMember("member1", "host")),
				cliConfigForUser("bob", hasHost(), hasMember("member1", "host"), hasMember("member2", "host")),
				cliConfigForUser("jenny", hasHost(), hasMember("member2", "host")))
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
			_, err := buildClientFromKubeconfigFiles(ctx, "https://dummy.openshift.com", kubeconfigFiles, defaultSAsNamespace(kubeSawAdmins, configuration.Host))

			// then
			require.Error(t, err)
			require.ErrorContains(t, err, "could not setup client from any of the provided kubeconfig files")
		})

		t.Run("test buildClientFromKubeconfigFiles cannot list service accounts", func(t *testing.T) {
			// given
			path := fmt.Sprintf("api/v1/namespaces/%s/serviceaccounts/", defaultSAsNamespace(kubeSawAdmins, configuration.Host))
			gock.New("https://dummy.openshift.com").Get(path).Persist().Reply(403)
			ctx := &generateContext{
				Terminal:            term,
				newRESTClient:       newExternalClient,
				kubeSawAdmins:       kubeSawAdmins,
				kubeconfigPaths:     kubeconfigFiles,
				tokenExpirationDays: 365,
			}

			// when
			_, err := buildClientFromKubeconfigFiles(ctx, "https://dummy.openshift.com", kubeconfigFiles, defaultSAsNamespace(kubeSawAdmins, configuration.Host))

			// then
			require.Error(t, err)
			require.ErrorContains(t, err, "could not setup client from any of the provided kubeconfig files")
		})

		t.Run("wrong kubesaw-admins.yaml file path", func(t *testing.T) {
			// given
			tempDir, err := os.MkdirTemp("", "ksctl-out-")
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
			tempDir, err := os.MkdirTemp("", "ksctl-out-")
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
			saInHostOnly.DefaultServiceAccountsNamespace.Host = "kubesaw-sre-host"
			kubeSawAdminsContent, err := yaml.Marshal(saInHostOnly)
			require.NoError(t, err)
			configFile := createKubeSawAdminsFile(t, "kubesaw.host.openshiftapps.com", kubeSawAdminsContent)
			tempDir, err := os.MkdirTemp("", "ksctl-out-")
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

	setupGockForServiceAccounts(t, "https://api.example.com", 365, newServiceAccount("openshift-customer-monitoring", "loki"))
	t.Cleanup(gock.OffAll)
	cl, err := newGockRESTClient("secret_token", "https://api.example.com")
	require.NoError(t, err)
	// when
	actualToken, err := GetServiceAccountToken(cl, types.NamespacedName{
		Namespace: "openshift-customer-monitoring",
		Name:      "loki",
	}, 365)

	// then
	require.NoError(t, err)
	assert.Equal(t, "token-secret-for-loki", actualToken) // `token-secret-for-loki` is the answered mock by Gock in `setupGockForServiceAccounts(...)`
}

func newGockRESTClient(token, apiEndpoint string) (*rest.RESTClient, error) {
	config := &rest.Config{
		BearerToken: token,
		Host:        apiEndpoint,
		Transport:   gock.DefaultTransport, // make sure that the underlying client's request are intercepted by Gock
		ContentConfig: rest.ContentConfig{
			GroupVersion:         &authv1.SchemeGroupVersion,
			NegotiatedSerializer: scheme.Codecs,
		},
	}
	return rest.RESTClientFor(config)
}

type userAndClusterAssertions func() (string, []userConfigClusterAssertions)

func cliConfigForUser(name string, clusterAssertions ...userConfigClusterAssertions) userAndClusterAssertions {
	return func() (string, []userConfigClusterAssertions) {
		return name, clusterAssertions
	}
}

func verifyKsctlConfigFiles(t *testing.T, tempDir string, userAndClusterAssertionsPairs ...userAndClusterAssertions) {
	tempDirInfo, err := os.ReadDir(tempDir)
	require.NoError(t, err)
	assert.Len(t, tempDirInfo, len(userAndClusterAssertionsPairs))
	for _, userAndClusterAssertions := range userAndClusterAssertionsPairs {
		userName, configClusterAssertions := userAndClusterAssertions()
		userDirInfo, err := os.ReadDir(path.Join(tempDir, userName))
		require.NoError(t, err)

		assert.Len(t, userDirInfo, 1)
		assert.Equal(t, "ksctl.yaml", userDirInfo[0].Name())
		content, err := os.ReadFile(path.Join(tempDir, userName, userDirInfo[0].Name()))
		require.NoError(t, err)

		ksctlConfig := configuration.KsctlConfig{}
		err = yaml.Unmarshal(content, &ksctlConfig)
		require.NoError(t, err)

		userConfig := assertKsctlConfig(t, ksctlConfig, userName).
			hasNumberOfClusters(len(configClusterAssertions))
		for _, applyAssertion := range configClusterAssertions {
			applyAssertion(t, userName, userConfig)
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
	assert.Equal(a.t, fmt.Sprintf("kubesaw.%s.openshiftapps.com", subDomain), a.ksctlConfig.ClusterAccessDefinitions[clusterName].ServerName)
	assert.Equal(a.t, fmt.Sprintf("https://api.kubesaw.%s.openshiftapps.com:6443", subDomain), a.ksctlConfig.ClusterAccessDefinitions[clusterName].ServerAPI)

	assert.Equal(a.t, fmt.Sprintf("token-secret-for-%s", a.saBaseName), a.ksctlConfig.ClusterAccessDefinitions[clusterName].Token)
}

func setupGockForListServiceAccounts(t *testing.T, kubeSawAdmins *assets.KubeSawAdmins, apiEndpoint string, clusterType configuration.ClusterType) {
	resultServiceAccounts := &corev1.ServiceAccountList{
		TypeMeta: metav1.TypeMeta{},
		ListMeta: metav1.ListMeta{},
		Items: []corev1.ServiceAccount{
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: defaultSAsNamespace(kubeSawAdmins, clusterType),
					Name:      clusterType.String(),
				},
			},
		},
	}
	resultServiceAccountsStr, err := json.Marshal(resultServiceAccounts)
	require.NoError(t, err)
	path := fmt.Sprintf("api/v1/namespaces/%s/serviceaccounts/", defaultSAsNamespace(kubeSawAdmins, clusterType))
	t.Logf("mocking access to List %s/%s", apiEndpoint, path)
	gock.New(apiEndpoint).
		Get(path).
		Persist().
		Reply(200).
		BodyString(string(resultServiceAccountsStr))
}

func setupGockForServiceAccounts(t *testing.T, apiEndpoint string, tokenExpirationDays int, sas ...*corev1.ServiceAccount) {
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
			AddMatcher(func(request *http.Request, _ *gock.Request) (bool, error) {
				requestBody, err := io.ReadAll(request.Body)
				if err != nil {
					return false, err
				}
				if err := request.Body.Close(); err != nil {
					return false, err
				}
				tokenRequest := &authv1.TokenRequest{}
				if err := json.Unmarshal(requestBody, tokenRequest); err != nil {
					return false, err
				}
				fmt.Println(tokenRequest)
				expectedExpiry := int64(tokenExpirationDays * 24 * 60 * 60)
				if tokenRequest.Spec.ExpirationSeconds == nil {
					assert.NotEmpty(t, tokenRequest.Spec.ExpirationSeconds)
					return false, nil
				}
				if *tokenRequest.Spec.ExpirationSeconds != expectedExpiry {
					assert.Equal(t, expectedExpiry, *tokenRequest.Spec.ExpirationSeconds)
					return false, nil
				}
				return true, nil
			}).
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

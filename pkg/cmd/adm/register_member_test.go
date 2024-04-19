package adm

import (
	"context"
	"strings"
	"testing"
	"time"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/cluster"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/h2non/gock"
	"github.com/kubesaw/ksctl/pkg/client"
	"github.com/kubesaw/ksctl/pkg/configuration"
	. "github.com/kubesaw/ksctl/pkg/test"
	"github.com/kubesaw/ksctl/pkg/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

func TestRegisterMember(t *testing.T) {
	// given
	SetFileConfig(t, Host(), Member())
	hostKubeconfig := PersistKubeConfigFile(t, HostKubeConfig())
	memberKubeconfig := PersistKubeConfigFile(t, MemberKubeConfig())
	gock.New(AddClusterScriptDomain).
		Get(AddClusterScriptPath).
		Persist().
		Reply(200)
	defer gock.OffAll()

	var expectedHostArgs []string
	var expectedMemberArgs []string

	var counter int

	hostDeploymentName := test.NamespacedName("toolchain-host-operator", "host-operator-controller-manager")
	deployment := newDeployment(hostDeploymentName, 1)
	deployment.Labels = map[string]string{"olm.owner.namespace": "toolchain-host-operator"}

	// the command creator mocks the execution of the add-cluster.sh. We check that we're passing the correct arguments
	// and we also create the expected ToolchainCluster objects.
	commandCreator := func(cl runtimeclient.Client, hostReady, memberReady bool, nofPreexistingClustersOnEndpoint int) client.CommandCreator {
		return NewCommandCreator(t, "echo", "bash",
			func(t *testing.T, args ...string) {
				if counter == 0 {
					AssertFirstArgPrefixRestEqual("(.*)/add-cluster-(.*)", expectedHostArgs...)(t, args...)
					status := corev1.ConditionFalse
					if hostReady {
						status = corev1.ConditionTrue
					}
					// there's always at most 1 toolchain cluster for the host in the member cluster
					expectedHostToolchainClusterName, err := utils.GetToolchainClusterName("host", "https://cool-server.com", 0)
					require.NoError(t, err)
					expectedHostToolchainCluster := &toolchainv1alpha1.ToolchainCluster{
						ObjectMeta: metav1.ObjectMeta{
							Name:      expectedHostToolchainClusterName,
							Namespace: "toolchain-member-operator",
						},
						Spec: toolchainv1alpha1.ToolchainClusterSpec{
							APIEndpoint: "https://cool-server.com",
						},
						Status: toolchainv1alpha1.ToolchainClusterStatus{
							Conditions: []toolchainv1alpha1.ToolchainClusterCondition{
								{
									Type:   toolchainv1alpha1.ToolchainClusterReady,
									Status: status,
								},
							},
						},
					}
					require.NoError(t, cl.Create(context.TODO(), expectedHostToolchainCluster))
				} else {
					AssertFirstArgPrefixRestEqual("(.*)/add-cluster-(.*)", expectedMemberArgs...)(t, args...)
					status := corev1.ConditionFalse
					if memberReady {
						status = corev1.ConditionTrue
					}
					expectedMemberToolchainClusterName, err := utils.GetToolchainClusterName("member", "https://cool-server.com", nofPreexistingClustersOnEndpoint)
					require.NoError(t, err)
					expectedMemberToolchainCluster := &toolchainv1alpha1.ToolchainCluster{
						ObjectMeta: metav1.ObjectMeta{
							Name:      expectedMemberToolchainClusterName,
							Namespace: "toolchain-host-operator",
						},
						Spec: toolchainv1alpha1.ToolchainClusterSpec{
							APIEndpoint: "https://cool-server.com",
						},
						Status: toolchainv1alpha1.ToolchainClusterStatus{
							Conditions: []toolchainv1alpha1.ToolchainClusterCondition{
								{
									Type:   toolchainv1alpha1.ToolchainClusterReady,
									Status: status,
								},
							},
						},
					}
					require.NoError(t, cl.Create(context.TODO(), expectedMemberToolchainCluster))
				}
				counter++
			})
	}

	t.Run("produces valid example SPC", func(t *testing.T) {
		// given
		term := NewFakeTerminalWithResponse("Y")
		newClient, fakeClient := newFakeClientsFromRestConfig(t, deployment)
		ctx := newExtendedCommandContext(term, newClient)
		counter = 0
		addClusterCommand := commandCreator(fakeClient, true, true, 0)
		expectedHostArgs = []string{"--type", "host", "--host-kubeconfig", hostKubeconfig, "--host-ns", "toolchain-host-operator", "--member-kubeconfig", memberKubeconfig, "--member-ns", "toolchain-member-operator"}
		expectedMemberArgs = []string{"--type", "member", "--host-kubeconfig", hostKubeconfig, "--host-ns", "toolchain-host-operator", "--member-kubeconfig", memberKubeconfig, "--member-ns", "toolchain-member-operator"}

		expectedExampleSPC := &toolchainv1alpha1.SpaceProvisionerConfig{
			TypeMeta: metav1.TypeMeta{
				Kind:       "SpaceProvisionerConfig",
				APIVersion: toolchainv1alpha1.GroupVersion.Identifier(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "member-cool-server.com",
				Namespace: "toolchain-host-operator",
			},
			Spec: toolchainv1alpha1.SpaceProvisionerConfigSpec{
				ToolchainCluster: "member-cool-server.com",
				Enabled:          false,
				PlacementRoles: []string{
					cluster.RoleLabel(cluster.Tenant),
				},
			},
		}

		// when
		err := registerMemberCluster(ctx, addClusterCommand, hostKubeconfig, memberKubeconfig, false, 1*time.Second)

		// then
		require.NoError(t, err)
		assert.Equal(t, 2, counter)
		assert.Contains(t, term.Output(), "Modify and apply the following SpaceProvisionerConfig to the host cluster")
		actualExampleSPC := extractExampleSPCFromOutput(t, term.Output())
		assert.Equal(t, *expectedExampleSPC, actualExampleSPC)
	})

	t.Run("reports error when member ToolchainCluster is not ready in host", func(t *testing.T) {
		// given
		term := NewFakeTerminalWithResponse("Y")
		newClient, fakeClient := newFakeClientsFromRestConfig(t, deployment)
		ctx := newExtendedCommandContext(term, newClient)
		counter = 0
		addClusterCommand := commandCreator(fakeClient, true, false, 0)
		expectedHostArgs = []string{"--type", "host", "--host-kubeconfig", hostKubeconfig, "--host-ns", "toolchain-host-operator", "--member-kubeconfig", memberKubeconfig, "--member-ns", "toolchain-member-operator"}
		expectedMemberArgs = []string{"--type", "member", "--host-kubeconfig", hostKubeconfig, "--host-ns", "toolchain-host-operator", "--member-kubeconfig", memberKubeconfig, "--member-ns", "toolchain-member-operator"}

		// when
		err := registerMemberCluster(ctx, addClusterCommand, hostKubeconfig, memberKubeconfig, false, 1*time.Second)

		// then
		require.Error(t, err)
		assert.Equal(t, 2, counter)
		tcs := &toolchainv1alpha1.ToolchainClusterList{}
		require.NoError(t, fakeClient.List(context.TODO(), tcs, runtimeclient.InNamespace("toolchain-host-operator")))
		assert.Len(t, tcs.Items, 1)
		require.NoError(t, fakeClient.List(context.TODO(), tcs, runtimeclient.InNamespace("toolchain-member-operator")))
		assert.Len(t, tcs.Items, 1)
		assert.Contains(t, term.Output(), "The ToolchainCluster resource representing the member in the host cluster has not become ready.")
	})

	t.Run("reports error when host ToolchainCluster is not ready in member", func(t *testing.T) {
		// given
		term := NewFakeTerminalWithResponse("Y")
		newClient, fakeClient := newFakeClientsFromRestConfig(t, deployment)
		ctx := newExtendedCommandContext(term, newClient)
		counter = 0
		addClusterCommand := commandCreator(fakeClient, false, false, 0)
		expectedHostArgs = []string{"--type", "host", "--host-kubeconfig", hostKubeconfig, "--host-ns", "toolchain-host-operator", "--member-kubeconfig", memberKubeconfig, "--member-ns", "toolchain-member-operator"}
		expectedMemberArgs = []string{"--type", "member", "--host-kubeconfig", hostKubeconfig, "--host-ns", "toolchain-host-operator", "--member-kubeconfig", memberKubeconfig, "--member-ns", "toolchain-member-operator"}

		// when
		err := registerMemberCluster(ctx, addClusterCommand, hostKubeconfig, memberKubeconfig, false, 1*time.Second)

		// then
		require.Error(t, err)
		assert.Equal(t, 1, counter)
		tcs := &toolchainv1alpha1.ToolchainClusterList{}
		require.NoError(t, fakeClient.List(context.TODO(), tcs, runtimeclient.InNamespace("toolchain-host-operator")))
		assert.Empty(t, tcs.Items)
		assert.Contains(t, term.Output(), "The ToolchainCluster resource representing the host in the member cluster has not become ready.")
		require.NoError(t, fakeClient.List(context.TODO(), tcs, runtimeclient.InNamespace("toolchain-member-operator")))
		assert.Len(t, tcs.Items, 1)
	})

	t.Run("single toolchain in cluster", func(t *testing.T) {
		// given
		term := NewFakeTerminalWithResponse("Y")
		newClient, fakeClient := newFakeClientsFromRestConfig(t, deployment)
		ctx := newExtendedCommandContext(term, newClient)
		counter = 0
		addClusterCommand := commandCreator(fakeClient, true, true, 0)
		expectedHostArgs = []string{"--type", "host", "--host-kubeconfig", hostKubeconfig, "--host-ns", "toolchain-host-operator", "--member-kubeconfig", memberKubeconfig, "--member-ns", "toolchain-member-operator"}
		expectedMemberArgs = []string{"--type", "member", "--host-kubeconfig", hostKubeconfig, "--host-ns", "toolchain-host-operator", "--member-kubeconfig", memberKubeconfig, "--member-ns", "toolchain-member-operator"}

		// when
		err := registerMemberCluster(ctx, addClusterCommand, hostKubeconfig, memberKubeconfig, false, 1*time.Second)

		// then
		require.NoError(t, err)
		assert.Equal(t, 2, counter)
		assert.Contains(t, term.Output(), "Modify and apply the following SpaceProvisionerConfig to the host cluster")
		assert.Contains(t, term.Output(), "kind: SpaceProvisionerConfig")
	})

	t.Run("single toolchain in cluster with --lets-encrypt", func(t *testing.T) {
		// given
		term := NewFakeTerminalWithResponse("Y")
		newClient, fakeClient := newFakeClientsFromRestConfig(t, deployment)
		ctx := newExtendedCommandContext(term, newClient)
		counter = 0
		addClusterCommand := commandCreator(fakeClient, true, true, 0)
		expectedHostArgs = []string{"--type", "host", "--host-kubeconfig", hostKubeconfig, "--host-ns", "toolchain-host-operator", "--member-kubeconfig", memberKubeconfig, "--member-ns", "toolchain-member-operator", "--lets-encrypt"}
		expectedMemberArgs = []string{"--type", "member", "--host-kubeconfig", hostKubeconfig, "--host-ns", "toolchain-host-operator", "--member-kubeconfig", memberKubeconfig, "--member-ns", "toolchain-member-operator", "--lets-encrypt"}

		// when
		err := registerMemberCluster(ctx, addClusterCommand, hostKubeconfig, memberKubeconfig, true, 1*time.Second)

		// then
		require.NoError(t, err)
		assert.Equal(t, 2, counter)
		assert.Contains(t, term.Output(), "Modify and apply the following SpaceProvisionerConfig to the host cluster")
		assert.Contains(t, term.Output(), "kind: SpaceProvisionerConfig")
	})

	t.Run("multiple toolchains in cluster", func(t *testing.T) {
		// given
		term := NewFakeTerminalWithResponse("Y")
		newClient, fakeClient := newFakeClientsFromRestConfig(t, deployment)
		ctx := newExtendedCommandContext(term, newClient)
		counter = 0
		preexistingToolchainCluster := &toolchainv1alpha1.ToolchainCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "member-cool-server.com",
				Namespace: "toolchain-host-operator",
			},
			Spec: toolchainv1alpha1.ToolchainClusterSpec{
				APIEndpoint: "https://cool-server.com",
			},
			Status: toolchainv1alpha1.ToolchainClusterStatus{
				Conditions: []toolchainv1alpha1.ToolchainClusterCondition{
					{
						Type:   toolchainv1alpha1.ToolchainClusterReady,
						Status: corev1.ConditionTrue,
					},
				},
			},
		}
		require.NoError(t, fakeClient.Create(context.TODO(), preexistingToolchainCluster.DeepCopy()))
		preexistingToolchainCluster.Name = "member-cool-server.com1"
		require.NoError(t, fakeClient.Create(context.TODO(), preexistingToolchainCluster.DeepCopy()))

		addClusterCommand := commandCreator(fakeClient, true, true, 2)
		expectedHostArgs = []string{"--type", "host", "--host-kubeconfig", hostKubeconfig, "--host-ns", "toolchain-host-operator", "--member-kubeconfig", memberKubeconfig, "--member-ns", "toolchain-member-operator"}
		expectedMemberArgs = []string{"--type", "member", "--host-kubeconfig", hostKubeconfig, "--host-ns", "toolchain-host-operator", "--member-kubeconfig", memberKubeconfig, "--member-ns", "toolchain-member-operator", "--multi-member", "2"}

		// when
		err := registerMemberCluster(ctx, addClusterCommand, hostKubeconfig, memberKubeconfig, false, 1*time.Second)

		// then
		require.NoError(t, err)
		assert.Equal(t, 2, counter)
		assert.Contains(t, term.Output(), "Modify and apply the following SpaceProvisionerConfig to the host cluster")
		assert.Contains(t, term.Output(), "kind: SpaceProvisionerConfig")
		assert.Contains(t, term.Output(), "toolchainCluster: member-cool-server.com2")
	})

	t.Run("cannot register the same member twice", func(t *testing.T) {
		// given
		term1 := NewFakeTerminalWithResponse("Y")
		term2 := NewFakeTerminalWithResponse("Y")
		newClient, fakeClient := newFakeClientsFromRestConfig(t, deployment)
		ctx1 := newExtendedCommandContext(term1, newClient)
		ctx2 := newExtendedCommandContext(term2, newClient)
		counter = 0
		addClusterCommand := commandCreator(fakeClient, true, true, 0)
		expectedHostArgs = []string{"--type", "host", "--host-kubeconfig", hostKubeconfig, "--host-ns", "toolchain-host-operator", "--member-kubeconfig", memberKubeconfig, "--member-ns", "toolchain-member-operator"}
		expectedMemberArgs = []string{"--type", "member", "--host-kubeconfig", hostKubeconfig, "--host-ns", "toolchain-host-operator", "--member-kubeconfig", memberKubeconfig, "--member-ns", "toolchain-member-operator"}

		// when
		err1 := registerMemberCluster(ctx1, addClusterCommand, hostKubeconfig, memberKubeconfig, false, 1*time.Second)
		err2 := registerMemberCluster(ctx2, addClusterCommand, hostKubeconfig, memberKubeconfig, false, 1*time.Second)

		// then
		require.NoError(t, err1)
		assert.Equal(t, 2, counter)
		assert.Contains(t, term1.Output(), "Modify and apply the following SpaceProvisionerConfig to the host cluster")
		assert.Contains(t, term1.Output(), "kind: SpaceProvisionerConfig")

		require.Error(t, err2)
		assert.Equal(t, "the member cluster (https://cool-server.com) is already registered with some host cluster in namespace toolchain-member-operator", err2.Error())
	})
}

func TestRunAddClusterScriptSuccess(t *testing.T) {
	// given
	SetFileConfig(t, Host(), Member())
	hostKubeconfig := PersistKubeConfigFile(t, HostKubeConfig())
	memberKubeconfig := PersistKubeConfigFile(t, MemberKubeConfig())
	gock.New(AddClusterScriptDomain).
		Get(AddClusterScriptPath).
		Persist().
		Reply(200)
	defer gock.OffAll()
	term := NewFakeTerminalWithResponse("Y")

	test := func(t *testing.T, clusterType configuration.ClusterType, multiMember int, letEncrypt bool, additionalExpectedArgs ...string) {
		// given
		expArgs := []string{"--type", clusterType.String(), "--host-kubeconfig", hostKubeconfig, "--host-ns", "host-ns", "--member-kubeconfig", memberKubeconfig, "--member-ns", "member-ns"}
		expArgs = append(expArgs, additionalExpectedArgs...)
		ocCommandCreator := NewCommandCreator(t, "echo", "bash",
			AssertFirstArgPrefixRestEqual("(.*)/add-cluster-(.*)", expArgs...))

		// when
		err := runAddClusterScript(term, ocCommandCreator, clusterType, hostKubeconfig, "host-ns", memberKubeconfig, "member-ns", multiMember, letEncrypt)

		// then
		require.NoError(t, err)
		// on Linux, the output contains `Command to be called: bash /tmp/add-cluster-`
		// on macOS, the output contains something like `Command to be called: bash /var/folders/b8/wy8kq7_179l7yswz6gz6qx800000gp/T/add-cluster-369107288.sh`
		assert.Contains(t, term.Output(), "Command to be called: bash ")
		assert.Contains(t, term.Output(), "add-cluster-")
		assert.Contains(t, term.Output(), strings.Join(expArgs, " "))
	}

	for _, clusterType := range configuration.ClusterTypes {
		t.Run("for cluster name: "+clusterType.String(), func(t *testing.T) {
			t.Run("single toolchain in cluster", func(t *testing.T) {
				test(t, clusterType, 0, false)
			})
			t.Run("single toolchain in cluster with letsencrypt", func(t *testing.T) {
				test(t, clusterType, 0, true, "--lets-encrypt")
			})
			t.Run("multiple toolchains in cluster", func(t *testing.T) {
				test(t, clusterType, 5, false, "--multi-member", "5")
			})
			t.Run("multiple toolchains in cluster with letsencrypt", func(t *testing.T) {
				test(t, clusterType, 5, true, "--multi-member", "5", "--lets-encrypt")
			})
		})
	}
}

func TestRunAddClusterScriptFailed(t *testing.T) {
	// given
	SetFileConfig(t, Host(), Member())
	hostKubeconfig := PersistKubeConfigFile(t, HostKubeConfig())
	memberKubeconfig := PersistKubeConfigFile(t, MemberKubeConfig())
	gock.New(AddClusterScriptDomain).
		Get(AddClusterScriptPath).
		Persist().
		Reply(404)
	defer gock.OffAll()

	for _, clusterType := range configuration.ClusterTypes {
		t.Run("for cluster name: "+clusterType.String(), func(t *testing.T) {
			// given
			expArgs := []string{"--type", clusterType.String(), "--host-kubeconfig", hostKubeconfig, "--host-ns", "host-ns", "--member-kubeconfig", memberKubeconfig, "--member-ns", "member-ns", "--lets-encrypt"}
			ocCommandCreator := NewCommandCreator(t, "echo", "bash",
				AssertFirstArgPrefixRestEqual("(.*)/add-cluster-(.*)", expArgs...))
			term := NewFakeTerminalWithResponse("Y")

			// when
			err := runAddClusterScript(term, ocCommandCreator, clusterType, hostKubeconfig, "host-ns", memberKubeconfig, "member-ns", 0, true)

			// then
			require.Error(t, err)
			assert.NotContains(t, term.Output(), "Command to be called")
		})
	}
}

func whenDeploymentThenUpdated(t *testing.T, fakeClient *test.FakeClient, namespacedName types.NamespacedName, currentReplicas int32, numberOfUpdateCalls *int) func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.UpdateOption) error {
	return func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.UpdateOption) error {
		if deployment, ok := obj.(*appsv1.Deployment); ok {
			checkDeploymentBeingUpdated(t, fakeClient, namespacedName, currentReplicas, numberOfUpdateCalls, deployment)
		}
		return fakeClient.Client.Update(ctx, obj, opts...)
	}
}

func newFakeClientsFromRestConfig(t *testing.T, initObjs ...runtime.Object) (newClientFromRestConfigFunc, *test.FakeClient) {
	fakeClient := test.NewFakeClient(t, initObjs...)
	fakeClient.MockCreate = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.CreateOption) error {
		return fakeClient.Client.Create(ctx, obj, opts...)
	}
	fakeClient.MockUpdate = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.UpdateOption) error {
		return fakeClient.Client.Update(ctx, obj, opts...)
	}
	return func(cfg *rest.Config) (runtimeclient.Client, error) {
			assert.Contains(t, cfg.Host, "http")
			assert.Contains(t, cfg.Host, "://")
			assert.Contains(t, cfg.Host, ".com")
			return fakeClient, nil
		},
		fakeClient
}

func extractExampleSPCFromOutput(t *testing.T, output string) toolchainv1alpha1.SpaceProvisionerConfig {
	t.Helper()

	// the example is the last thing in the output, separated by an empty line
	// the output ends with an empty line, so we need to look for the second last one.
	emptyLineIdx := strings.LastIndex(output[0:len(output)-2], "\n\n")

	require.GreaterOrEqual(t, emptyLineIdx, 0)

	spc := toolchainv1alpha1.SpaceProvisionerConfig{}

	spcYaml := output[emptyLineIdx+2:]
	err := yaml.Unmarshal([]byte(spcYaml), &spc)
	require.NoError(t, err)
	return spc
}

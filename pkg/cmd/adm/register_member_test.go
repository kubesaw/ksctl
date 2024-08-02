package adm

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/cluster"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/ghodss/yaml"
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
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/client-go/util/homedir"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func TestRegisterMember(t *testing.T) {
	// given
	SetFileConfig(t, Host(), Member())
	hostKubeconfig := PersistKubeConfigFile(t, HostKubeConfig())
	memberKubeconfig := PersistKubeConfigFile(t, MemberKubeConfig())

	hostDeploymentName := test.NamespacedName("toolchain-host-operator", "host-operator-controller-manager")
	deployment := newDeployment(hostDeploymentName, 1)
	deployment.Labels = map[string]string{"olm.owner.namespace": "toolchain-host-operator"}
	toolchainClusterMemberSa := corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "toolchaincluster-member",
			Namespace: "toolchain-member-operator",
		},
	}
	toolchainClusterHostSa := corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "toolchaincluster-host",
			Namespace: "toolchain-host-operator",
		},
	}

	test.SetupGockForServiceAccounts(t, "https://cool-server.com",
		types.NamespacedName{Name: toolchainClusterMemberSa.Name, Namespace: toolchainClusterMemberSa.Namespace},
		types.NamespacedName{Namespace: toolchainClusterHostSa.Namespace, Name: toolchainClusterHostSa.Name},
	)
	hostToolchainClusterName, err := utils.GetToolchainClusterName(string(configuration.Host), "https://cool-server.com", "")
	require.NoError(t, err)
	memberToolchainClusterName, err := utils.GetToolchainClusterName(string(configuration.Member), "https://cool-server.com", "")
	require.NoError(t, err)

	t.Run("produces valid example SPC", func(t *testing.T) {
		// given
		term := NewFakeTerminalWithResponse("Y")
		newClient, fakeClient := newFakeClientsFromRestConfig(t, deployment, &toolchainClusterMemberSa, &toolchainClusterHostSa)
		// force the ready condition on the toolchaincluster created ( this is done by the tc controller in prod env )
		mockCreateToolchainClusterWithReadyCondition(fakeClient)
		ctx := newExtendedCommandContext(term, newClient)

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
		err := registerMemberCluster(ctx, 1*time.Second, newRegisterMemberArgsWith(hostKubeconfig, memberKubeconfig, false))

		// then
		require.NoError(t, err)
		// check the expected secrets are there with the kubeconfigs
		// the member kubeconfig secret in the host namespace
		verifyToolchainClusterSecret(t, fakeClient, "toolchain-host-operator", "toolchain-member-operator", memberToolchainClusterName)
		// the host secret in the member namespace
		verifyToolchainClusterSecret(t, fakeClient, "toolchain-member-operator", "toolchain-host-operator", hostToolchainClusterName)
		tcs := &toolchainv1alpha1.ToolchainClusterList{}
		require.NoError(t, fakeClient.List(context.TODO(), tcs, runtimeclient.InNamespace("toolchain-host-operator")))
		assert.Len(t, tcs.Items, 1)
		require.NoError(t, fakeClient.List(context.TODO(), tcs, runtimeclient.InNamespace("toolchain-member-operator")))
		assert.Len(t, tcs.Items, 1)
		assert.Contains(t, term.Output(), "Modify and apply the following SpaceProvisionerConfig to the host cluster")
		actualExampleSPC := extractExampleSPCFromOutput(t, term.Output())
		assert.Equal(t, *expectedExampleSPC, actualExampleSPC)
	})

	t.Run("reports error when member ToolchainCluster is not ready in host", func(t *testing.T) {
		// given
		term := NewFakeTerminalWithResponse("Y")
		newClient, fakeClient := newFakeClientsFromRestConfig(t, deployment, &toolchainClusterMemberSa, &toolchainClusterHostSa)
		mockCreateToolchainClusterInNamespaceWithReadyCondition(fakeClient, "toolchain-member-operator") // we set to ready only the host toolchaincluster in member operator namespace
		ctx := newExtendedCommandContext(term, newClient)

		// when
		err := registerMemberCluster(ctx, 1*time.Second, newRegisterMemberArgsWith(hostKubeconfig, memberKubeconfig, false))

		// then
		require.Error(t, err)
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
		newClient, fakeClient := newFakeClientsFromRestConfig(t, deployment, &toolchainClusterMemberSa, &toolchainClusterHostSa)
		mockCreateToolchainClusterInNamespaceWithReadyCondition(fakeClient, "toolchain-host-operator") // set to ready only the member toolchaincluster in host operator namespace
		ctx := newExtendedCommandContext(term, newClient)

		// when
		err := registerMemberCluster(ctx, 1*time.Second, newRegisterMemberArgsWith(hostKubeconfig, memberKubeconfig, false))

		// then
		require.Error(t, err)
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
		newClient, fakeClient := newFakeClientsFromRestConfig(t, deployment, &toolchainClusterMemberSa, &toolchainClusterHostSa)
		mockCreateToolchainClusterWithReadyCondition(fakeClient)
		ctx := newExtendedCommandContext(term, newClient)

		// when
		err := registerMemberCluster(ctx, 1*time.Second, newRegisterMemberArgsWith(hostKubeconfig, memberKubeconfig, false))

		// then
		require.NoError(t, err)
		assert.Contains(t, term.Output(), "Modify and apply the following SpaceProvisionerConfig to the host cluster")
		assert.Contains(t, term.Output(), "kind: SpaceProvisionerConfig")
	})

	t.Run("single toolchain in cluster with --lets-encrypt", func(t *testing.T) {
		// given
		term := NewFakeTerminalWithResponse("Y")
		newClient, fakeClient := newFakeClientsFromRestConfig(t, deployment, &toolchainClusterMemberSa, &toolchainClusterHostSa)
		mockCreateToolchainClusterWithReadyCondition(fakeClient)
		ctx := newExtendedCommandContext(term, newClient)

		// when
		err := registerMemberCluster(ctx, 1*time.Second, newRegisterMemberArgsWith(hostKubeconfig, memberKubeconfig, true))

		// then
		require.NoError(t, err)
		assert.Contains(t, term.Output(), "Modify and apply the following SpaceProvisionerConfig to the host cluster")
		assert.Contains(t, term.Output(), "kind: SpaceProvisionerConfig")
	})

	t.Run("multiple toolchains in cluster", func(t *testing.T) {
		// given
		term := NewFakeTerminalWithResponse("Y")
		newClient, fakeClient := newFakeClientsFromRestConfig(t, deployment, &toolchainClusterMemberSa, &toolchainClusterHostSa)
		mockCreateToolchainClusterWithReadyCondition(fakeClient)
		ctx := newExtendedCommandContext(term, newClient)
		preexistingToolchainCluster := &toolchainv1alpha1.ToolchainCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "member-cool-server.com",
				Namespace: "toolchain-host-operator",
			},
			Spec: toolchainv1alpha1.ToolchainClusterSpec{
				APIEndpoint: "https://cool-server.com",
			},
			Status: toolchainv1alpha1.ToolchainClusterStatus{
				Conditions: []toolchainv1alpha1.Condition{
					{
						Type:   toolchainv1alpha1.ConditionReady,
						Status: corev1.ConditionTrue,
					},
				},
			},
		}
		require.NoError(t, fakeClient.Create(context.TODO(), preexistingToolchainCluster.DeepCopy()))
		preexistingToolchainCluster.Name = "member-cool-server.com1"
		require.NoError(t, fakeClient.Create(context.TODO(), preexistingToolchainCluster.DeepCopy()))

		// when
		err := registerMemberCluster(ctx, 1*time.Second, newRegisterMemberArgsWithSuffix(hostKubeconfig, memberKubeconfig, false, "2"))

		// then
		require.NoError(t, err)
		assert.Contains(t, term.Output(), "Modify and apply the following SpaceProvisionerConfig to the host cluster")
		assert.Contains(t, term.Output(), "kind: SpaceProvisionerConfig")
		assert.Contains(t, term.Output(), "toolchainCluster: member-cool-server.com2")
	})

	t.Run("cannot register the same member twice with different names", func(t *testing.T) {
		// given
		term1 := NewFakeTerminalWithResponse("Y")
		term2 := NewFakeTerminalWithResponse("Y")
		newClient, fakeClient := newFakeClientsFromRestConfig(t, deployment, &toolchainClusterMemberSa, &toolchainClusterHostSa)
		mockCreateToolchainClusterWithReadyCondition(fakeClient)
		ctx1 := newExtendedCommandContext(term1, newClient)
		ctx2 := newExtendedCommandContext(term2, newClient)

		// when
		err1 := registerMemberCluster(ctx1, 1*time.Second, newRegisterMemberArgsWith(hostKubeconfig, memberKubeconfig, false))
		err2 := registerMemberCluster(ctx2, 1*time.Second, newRegisterMemberArgsWithSuffix(hostKubeconfig, memberKubeconfig, false, "1"))

		// then
		require.NoError(t, err1)
		assert.Contains(t, term1.Output(), "Modify and apply the following SpaceProvisionerConfig to the host cluster")
		assert.Contains(t, term1.Output(), "kind: SpaceProvisionerConfig")

		require.Error(t, err2)
		assert.Equal(t, `Cannot proceed because of the following problems:
	- the newly registered member cluster would have a different name (member-cool-server.com1) than the already existing one (member-cool-server.com) which would lead to invalid configuration. Consider using the --name-suffix parameter to match the existing member registration if you intend to just update it instead of creating a new registration`, err2.Error())
	})

	t.Run("warns when updating existing registration", func(t *testing.T) {
		// given
		term1 := NewFakeTerminalWithResponse("Y")
		term2 := NewFakeTerminalWithResponse("Y")
		newClient, fakeClient := newFakeClientsFromRestConfig(t, deployment, &toolchainClusterMemberSa, &toolchainClusterHostSa)
		mockCreateToolchainClusterWithReadyCondition(fakeClient)
		ctx1 := newExtendedCommandContext(term1, newClient)
		ctx2 := newExtendedCommandContext(term2, newClient)

		// when
		err1 := registerMemberCluster(ctx1, 1*time.Second, newRegisterMemberArgsWith(hostKubeconfig, memberKubeconfig, false))
		err2 := registerMemberCluster(ctx2, 1*time.Second, newRegisterMemberArgsWithSuffix(hostKubeconfig, memberKubeconfig, false, ""))

		// then
		require.NoError(t, err1)
		assert.Contains(t, term1.Output(), "Modify and apply the following SpaceProvisionerConfig to the host cluster")
		assert.Contains(t, term1.Output(), "kind: SpaceProvisionerConfig")

		require.NoError(t, err2)
		assert.Contains(t, term2.Output(), "Modify and apply the following SpaceProvisionerConfig to the host cluster")
		assert.Contains(t, term2.Output(), "kind: SpaceProvisionerConfig")
		assert.Contains(t, term2.Output(), "Please confirm that the following is ok and you are willing to proceed:")
		assert.Contains(t, term2.Output(), "- there already is a registered member for the same member API endpoint and operator namespace")
	})

	t.Run("Errors when member already registered with multiple hosts", func(t *testing.T) {
		// given
		term := NewFakeTerminalWithResponse("Y")
		newClient, fakeClient := newFakeClientsFromRestConfig(t, deployment, &toolchainClusterMemberSa, &toolchainClusterHostSa)
		mockCreateToolchainClusterWithReadyCondition(fakeClient)
		ctx := newExtendedCommandContext(term, newClient)
		preexistingToolchainCluster1 := &toolchainv1alpha1.ToolchainCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "host-not-so-cool-server.com",
				Namespace: "toolchain-member-operator",
			},
			Spec: toolchainv1alpha1.ToolchainClusterSpec{
				APIEndpoint: "https://not-so-cool-server.com",
			},
			Status: toolchainv1alpha1.ToolchainClusterStatus{
				Conditions: []toolchainv1alpha1.Condition{
					{
						Type:   toolchainv1alpha1.ConditionReady,
						Status: corev1.ConditionTrue,
					},
				},
			},
		}
		preexistingToolchainCluster2 := &toolchainv1alpha1.ToolchainCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "host-uncool-server.com",
				Namespace: "toolchain-member-operator",
			},
			Spec: toolchainv1alpha1.ToolchainClusterSpec{
				APIEndpoint: "https://uncool-server.com",
			},
			Status: toolchainv1alpha1.ToolchainClusterStatus{
				Conditions: []toolchainv1alpha1.Condition{
					{
						Type:   toolchainv1alpha1.ConditionReady,
						Status: corev1.ConditionTrue,
					},
				},
			},
		}
		require.NoError(t, fakeClient.Create(context.TODO(), preexistingToolchainCluster1.DeepCopy()))
		require.NoError(t, fakeClient.Create(context.TODO(), preexistingToolchainCluster2.DeepCopy()))

		// when
		err := registerMemberCluster(ctx, 1*time.Second, newRegisterMemberArgsWith(hostKubeconfig, memberKubeconfig, false))

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), `Cannot proceed because of the following problems:
	- member misconfigured: the member cluster (https://cool-server.com) is already registered with more than 1 host in namespace toolchain-member-operator`)
		assert.NotContains(t, term.Output(), "kind: SpaceProvisionerConfig")
	})

	t.Run("Errors when registering into another host", func(t *testing.T) {
		// given
		term := NewFakeTerminalWithResponse("Y")
		newClient, fakeClient := newFakeClientsFromRestConfig(t, deployment)
		ctx := newExtendedCommandContext(term, newClient)
		preexistingToolchainCluster := &toolchainv1alpha1.ToolchainCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "host-not-so-cool-server.com",
				Namespace: "toolchain-member-operator",
			},
			Spec: toolchainv1alpha1.ToolchainClusterSpec{
				APIEndpoint: "https://not-so-cool-server.com",
			},
			Status: toolchainv1alpha1.ToolchainClusterStatus{
				Conditions: []toolchainv1alpha1.Condition{
					{
						Type:   toolchainv1alpha1.ConditionReady,
						Status: corev1.ConditionTrue,
					},
				},
			},
		}
		require.NoError(t, fakeClient.Create(context.TODO(), preexistingToolchainCluster.DeepCopy()))

		// when
		err := registerMemberCluster(ctx, 1*time.Second, newRegisterMemberArgsWith(hostKubeconfig, memberKubeconfig, false))

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), `Cannot proceed because of the following problems:
	- the member is already registered with another host (https://not-so-cool-server.com) so registering it with the new one (https://cool-server.com) would result in an invalid configuration`)
		assert.NotContains(t, term.Output(), "kind: SpaceProvisionerConfig")
	})

	t.Run("Errors when host with different name already exists", func(t *testing.T) {
		// given
		term := NewFakeTerminalWithResponse("Y")
		newClient, fakeClient := newFakeClientsFromRestConfig(t, deployment)
		ctx := newExtendedCommandContext(term, newClient)
		preexistingToolchainCluster := &toolchainv1alpha1.ToolchainCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "host-with-weird-name",
				Namespace: "toolchain-member-operator",
			},
			Spec: toolchainv1alpha1.ToolchainClusterSpec{
				APIEndpoint: "https://cool-server.com",
			},
			Status: toolchainv1alpha1.ToolchainClusterStatus{
				Conditions: []toolchainv1alpha1.Condition{
					{
						Type:   toolchainv1alpha1.ConditionReady,
						Status: corev1.ConditionTrue,
					},
				},
			},
		}
		require.NoError(t, fakeClient.Create(context.TODO(), preexistingToolchainCluster.DeepCopy()))

		// when
		err := registerMemberCluster(ctx, 1*time.Second, newRegisterMemberArgsWith(hostKubeconfig, memberKubeconfig, false))

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), `Cannot proceed because of the following problems:
	- the host is already in the member namespace using a ToolchainCluster object with the name 'host-with-weird-name' but the new registration would use a ToolchainCluster with the name 'host-cool-server.com' which would lead to an invalid configuration`)
		assert.NotContains(t, term.Output(), "kind: SpaceProvisionerConfig")
	})

	t.Run("Errors when member with different name already exists", func(t *testing.T) {
		// given
		term := NewFakeTerminalWithResponse("Y")
		newClient, fakeClient := newFakeClientsFromRestConfig(t, deployment)
		ctx := newExtendedCommandContext(term, newClient)
		preexistingToolchainCluster := &toolchainv1alpha1.ToolchainCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "member-with-weird-name",
				Namespace: "toolchain-host-operator",
				Labels: map[string]string{
					"namespace": "toolchain-member-operator",
				},
			},
			Spec: toolchainv1alpha1.ToolchainClusterSpec{
				APIEndpoint: "https://cool-server.com",
			},
			Status: toolchainv1alpha1.ToolchainClusterStatus{
				Conditions: []toolchainv1alpha1.Condition{
					{
						Type:   toolchainv1alpha1.ConditionReady,
						Status: corev1.ConditionTrue,
					},
				},
			},
		}
		require.NoError(t, fakeClient.Create(context.TODO(), preexistingToolchainCluster.DeepCopy()))

		// when
		err := registerMemberCluster(ctx, 1*time.Second, newRegisterMemberArgsWith(hostKubeconfig, memberKubeconfig, false))

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), `Cannot proceed because of the following problems:
	- the newly registered member cluster would have a different name (member-cool-server.com) than the already existing one (member-with-weird-name) which would lead to invalid configuration. Consider using the --name-suffix parameter to match the existing member registration if you intend to just update it instead of creating a new registration`)
		assert.NotContains(t, term.Output(), "kind: SpaceProvisionerConfig")
	})

	t.Run("reports error when member toolchaincluster ServiceAccount is not there", func(t *testing.T) {
		// given
		term := NewFakeTerminalWithResponse("Y")
		newClient, fakeClient := newFakeClientsFromRestConfig(t, deployment, &toolchainClusterHostSa) // we pre-provision only the host toolchaincluster ServiceAccount
		ctx := newExtendedCommandContext(term, newClient)

		// when
		err := registerMemberCluster(ctx, 1*time.Second, newRegisterMemberArgsWith(hostKubeconfig, memberKubeconfig, false))

		// then
		require.Error(t, err)
		assert.Contains(t, term.Output(), "The toolchaincluster-member ServiceAccount in the member cluster is not present.")
		tcs := &toolchainv1alpha1.ToolchainClusterList{}
		require.NoError(t, fakeClient.List(context.TODO(), tcs, runtimeclient.InNamespace("toolchain-host-operator")))
		assert.Empty(t, tcs.Items)
		require.NoError(t, fakeClient.List(context.TODO(), tcs, runtimeclient.InNamespace("toolchain-member-operator")))
		assert.Empty(t, tcs.Items)
	})

	t.Run("reports error when host toolchaincluster ServiceAccount is not there", func(t *testing.T) {
		// given
		term := NewFakeTerminalWithResponse("Y")
		newClient, fakeClient := newFakeClientsFromRestConfig(t, deployment, &toolchainClusterMemberSa)  // we pre-provision only the member toolchaincluster ServiceAccount
		mockCreateToolchainClusterInNamespaceWithReadyCondition(fakeClient, "toolchain-member-operator") // set to ready only the host toolchaincluster in member operator namespace
		ctx := newExtendedCommandContext(term, newClient)

		// when
		err := registerMemberCluster(ctx, 1*time.Second, newRegisterMemberArgsWith(hostKubeconfig, memberKubeconfig, false))

		// then
		require.Error(t, err)
		assert.Contains(t, term.Output(), "The toolchaincluster-host ServiceAccount in the host cluster is not present.")
		tcs := &toolchainv1alpha1.ToolchainClusterList{}
		require.NoError(t, fakeClient.List(context.TODO(), tcs, runtimeclient.InNamespace("toolchain-host-operator")))
		assert.Empty(t, tcs.Items)
		require.NoError(t, fakeClient.List(context.TODO(), tcs, runtimeclient.InNamespace("toolchain-member-operator")))
		assert.Len(t, tcs.Items, 1)
	})
}

func mockCreateToolchainClusterInNamespaceWithReadyCondition(fakeClient *test.FakeClient, namespace string) {
	fakeClient.MockCreate = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.CreateOption) error {
		if obj, ok := obj.(*toolchainv1alpha1.ToolchainCluster); ok {
			// force the ready condition on the member toolchaincluster in host cluster only
			if obj.GetNamespace() == namespace {
				obj.Status = toolchainv1alpha1.ToolchainClusterStatus{
					Conditions: []toolchainv1alpha1.Condition{
						{
							Type:   toolchainv1alpha1.ConditionReady,
							Status: corev1.ConditionTrue,
						},
					},
				}
			}
		}
		return fakeClient.Client.Create(ctx, obj, opts...)
	}
}

func mockCreateToolchainClusterWithReadyCondition(fakeClient *test.FakeClient) {
	fakeClient.MockCreate = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.CreateOption) error {
		if obj, ok := obj.(*toolchainv1alpha1.ToolchainCluster); ok {
			obj.Status = toolchainv1alpha1.ToolchainClusterStatus{
				Conditions: []toolchainv1alpha1.Condition{
					{
						Type:   toolchainv1alpha1.ConditionReady,
						Status: corev1.ConditionTrue,
					},
				},
			}
		}
		return fakeClient.Client.Create(ctx, obj, opts...)
	}
}

func verifyToolchainClusterSecret(t *testing.T, fakeClient *test.FakeClient, secretNamespace, accessNamespace, tcName string) {
	secrets := &corev1.SecretList{}
	require.NoError(t, fakeClient.List(context.TODO(), secrets, runtimeclient.InNamespace(secretNamespace)))
	assert.Len(t, secrets.Items, 1)
	assert.NotEmpty(t, secrets.Items[0].Labels)
	assert.Equal(t, tcName, secrets.Items[0].Labels[toolchainv1alpha1.ToolchainClusterLabel])
	assert.NotEmpty(t, secrets.Items[0].StringData["kubeconfig"])
	apiConfig, err := clientcmd.Load([]byte(secrets.Items[0].StringData["kubeconfig"]))
	require.NoError(t, err)
	require.False(t, api.IsConfigEmpty(apiConfig))
	assert.Equal(t, "https://cool-server.com", apiConfig.Clusters["cluster"].Server)
	assert.True(t, apiConfig.Clusters["cluster"].InsecureSkipTLSVerify) // by default the insecure flag is being set
	assert.Equal(t, "cluster", apiConfig.Contexts["ctx"].Cluster)
	assert.Equal(t, accessNamespace, apiConfig.Contexts["ctx"].Namespace)
	assert.NotEmpty(t, apiConfig.AuthInfos["auth"].Token)
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
	}, fakeClient
}

func extractExampleSPCFromOutput(t *testing.T, output string) toolchainv1alpha1.SpaceProvisionerConfig {
	t.Helper()

	// the example is the last thing in the output, separated by an empty line
	// the output ends with an empty line, so we need to look for the second last one.
	afterObjectIdx := strings.LastIndex(output, "\n-------")
	beforeObjectIdx := strings.LastIndex(output[0:afterObjectIdx], "-------\n")

	require.GreaterOrEqual(t, afterObjectIdx, 0)
	require.GreaterOrEqual(t, beforeObjectIdx, 0)
	require.GreaterOrEqual(t, afterObjectIdx, beforeObjectIdx)

	spc := toolchainv1alpha1.SpaceProvisionerConfig{}

	spcYaml := output[beforeObjectIdx+8 : afterObjectIdx]
	err := yaml.Unmarshal([]byte(spcYaml), &spc)
	require.NoError(t, err)
	return spc
}

func newRegisterMemberArgsWith(hostKubeconfig, memberKubeconfig string, useLetsEncrypt bool) registerMemberArgs {
	args := defaultRegisterMemberArgs()
	args.hostKubeConfig = hostKubeconfig
	args.memberKubeConfig = memberKubeconfig
	args.useLetsEncrypt = useLetsEncrypt
	return args
}

func newRegisterMemberArgsWithSuffix(hostKubeconfig, memberKubeconfig string, useLetsEncrypt bool, nameSuffix string) registerMemberArgs {
	args := defaultRegisterMemberArgs()
	args.hostKubeConfig = hostKubeconfig
	args.memberKubeConfig = memberKubeconfig
	args.useLetsEncrypt = useLetsEncrypt
	args.nameSuffix = nameSuffix
	return args
}

func defaultRegisterMemberArgs() registerMemberArgs {
	// keep these values in sync with the values in NewRegisterMemberCmd() function
	args := registerMemberArgs{}

	defaultKubeConfigPath := ""
	if home := homedir.HomeDir(); home != "" {
		defaultKubeConfigPath = filepath.Join(home, ".kube", "config")
	}

	args.hostKubeConfig = defaultKubeConfigPath
	args.memberKubeConfig = defaultKubeConfigPath
	args.hostNamespace = "toolchain-host-operator"
	args.memberNamespace = "toolchain-member-operator"
	args.useLetsEncrypt = true

	return args
}

package adm

import (
	"context"
	"strings"
	"testing"
	"time"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/cluster"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/ghodss/yaml"
	"github.com/h2non/gock"
	"github.com/kubesaw/ksctl/pkg/client"
	"github.com/kubesaw/ksctl/pkg/configuration"
	. "github.com/kubesaw/ksctl/pkg/test"
	"github.com/kubesaw/ksctl/pkg/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
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
	commandCreator := func(cl runtimeclient.Client, hostReady, memberReady bool, nameSuffix string, allowUpdates bool) client.CommandCreator {
		return NewCommandCreator(t, "echo", "bash",
			func(t *testing.T, args ...string) {
				t.Helper()
				persist := func(tc *toolchainv1alpha1.ToolchainCluster) {
					t.Helper()
					if allowUpdates {
						if err := cl.Create(context.TODO(), tc); err != nil {
							if errors.IsAlreadyExists(err) {
								current := &toolchainv1alpha1.ToolchainCluster{}
								require.NoError(t, cl.Get(context.TODO(), runtimeclient.ObjectKeyFromObject(tc), current))
								current.Spec = tc.Spec
								current.Status = tc.Status
								require.NoError(t, cl.Update(context.TODO(), current))
							} else {
								require.NoError(t, err)
							}
						}
					} else {
						require.NoError(t, cl.Create(context.TODO(), tc))
					}
				}
				if counter == 0 {
					AssertFirstArgPrefixRestEqual("(.*)/add-cluster-(.*)", expectedHostArgs...)(t, args...)
					status := corev1.ConditionFalse
					if hostReady {
						status = corev1.ConditionTrue
					}
					// there's always at most 1 toolchain cluster for the host in the member cluster
					expectedHostToolchainClusterName, err := utils.GetToolchainClusterName("host", "https://cool-server.com", "")
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
					persist(expectedHostToolchainCluster)
				} else {
					AssertFirstArgPrefixRestEqual("(.*)/add-cluster-(.*)", expectedMemberArgs...)(t, args...)
					status := corev1.ConditionFalse
					if memberReady {
						status = corev1.ConditionTrue
					}
					expectedMemberToolchainClusterName, err := utils.GetToolchainClusterName("member", "https://cool-server.com", nameSuffix)
					require.NoError(t, err)
					expectedMemberToolchainCluster := &toolchainv1alpha1.ToolchainCluster{
						ObjectMeta: metav1.ObjectMeta{
							Name:      expectedMemberToolchainClusterName,
							Namespace: "toolchain-host-operator",
							Labels: map[string]string{
								"namespace": "toolchain-member-operator",
							},
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
					persist(expectedMemberToolchainCluster)
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
		addClusterCommand := commandCreator(fakeClient, true, true, "", false)
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
		err := registerMemberCluster(ctx, addClusterCommand, 1*time.Second, newRegisterMemberArgsWith(hostKubeconfig, memberKubeconfig, false))

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
		addClusterCommand := commandCreator(fakeClient, true, false, "", false)
		expectedHostArgs = []string{"--type", "host", "--host-kubeconfig", hostKubeconfig, "--host-ns", "toolchain-host-operator", "--member-kubeconfig", memberKubeconfig, "--member-ns", "toolchain-member-operator"}
		expectedMemberArgs = []string{"--type", "member", "--host-kubeconfig", hostKubeconfig, "--host-ns", "toolchain-host-operator", "--member-kubeconfig", memberKubeconfig, "--member-ns", "toolchain-member-operator"}

		// when
		err := registerMemberCluster(ctx, addClusterCommand, 1*time.Second, newRegisterMemberArgsWith(hostKubeconfig, memberKubeconfig, false))

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
		addClusterCommand := commandCreator(fakeClient, false, false, "", false)
		expectedHostArgs = []string{"--type", "host", "--host-kubeconfig", hostKubeconfig, "--host-ns", "toolchain-host-operator", "--member-kubeconfig", memberKubeconfig, "--member-ns", "toolchain-member-operator"}
		expectedMemberArgs = []string{"--type", "member", "--host-kubeconfig", hostKubeconfig, "--host-ns", "toolchain-host-operator", "--member-kubeconfig", memberKubeconfig, "--member-ns", "toolchain-member-operator"}

		// when
		err := registerMemberCluster(ctx, addClusterCommand, 1*time.Second, newRegisterMemberArgsWith(hostKubeconfig, memberKubeconfig, false))

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
		addClusterCommand := commandCreator(fakeClient, true, true, "", false)
		expectedHostArgs = []string{"--type", "host", "--host-kubeconfig", hostKubeconfig, "--host-ns", "toolchain-host-operator", "--member-kubeconfig", memberKubeconfig, "--member-ns", "toolchain-member-operator"}
		expectedMemberArgs = []string{"--type", "member", "--host-kubeconfig", hostKubeconfig, "--host-ns", "toolchain-host-operator", "--member-kubeconfig", memberKubeconfig, "--member-ns", "toolchain-member-operator"}

		// when
		err := registerMemberCluster(ctx, addClusterCommand, 1*time.Second, newRegisterMemberArgsWith(hostKubeconfig, memberKubeconfig, false))

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
		addClusterCommand := commandCreator(fakeClient, true, true, "", false)
		expectedHostArgs = []string{"--type", "host", "--host-kubeconfig", hostKubeconfig, "--host-ns", "toolchain-host-operator", "--member-kubeconfig", memberKubeconfig, "--member-ns", "toolchain-member-operator", "--lets-encrypt"}
		expectedMemberArgs = []string{"--type", "member", "--host-kubeconfig", hostKubeconfig, "--host-ns", "toolchain-host-operator", "--member-kubeconfig", memberKubeconfig, "--member-ns", "toolchain-member-operator", "--lets-encrypt"}

		// when
		err := registerMemberCluster(ctx, addClusterCommand, 1*time.Second, newRegisterMemberArgsWith(hostKubeconfig, memberKubeconfig, true))

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

		addClusterCommand := commandCreator(fakeClient, true, true, "2", false)
		expectedHostArgs = []string{"--type", "host", "--host-kubeconfig", hostKubeconfig, "--host-ns", "toolchain-host-operator", "--member-kubeconfig", memberKubeconfig, "--member-ns", "toolchain-member-operator"}
		expectedMemberArgs = []string{"--type", "member", "--host-kubeconfig", hostKubeconfig, "--host-ns", "toolchain-host-operator", "--member-kubeconfig", memberKubeconfig, "--member-ns", "toolchain-member-operator", "--multi-member", "2"}

		// when
		err := registerMemberCluster(ctx, addClusterCommand, 1*time.Second, newRegisterMemberArgsWithSuffix(hostKubeconfig, memberKubeconfig, false, "2"))

		// then
		require.NoError(t, err)
		assert.Equal(t, 2, counter)
		assert.Contains(t, term.Output(), "Modify and apply the following SpaceProvisionerConfig to the host cluster")
		assert.Contains(t, term.Output(), "kind: SpaceProvisionerConfig")
		assert.Contains(t, term.Output(), "toolchainCluster: member-cool-server.com2")
	})

	t.Run("cannot register the same member twice with different names", func(t *testing.T) {
		// given
		term1 := NewFakeTerminalWithResponse("Y")
		term2 := NewFakeTerminalWithResponse("Y")
		newClient, fakeClient := newFakeClientsFromRestConfig(t, deployment)
		ctx1 := newExtendedCommandContext(term1, newClient)
		ctx2 := newExtendedCommandContext(term2, newClient)
		counter = 0
		addClusterCommand := commandCreator(fakeClient, true, true, "", true)
		expectedHostArgs = []string{"--type", "host", "--host-kubeconfig", hostKubeconfig, "--host-ns", "toolchain-host-operator", "--member-kubeconfig", memberKubeconfig, "--member-ns", "toolchain-member-operator"}
		expectedMemberArgs = []string{"--type", "member", "--host-kubeconfig", hostKubeconfig, "--host-ns", "toolchain-host-operator", "--member-kubeconfig", memberKubeconfig, "--member-ns", "toolchain-member-operator"}

		// when
		err1 := registerMemberCluster(ctx1, addClusterCommand, 1*time.Second, newRegisterMemberArgsWith(hostKubeconfig, memberKubeconfig, false))
		addClusterCommand = commandCreator(fakeClient, true, true, "1", true)
		err2 := registerMemberCluster(ctx2, addClusterCommand, 1*time.Second, newRegisterMemberArgsWithSuffix(hostKubeconfig, memberKubeconfig, false, "1"))

		// then
		require.NoError(t, err1)
		assert.Equal(t, 2, counter)
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
		newClient, fakeClient := newFakeClientsFromRestConfig(t, deployment)
		ctx1 := newExtendedCommandContext(term1, newClient)
		ctx2 := newExtendedCommandContext(term2, newClient)
		counter1 := 0
		counter2 := 0
		counter = 0
		addClusterCommand := commandCreator(fakeClient, true, true, "", true)
		expectedHostArgs = []string{"--type", "host", "--host-kubeconfig", hostKubeconfig, "--host-ns", "toolchain-host-operator", "--member-kubeconfig", memberKubeconfig, "--member-ns", "toolchain-member-operator"}
		expectedMemberArgs = []string{"--type", "member", "--host-kubeconfig", hostKubeconfig, "--host-ns", "toolchain-host-operator", "--member-kubeconfig", memberKubeconfig, "--member-ns", "toolchain-member-operator"}

		// when
		err1 := registerMemberCluster(ctx1, addClusterCommand, 1*time.Second, newRegisterMemberArgsWith(hostKubeconfig, memberKubeconfig, false))
		counter1 = counter
		counter = 0
		err2 := registerMemberCluster(ctx2, addClusterCommand, 1*time.Second, newRegisterMemberArgsWithSuffix(hostKubeconfig, memberKubeconfig, false, ""))
		counter2 = counter

		// then
		require.NoError(t, err1)
		assert.Equal(t, 2, counter1)
		assert.Equal(t, 2, counter2)
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
		newClient, fakeClient := newFakeClientsFromRestConfig(t, deployment)
		ctx := newExtendedCommandContext(term, newClient)
		counter = 0
		preexistingToolchainCluster1 := &toolchainv1alpha1.ToolchainCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "host-not-so-cool-server.com",
				Namespace: "toolchain-member-operator",
			},
			Spec: toolchainv1alpha1.ToolchainClusterSpec{
				APIEndpoint: "https://not-so-cool-server.com",
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
		preexistingToolchainCluster2 := &toolchainv1alpha1.ToolchainCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "host-uncool-server.com",
				Namespace: "toolchain-member-operator",
			},
			Spec: toolchainv1alpha1.ToolchainClusterSpec{
				APIEndpoint: "https://uncool-server.com",
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
		require.NoError(t, fakeClient.Create(context.TODO(), preexistingToolchainCluster1.DeepCopy()))
		require.NoError(t, fakeClient.Create(context.TODO(), preexistingToolchainCluster2.DeepCopy()))

		addClusterCommand := commandCreator(fakeClient, true, true, "1", false)
		expectedHostArgs = []string{"--type", "host", "--host-kubeconfig", hostKubeconfig, "--host-ns", "toolchain-host-operator", "--member-kubeconfig", memberKubeconfig, "--member-ns", "toolchain-member-operator"}
		expectedMemberArgs = []string{"--type", "member", "--host-kubpconfig", hostKubeconfig, "--host-ns", "toolchain-host-operator", "--member-kubeconfig", memberKubeconfig, "--member-ns", "toolchain-member-operator", "--multi-member", "2"}

		// when
		err := registerMemberCluster(ctx, addClusterCommand, 1*time.Second, newRegisterMemberArgsWith(hostKubeconfig, memberKubeconfig, false))

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), `Cannot proceed because of the following problems:
- member misconfigured: the member cluster (https://cool-server.com) is already registered with more than 1 host in namespace toolchain-member-operator`)
		assert.Equal(t, 0, counter)
		assert.NotContains(t, term.Output(), "kind: SpaceProvisionerConfig")
	})

	t.Run("Errors when registering into another host", func(t *testing.T) {
		// given
		term := NewFakeTerminalWithResponse("Y")
		newClient, fakeClient := newFakeClientsFromRestConfig(t, deployment)
		ctx := newExtendedCommandContext(term, newClient)
		counter = 0
		preexistingToolchainCluster := &toolchainv1alpha1.ToolchainCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "host-not-so-cool-server.com",
				Namespace: "toolchain-member-operator",
			},
			Spec: toolchainv1alpha1.ToolchainClusterSpec{
				APIEndpoint: "https://not-so-cool-server.com",
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

		addClusterCommand := commandCreator(fakeClient, true, true, "1", false)
		expectedHostArgs = []string{"--type", "host", "--host-kubeconfig", hostKubeconfig, "--host-ns", "toolchain-host-operator", "--member-kubeconfig", memberKubeconfig, "--member-ns", "toolchain-member-operator"}
		expectedMemberArgs = []string{"--type", "member", "--host-kubeconfig", hostKubeconfig, "--host-ns", "toolchain-host-operator", "--member-kubeconfig", memberKubeconfig, "--member-ns", "toolchain-member-operator", "--multi-member", "2"}

		// when
		err := registerMemberCluster(ctx, addClusterCommand, 1*time.Second, newRegisterMemberArgsWith(hostKubeconfig, memberKubeconfig, false))

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), `Cannot proceed because of the following problems:
- the member is already registered with another host (https://not-so-cool-server.com) so registering it with the new one (https://cool-server.com) would result in an invalid configuration`)
		assert.Equal(t, 0, counter)
		assert.NotContains(t, term.Output(), "kind: SpaceProvisionerConfig")
	})

	t.Run("Errors when host with different name already exists", func(t *testing.T) {
		// given
		term := NewFakeTerminalWithResponse("Y")
		newClient, fakeClient := newFakeClientsFromRestConfig(t, deployment)
		ctx := newExtendedCommandContext(term, newClient)
		counter = 0
		preexistingToolchainCluster := &toolchainv1alpha1.ToolchainCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "host-with-weird-name",
				Namespace: "toolchain-member-operator",
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

		addClusterCommand := commandCreator(fakeClient, true, true, "1", false)
		expectedHostArgs = []string{"--type", "host", "--host-kubeconfig", hostKubeconfig, "--host-ns", "toolchain-host-operator", "--member-kubeconfig", memberKubeconfig, "--member-ns", "toolchain-member-operator"}
		expectedMemberArgs = []string{"--type", "member", "--host-kubeconfig", hostKubeconfig, "--host-ns", "toolchain-host-operator", "--member-kubeconfig", memberKubeconfig, "--member-ns", "toolchain-member-operator", "--multi-member", "2"}

		// when
		err := registerMemberCluster(ctx, addClusterCommand, 1*time.Second, newRegisterMemberArgsWith(hostKubeconfig, memberKubeconfig, false))

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), `Cannot proceed because of the following problems:
- the host is already in the member namespace using a ToolchainCluster object with the name 'host-with-weird-name' but the new registration would use a ToolchainCluster with the name 'host-cool-server.com' which would lead to an invalid configuration`)
		assert.Equal(t, 0, counter)
		assert.NotContains(t, term.Output(), "kind: SpaceProvisionerConfig")
	})

	t.Run("Errors when member with different name already exists", func(t *testing.T) {
		// given
		term := NewFakeTerminalWithResponse("Y")
		newClient, fakeClient := newFakeClientsFromRestConfig(t, deployment)
		ctx := newExtendedCommandContext(term, newClient)
		counter = 0
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
				Conditions: []toolchainv1alpha1.ToolchainClusterCondition{
					{
						Type:   toolchainv1alpha1.ToolchainClusterReady,
						Status: corev1.ConditionTrue,
					},
				},
			},
		}
		require.NoError(t, fakeClient.Create(context.TODO(), preexistingToolchainCluster.DeepCopy()))

		addClusterCommand := commandCreator(fakeClient, true, true, "", false)
		expectedHostArgs = []string{"--type", "host", "--host-kubeconfig", hostKubeconfig, "--host-ns", "toolchain-host-operator", "--member-kubeconfig", memberKubeconfig, "--member-ns", "toolchain-member-operator"}
		expectedMemberArgs = []string{"--type", "member", "--host-kubeconfig", hostKubeconfig, "--host-ns", "toolchain-host-operator", "--member-kubeconfig", memberKubeconfig, "--member-ns", "toolchain-member-operator", "--multi-member", "2"}

		// when
		err := registerMemberCluster(ctx, addClusterCommand, 1*time.Second, newRegisterMemberArgsWith(hostKubeconfig, memberKubeconfig, false))

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), `Cannot proceed because of the following problems:
- the newly registered member cluster would have a different name (member-cool-server.com) than the already existing one (member-with-weird-name) which would lead to invalid configuration. Consider using the --name-suffix parameter to match the existing member registration if you intend to just update it instead of creating a new registration`)
		assert.Equal(t, 0, counter)
		assert.NotContains(t, term.Output(), "kind: SpaceProvisionerConfig")
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

	test := func(t *testing.T, clusterType configuration.ClusterType, nameSuffix string, letEncrypt bool, additionalExpectedArgs ...string) {
		// given
		expArgs := []string{"--type", clusterType.String(), "--host-kubeconfig", hostKubeconfig, "--host-ns", "host-ns", "--member-kubeconfig", memberKubeconfig, "--member-ns", "member-ns"}
		expArgs = append(expArgs, additionalExpectedArgs...)
		ocCommandCreator := NewCommandCreator(t, "echo", "bash",
			AssertFirstArgPrefixRestEqual("(.*)/add-cluster-(.*)", expArgs...))

		// when
		err := runAddClusterScript(term, ocCommandCreator, clusterType, hostKubeconfig, "host-ns", memberKubeconfig, "member-ns", nameSuffix, letEncrypt)

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
				test(t, clusterType, "", false)
			})
			t.Run("single toolchain in cluster with letsencrypt", func(t *testing.T) {
				test(t, clusterType, "", true, "--lets-encrypt")
			})
			t.Run("multiple toolchains in cluster", func(t *testing.T) {
				test(t, clusterType, "asdf", false, "--multi-member", "asdf")
			})
			t.Run("multiple toolchains in cluster with letsencrypt", func(t *testing.T) {
				test(t, clusterType, "42", true, "--multi-member", "42", "--lets-encrypt")
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
			err := runAddClusterScript(term, ocCommandCreator, clusterType, hostKubeconfig, "host-ns", memberKubeconfig, "member-ns", "", true)

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
	args := newRegisterMemberArgs()
	args.hostKubeConfig = hostKubeconfig
	args.memberKubeConfig = memberKubeconfig
	args.useLetsEncrypt = useLetsEncrypt
	return args
}

func newRegisterMemberArgsWithSuffix(hostKubeconfig, memberKubeconfig string, useLetsEncrypt bool, nameSuffix string) registerMemberArgs {
	args := newRegisterMemberArgs()
	args.hostKubeConfig = hostKubeconfig
	args.memberKubeConfig = memberKubeconfig
	args.useLetsEncrypt = useLetsEncrypt
	args.nameSuffix = nameSuffix
	return args
}

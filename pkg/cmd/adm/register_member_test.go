package adm

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/cluster"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/ghodss/yaml"
	"github.com/kubesaw/ksctl/pkg/configuration"
	clicontext "github.com/kubesaw/ksctl/pkg/context"
	. "github.com/kubesaw/ksctl/pkg/test"
	"github.com/kubesaw/ksctl/pkg/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/client-go/util/homedir"
	pointer "k8s.io/utils/ptr"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func TestRegisterMember(t *testing.T) {
	// given
	SetFileConfig(t, Host(), Member())
	hostKubeconfig := PersistKubeConfigFile(t, HostKubeConfig())
	memberKubeconfig := PersistKubeConfigFile(t, MemberKubeConfig())

	toolchainClusterMemberSa := corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "toolchaincluster-member",
			Namespace: test.MemberOperatorNs,
		},
	}
	toolchainClusterHostSa := corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "toolchaincluster-host",
			Namespace: test.HostOperatorNs,
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

	t.Run("commandline parsing", func(t *testing.T) {
		testWithArgs := func(t *testing.T, args []string) registerMemberArgs {
			t.Helper()

			var parsedArgs *registerMemberArgs
			cmd := newRegisterMemberCmd(func(_ *extendedCommandContext, parsed registerMemberArgs, _ restartFunc) error {
				parsedArgs = &parsed
				return nil
			})

			cmd.SetErr(&strings.Builder{})
			cmd.SetOut(&strings.Builder{})
			cmd.SetArgs(args)

			require.NoError(t, cmd.Execute())
			require.NotNil(t, parsedArgs)

			return *parsedArgs
		}

		t.Run("insecureSkipTlsVerify not specified", func(t *testing.T) {
			args := testWithArgs(t, []string{"--host-kubeconfig=h", "--member-kubeconfig", "m"})
			assert.Nil(t, args.skipTlsVerify)
			assert.Equal(t, "h", args.hostKubeConfig)
			assert.Equal(t, "m", args.memberKubeConfig)
		})

		t.Run("insecureSkipTlsVerify false", func(t *testing.T) {
			args := testWithArgs(t, []string{"--host-kubeconfig=h", "--member-kubeconfig", "m", "--insecure-skip-tls-verify=false"})
			require.NotNil(t, args.skipTlsVerify)
			assert.False(t, *args.skipTlsVerify)
		})

		t.Run("insecureSkipTlsVerify true", func(t *testing.T) {
			args := testWithArgs(t, []string{"--host-kubeconfig=h", "--member-kubeconfig", "m", "--insecure-skip-tls-verify"})
			require.NotNil(t, args.skipTlsVerify)
			assert.True(t, *args.skipTlsVerify)
		})

		t.Run("lets-encrypt false", func(t *testing.T) {
			args := testWithArgs(t, []string{"--host-kubeconfig=h", "--member-kubeconfig", "m", "--lets-encrypt=false"})
			require.NotNil(t, args.skipTlsVerify)
			assert.True(t, *args.skipTlsVerify)
		})
	})

	t.Run("produces valid example SPC", func(t *testing.T) {
		// given
		term := NewFakeTerminalWithResponse("Y")
		newClient, fakeClient := newFakeClientsFromRestConfig(t, &toolchainClusterMemberSa, &toolchainClusterHostSa)
		// force the ready condition on the toolchaincluster created ( this is done by the tc controller in prod env )
		mockCreateToolchainClusterWithReadyCondition(t, fakeClient)
		ctx := newExtendedCommandContext(term, newClient)

		expectedExampleSPC := &toolchainv1alpha1.SpaceProvisionerConfig{
			TypeMeta: metav1.TypeMeta{
				Kind:       "SpaceProvisionerConfig",
				APIVersion: toolchainv1alpha1.GroupVersion.Identifier(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "member-cool-server.com",
				Namespace: test.HostOperatorNs,
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
		err := registerMemberCluster(ctx, newRegisterMemberArgsWith(hostKubeconfig, memberKubeconfig, false), func(_ *clicontext.CommandContext, _ string, _ ConfigFlagsAndClientGetterFunc) error {
			return nil
		})

		// then
		require.NoError(t, err)
		// check the expected secrets are there with the kubeconfigs
		// the member kubeconfig secret in the host namespace
		verifyToolchainClusterSecret(t, fakeClient, toolchainClusterMemberSa.Name, test.HostOperatorNs, test.MemberOperatorNs, memberToolchainClusterName)
		// the host secret in the member namespace
		verifyToolchainClusterSecret(t, fakeClient, toolchainClusterHostSa.Name, test.MemberOperatorNs, test.HostOperatorNs, hostToolchainClusterName)
		tcs := &toolchainv1alpha1.ToolchainClusterList{}
		require.NoError(t, fakeClient.List(context.TODO(), tcs, runtimeclient.InNamespace(test.HostOperatorNs)))
		assert.Len(t, tcs.Items, 1)
		assert.Equal(t, memberToolchainClusterName, tcs.Items[0].Name)
		// secret ref in tc matches
		assert.Equal(t, toolchainClusterMemberSa.Name+"-"+memberToolchainClusterName, tcs.Items[0].Spec.SecretRef.Name)
		require.NoError(t, fakeClient.List(context.TODO(), tcs, runtimeclient.InNamespace(test.MemberOperatorNs)))
		assert.Len(t, tcs.Items, 1)
		assert.Equal(t, hostToolchainClusterName, tcs.Items[0].Name)
		// secret ref in tc matches
		assert.Equal(t, toolchainClusterHostSa.Name+"-"+hostToolchainClusterName, tcs.Items[0].Spec.SecretRef.Name)
		assert.Contains(t, term.Output(), "Modify and apply the following SpaceProvisionerConfig to the host cluster")
		actualExampleSPC := extractExampleSPCFromOutput(t, term.Output())
		assert.Equal(t, *expectedExampleSPC, actualExampleSPC)
	})

	t.Run("reports error when member ToolchainCluster is not ready in host", func(t *testing.T) {
		// given
		term := NewFakeTerminalWithResponse("Y")
		newClient, fakeClient := newFakeClientsFromRestConfig(t, &toolchainClusterMemberSa, &toolchainClusterHostSa)
		mockCreateToolchainClusterInNamespaceWithReadyCondition(t, fakeClient, test.MemberOperatorNs) // we set to ready only the host toolchaincluster in member operator namespace
		ctx := newExtendedCommandContext(term, newClient)

		// when
		err := registerMemberCluster(ctx, newRegisterMemberArgsWith(hostKubeconfig, memberKubeconfig, false), func(_ *clicontext.CommandContext, _ string, _ ConfigFlagsAndClientGetterFunc) error {
			return nil
		})

		// then
		require.Error(t, err)
		tcs := &toolchainv1alpha1.ToolchainClusterList{}
		require.NoError(t, fakeClient.List(context.TODO(), tcs, runtimeclient.InNamespace(test.HostOperatorNs)))
		assert.Len(t, tcs.Items, 1)
		require.NoError(t, fakeClient.List(context.TODO(), tcs, runtimeclient.InNamespace(test.MemberOperatorNs)))
		assert.Len(t, tcs.Items, 1)
		assert.Contains(t, term.Output(), "The ToolchainCluster resource representing the member in the host cluster has not become ready.")
	})

	t.Run("reports error when host ToolchainCluster is not ready in member", func(t *testing.T) {
		// given
		term := NewFakeTerminalWithResponse("Y")
		newClient, fakeClient := newFakeClientsFromRestConfig(t, &toolchainClusterMemberSa, &toolchainClusterHostSa)
		mockCreateToolchainClusterInNamespaceWithReadyCondition(t, fakeClient, test.HostOperatorNs) // set to ready only the member toolchaincluster in host operator namespace
		ctx := newExtendedCommandContext(term, newClient)

		// when
		err := registerMemberCluster(ctx, newRegisterMemberArgsWith(hostKubeconfig, memberKubeconfig, false), func(_ *clicontext.CommandContext, _ string, _ ConfigFlagsAndClientGetterFunc) error {
			return nil
		})

		// then
		require.Error(t, err)
		tcs := &toolchainv1alpha1.ToolchainClusterList{}
		require.NoError(t, fakeClient.List(context.TODO(), tcs, runtimeclient.InNamespace(test.HostOperatorNs)))
		assert.Empty(t, tcs.Items)
		assert.Contains(t, term.Output(), "The ToolchainCluster resource representing the host in the member cluster has not become ready.")
		require.NoError(t, fakeClient.List(context.TODO(), tcs, runtimeclient.InNamespace(test.MemberOperatorNs)))
		assert.Len(t, tcs.Items, 1)
	})

	t.Run("single toolchain in cluster", func(t *testing.T) {
		// given
		term := NewFakeTerminalWithResponse("Y")
		newClient, fakeClient := newFakeClientsFromRestConfig(t, &toolchainClusterMemberSa, &toolchainClusterHostSa)
		mockCreateToolchainClusterWithReadyCondition(t, fakeClient)
		ctx := newExtendedCommandContext(term, newClient)

		// when
		err := registerMemberCluster(ctx, newRegisterMemberArgsWith(hostKubeconfig, memberKubeconfig, false), func(_ *clicontext.CommandContext, _ string, _ ConfigFlagsAndClientGetterFunc) error {
			return nil
		})

		// then
		require.NoError(t, err)
		assert.Contains(t, term.Output(), "Modify and apply the following SpaceProvisionerConfig to the host cluster")
		assert.Contains(t, term.Output(), "kind: SpaceProvisionerConfig")
	})

	t.Run("single toolchain in cluster with --insecure-skip-tls-verify", func(t *testing.T) {
		// given
		term := NewFakeTerminalWithResponse("Y")
		newClient, fakeClient := newFakeClientsFromRestConfig(t, &toolchainClusterMemberSa, &toolchainClusterHostSa)
		mockCreateToolchainClusterWithReadyCondition(t, fakeClient)
		ctx := newExtendedCommandContext(term, newClient)

		// when
		err := registerMemberCluster(ctx, newRegisterMemberArgsWith(hostKubeconfig, memberKubeconfig, true), func(_ *clicontext.CommandContext, _ string, _ ConfigFlagsAndClientGetterFunc) error {
			return nil
		})

		// then
		require.NoError(t, err)
		assert.Contains(t, term.Output(), "Modify and apply the following SpaceProvisionerConfig to the host cluster")
		assert.Contains(t, term.Output(), "kind: SpaceProvisionerConfig")
	})

	t.Run("multiple toolchains in cluster", func(t *testing.T) {
		// given
		term := NewFakeTerminalWithResponse("Y")
		newClient, fakeClient := newFakeClientsFromRestConfig(t, &toolchainClusterMemberSa, &toolchainClusterHostSa)
		ctx := newExtendedCommandContext(term, newClient)
		preexistingToolchainCluster := &toolchainv1alpha1.ToolchainCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "member-cool-server.com",
				Namespace: test.HostOperatorNs,
			},
			Status: toolchainv1alpha1.ToolchainClusterStatus{
				APIEndpoint: "https://cool-server.com",
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

		mockCreateToolchainClusterWithReadyCondition(t, fakeClient)

		// when
		err := registerMemberCluster(ctx, newRegisterMemberArgsWithSuffix(hostKubeconfig, memberKubeconfig, false, "2"), func(_ *clicontext.CommandContext, _ string, _ ConfigFlagsAndClientGetterFunc) error {
			return nil
		})

		// then
		require.NoError(t, err)
		assert.Contains(t, term.Output(), "source cluster name: member-cool-server.com2")
		assert.Contains(t, term.Output(), "The name of the target cluster: member-cool-server.com")
		assert.Contains(t, term.Output(), "Modify and apply the following SpaceProvisionerConfig to the host cluster")
		assert.Contains(t, term.Output(), "kind: SpaceProvisionerConfig")
		assert.Contains(t, term.Output(), "toolchainCluster: member-cool-server.com2")
	})

	t.Run("cannot register the same member twice with different names", func(t *testing.T) {
		// given
		term1 := NewFakeTerminalWithResponse("Y")
		term2 := NewFakeTerminalWithResponse("Y")
		newClient, fakeClient := newFakeClientsFromRestConfig(t, &toolchainClusterMemberSa, &toolchainClusterHostSa)
		mockCreateToolchainClusterWithReadyCondition(t, fakeClient)
		ctx1 := newExtendedCommandContext(term1, newClient)
		ctx2 := newExtendedCommandContext(term2, newClient)

		// when
		err1 := registerMemberCluster(ctx1, newRegisterMemberArgsWith(hostKubeconfig, memberKubeconfig, false), func(_ *clicontext.CommandContext, _ string, _ ConfigFlagsAndClientGetterFunc) error {
			return nil
		})
		err2 := registerMemberCluster(ctx2, newRegisterMemberArgsWithSuffix(hostKubeconfig, memberKubeconfig, false, "1"), func(_ *clicontext.CommandContext, _ string, _ ConfigFlagsAndClientGetterFunc) error {
			return nil
		})

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
		newClient, fakeClient := newFakeClientsFromRestConfig(t, &toolchainClusterMemberSa, &toolchainClusterHostSa)
		mockCreateToolchainClusterWithReadyCondition(t, fakeClient)
		ctx1 := newExtendedCommandContext(term1, newClient)
		ctx2 := newExtendedCommandContext(term2, newClient)

		// when
		err1 := registerMemberCluster(ctx1, newRegisterMemberArgsWith(hostKubeconfig, memberKubeconfig, false), func(_ *clicontext.CommandContext, _ string, _ ConfigFlagsAndClientGetterFunc) error {
			return nil
		})
		err2 := registerMemberCluster(ctx2, newRegisterMemberArgsWithSuffix(hostKubeconfig, memberKubeconfig, false, ""), func(_ *clicontext.CommandContext, _ string, _ ConfigFlagsAndClientGetterFunc) error {
			return nil
		})

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
		newClient, fakeClient := newFakeClientsFromRestConfig(t, &toolchainClusterMemberSa, &toolchainClusterHostSa)
		mockCreateToolchainClusterWithReadyCondition(t, fakeClient)
		ctx := newExtendedCommandContext(term, newClient)
		preexistingToolchainCluster1 := &toolchainv1alpha1.ToolchainCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "host-not-so-cool-server.com",
				Namespace: test.MemberOperatorNs,
			},
			Status: toolchainv1alpha1.ToolchainClusterStatus{
				APIEndpoint: "https://not-so-cool-server.com",
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
				Namespace: test.MemberOperatorNs,
			},
			Status: toolchainv1alpha1.ToolchainClusterStatus{
				APIEndpoint: "https://uncool-server.com",
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
		err := registerMemberCluster(ctx, newRegisterMemberArgsWith(hostKubeconfig, memberKubeconfig, false), func(_ *clicontext.CommandContext, _ string, _ ConfigFlagsAndClientGetterFunc) error {
			return nil
		})

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), `Cannot proceed because of the following problems:
	- member misconfigured: the member cluster (https://cool-server.com) is already registered with more than 1 host in namespace toolchain-member-operator`)
		assert.NotContains(t, term.Output(), "kind: SpaceProvisionerConfig")
	})

	t.Run("Errors when registering into another host", func(t *testing.T) {
		// given
		term := NewFakeTerminalWithResponse("Y")
		newClient, fakeClient := newFakeClientsFromRestConfig(t)
		ctx := newExtendedCommandContext(term, newClient)
		preexistingToolchainCluster := &toolchainv1alpha1.ToolchainCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "host-not-so-cool-server.com",
				Namespace: test.MemberOperatorNs,
			},
			Status: toolchainv1alpha1.ToolchainClusterStatus{
				APIEndpoint: "https://not-so-cool-server.com",
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
		err := registerMemberCluster(ctx, newRegisterMemberArgsWith(hostKubeconfig, memberKubeconfig, false), func(_ *clicontext.CommandContext, _ string, _ ConfigFlagsAndClientGetterFunc) error {
			return nil
		})

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), `Cannot proceed because of the following problems:
	- the member is already registered with another host (https://not-so-cool-server.com) so registering it with the new one (https://cool-server.com) would result in an invalid configuration`)
		assert.NotContains(t, term.Output(), "kind: SpaceProvisionerConfig")
	})

	t.Run("Errors when host with different name already exists", func(t *testing.T) {
		// given
		term := NewFakeTerminalWithResponse("Y")
		newClient, fakeClient := newFakeClientsFromRestConfig(t)
		ctx := newExtendedCommandContext(term, newClient)
		preexistingToolchainCluster := &toolchainv1alpha1.ToolchainCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "host-with-weird-name",
				Namespace: test.MemberOperatorNs,
			},
			Status: toolchainv1alpha1.ToolchainClusterStatus{
				APIEndpoint: "https://cool-server.com",
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
		err := registerMemberCluster(ctx, newRegisterMemberArgsWith(hostKubeconfig, memberKubeconfig, false), func(_ *clicontext.CommandContext, _ string, _ ConfigFlagsAndClientGetterFunc) error {
			return nil
		})

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), `Cannot proceed because of the following problems:
	- the host is already in the member namespace using a ToolchainCluster object with the name 'host-with-weird-name' but the new registration would use a ToolchainCluster with the name 'host-cool-server.com' which would lead to an invalid configuration`)
		assert.NotContains(t, term.Output(), "kind: SpaceProvisionerConfig")
	})

	t.Run("Errors when member with different name already exists", func(t *testing.T) {
		// given
		term := NewFakeTerminalWithResponse("Y")
		newClient, fakeClient := newFakeClientsFromRestConfig(t)
		ctx := newExtendedCommandContext(term, newClient)
		preexistingToolchainCluster := &toolchainv1alpha1.ToolchainCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "member-with-weird-name",
				Namespace: test.HostOperatorNs,
			},
			Status: toolchainv1alpha1.ToolchainClusterStatus{
				APIEndpoint:       "https://cool-server.com",
				OperatorNamespace: test.MemberOperatorNs,
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
		err := registerMemberCluster(ctx, newRegisterMemberArgsWith(hostKubeconfig, memberKubeconfig, false), func(_ *clicontext.CommandContext, _ string, _ ConfigFlagsAndClientGetterFunc) error {
			return nil
		})

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), `Cannot proceed because of the following problems:
	- the newly registered member cluster would have a different name (member-cool-server.com) than the already existing one (member-with-weird-name) which would lead to invalid configuration. Consider using the --name-suffix parameter to match the existing member registration if you intend to just update it instead of creating a new registration`)
		assert.NotContains(t, term.Output(), "kind: SpaceProvisionerConfig")
	})

	t.Run("reports error when member toolchaincluster ServiceAccount is not there", func(t *testing.T) {
		// given
		term := NewFakeTerminalWithResponse("Y")
		newClient, fakeClient := newFakeClientsFromRestConfig(t, &toolchainClusterHostSa) // we pre-provision only the host toolchaincluster ServiceAccount
		mockCreateToolchainClusterWithReadyCondition(t, fakeClient)
		ctx := newExtendedCommandContext(term, newClient)

		// when
		err := registerMemberCluster(ctx, newRegisterMemberArgsWith(hostKubeconfig, memberKubeconfig, false), func(_ *clicontext.CommandContext, _ string, _ ConfigFlagsAndClientGetterFunc) error {
			return nil
		})

		// then
		require.Error(t, err)
		assert.Contains(t, term.Output(), "The toolchain-member-operator/toolchaincluster-member ServiceAccount is not present in the member cluster.")
		tcs := &toolchainv1alpha1.ToolchainClusterList{}
		require.NoError(t, fakeClient.List(context.TODO(), tcs, runtimeclient.InNamespace(test.HostOperatorNs)))
		assert.Empty(t, tcs.Items)
		require.NoError(t, fakeClient.List(context.TODO(), tcs, runtimeclient.InNamespace(test.MemberOperatorNs)))
		assert.Len(t, tcs.Items, 1)
	})

	t.Run("reports error when host toolchaincluster ServiceAccount is not there", func(t *testing.T) {
		// given
		term := NewFakeTerminalWithResponse("Y")
		newClient, fakeClient := newFakeClientsFromRestConfig(t)
		ctx := newExtendedCommandContext(term, newClient)

		// when
		err := registerMemberCluster(ctx, newRegisterMemberArgsWith(hostKubeconfig, memberKubeconfig, false), func(_ *clicontext.CommandContext, _ string, _ ConfigFlagsAndClientGetterFunc) error {
			return nil
		})

		// then
		require.Error(t, err)
		assert.Contains(t, term.Output(), "The toolchain-host-operator/toolchaincluster-host ServiceAccount is not present in the host cluster.")
		tcs := &toolchainv1alpha1.ToolchainClusterList{}
		require.NoError(t, fakeClient.List(context.TODO(), tcs, runtimeclient.InNamespace(test.HostOperatorNs)))
		assert.Empty(t, tcs.Items)
		require.NoError(t, fakeClient.List(context.TODO(), tcs, runtimeclient.InNamespace(test.MemberOperatorNs)))
		assert.Empty(t, tcs.Items)
	})

	t.Run("reports error when host-operator is not restarted", func(t *testing.T) {
		// given
		term := NewFakeTerminalWithResponse("Y")
		newClient, fakeClient := newFakeClientsFromRestConfig(t, &toolchainClusterMemberSa, &toolchainClusterHostSa)
		mockCreateToolchainClusterWithReadyCondition(t, fakeClient)
		ctx := newExtendedCommandContext(term, newClient)

		// when
		err := registerMemberCluster(ctx, newRegisterMemberArgsWith(hostKubeconfig, memberKubeconfig, false), func(_ *clicontext.CommandContext, _ string, _ ConfigFlagsAndClientGetterFunc) error {
			return fmt.Errorf("restart did not happen")
		})

		// then
		require.EqualError(t, err, "restart did not happen")
	})

	t.Run("Register-member calls restart ", func(t *testing.T) {
		// given
		term := NewFakeTerminalWithResponse("Y")
		newClient, fakeClient := newFakeClientsFromRestConfig(t, &toolchainClusterMemberSa, &toolchainClusterHostSa)
		mockCreateToolchainClusterWithReadyCondition(t, fakeClient)
		ctx := newExtendedCommandContext(term, newClient)
		called := 0
		// when
		err := registerMemberCluster(ctx, newRegisterMemberArgsWith(hostKubeconfig, memberKubeconfig, false), func(_ *clicontext.CommandContext, _ string, _ ConfigFlagsAndClientGetterFunc) error {
			called++
			return mockRestartReg(ctx.CommandContext, configuration.HostName, nil)
		})

		// then
		require.NoError(t, err)
		assert.Equal(t, 1, called)
	})
}

func TestCreateKubeConfig(t *testing.T) {
	t.Run("--insecureSkipTlsVerify", func(t *testing.T) {
		t.Run("overrides true with false", func(t *testing.T) {
			// given
			hostKubeconfigSecure := HostKubeConfig()
			hostKubeconfigSecure.Clusters["host"].InsecureSkipTLSVerify = true

			// when
			config, err := generateKubeConfig("token", "ns", pointer.To(false), hostKubeconfigSecure)
			require.NoError(t, err)

			// then
			assert.False(t, config.Clusters["cluster"].InsecureSkipTLSVerify)
		})
		t.Run("overrides false with true", func(t *testing.T) {
			// given
			hostKubeconfigSecure := HostKubeConfig()
			hostKubeconfigSecure.Clusters["host"].InsecureSkipTLSVerify = false

			// when
			config, err := generateKubeConfig("token", "ns", pointer.To(true), hostKubeconfigSecure)
			require.NoError(t, err)

			// then
			assert.True(t, config.Clusters["cluster"].InsecureSkipTLSVerify)
		})
		t.Run("leaves true from kubeconfig when undefined", func(t *testing.T) {
			// given
			hostKubeconfigSecure := HostKubeConfig()
			hostKubeconfigSecure.Clusters["host"].InsecureSkipTLSVerify = true

			// when
			config, err := generateKubeConfig("token", "ns", nil, hostKubeconfigSecure)
			require.NoError(t, err)

			// then
			assert.True(t, config.Clusters["cluster"].InsecureSkipTLSVerify)
		})
		t.Run("leaves false from kubeconfig when undefined", func(t *testing.T) {
			// given
			hostKubeconfigSecure := HostKubeConfig()
			hostKubeconfigSecure.Clusters["host"].InsecureSkipTLSVerify = false

			// when
			config, err := generateKubeConfig("token", "ns", nil, hostKubeconfigSecure)
			require.NoError(t, err)

			// then
			assert.False(t, config.Clusters["cluster"].InsecureSkipTLSVerify)
		})
	})

	t.Run("other auth methods cleared", func(t *testing.T) {
		// given
		kubeConfig := HostKubeConfig()

		auth := &clientcmdapi.AuthInfo{
			ClientCertificate:     "client-certificate",
			ClientCertificateData: []byte("client-certificate-data"),
			ClientKey:             "client-key",
			ClientKeyData:         []byte("client-key-data"),
			Token:                 "",
			TokenFile:             "token-file",
			Impersonate:           "root",
			ImpersonateUID:        "1",
			ImpersonateGroups:     []string{"root"},
			ImpersonateUserExtra:  map[string][]string{},
			Username:              "johndoe",
			Password:              "123456",
			AuthProvider: &clientcmdapi.AuthProviderConfig{
				Name:   "gimme-root",
				Config: map[string]string{},
			},
			Exec:       &clientcmdapi.ExecConfig{},
			Extensions: map[string]runtime.Object{},
		}
		kubeConfig.AuthInfos[kubeConfig.Contexts[kubeConfig.CurrentContext].AuthInfo] = auth

		// when
		config, err := generateKubeConfig("token", "ns", nil, kubeConfig)
		require.NoError(t, err)

		// then
		generatedAuth := config.AuthInfos[config.Contexts[config.CurrentContext].AuthInfo]

		assert.Equal(t, []byte("client-certificate-data"), generatedAuth.ClientCertificateData)
		assert.Equal(t, []byte("client-key-data"), generatedAuth.ClientKeyData)
		assert.Equal(t, "token", generatedAuth.Token)
		assert.Empty(t, generatedAuth.ClientKey)
		assert.Empty(t, generatedAuth.ClientCertificate)
		assert.Empty(t, generatedAuth.TokenFile)
		assert.Empty(t, generatedAuth.Impersonate)
		assert.Empty(t, generatedAuth.ImpersonateUID)
		assert.Empty(t, generatedAuth.ImpersonateGroups)
		assert.Empty(t, generatedAuth.ImpersonateUserExtra)
		assert.Empty(t, generatedAuth.Username)
		assert.Empty(t, generatedAuth.Password)
		assert.Nil(t, generatedAuth.AuthProvider)
		assert.Nil(t, generatedAuth.Exec)
	})

	t.Run("namespace overridden", func(t *testing.T) {
		// given
		kubeConfig := HostKubeConfig()
		require.Equal(t, "toolchain-host-operator", kubeConfig.Contexts[kubeConfig.CurrentContext].Namespace)

		// when
		config, err := generateKubeConfig("token", "ns", nil, kubeConfig)
		require.NoError(t, err)

		// then
		generatedContext := config.Contexts[config.CurrentContext]

		assert.Equal(t, "ns", generatedContext.Namespace)
	})

	t.Run("reads referenced files in kubeconfig to appropriate data fields", func(t *testing.T) {
		// given
		f, err := os.CreateTemp("", "ref-test")
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(f.Name(), []byte("data"), 0))
		defer os.Remove(f.Name())

		kubeConfig := HostKubeConfig()
		kubeConfig.Clusters["host"].CertificateAuthority = f.Name()
		kubeConfig.AuthInfos["auth"] = clientcmdapi.NewAuthInfo()
		kubeConfig.AuthInfos["auth"].ClientCertificate = f.Name()
		kubeConfig.AuthInfos["auth"].ClientKey = f.Name()

		kubeConfig.Contexts[kubeConfig.CurrentContext].AuthInfo = "auth"

		// when
		config, err := generateKubeConfig("token", "ns", nil, kubeConfig)
		require.NoError(t, err)

		// then
		context := config.Contexts[config.CurrentContext]
		generatedCluster := config.Clusters[context.Cluster]
		generatedAuth := config.AuthInfos[context.AuthInfo]

		assert.Equal(t, []byte("data"), generatedCluster.CertificateAuthorityData)
		assert.Empty(t, generatedCluster.CertificateAuthority)
		assert.Equal(t, []byte("data"), generatedAuth.ClientKeyData)
		assert.Empty(t, generatedAuth.ClientKey)
		assert.Equal(t, []byte("data"), generatedAuth.ClientCertificateData)
		assert.Empty(t, generatedAuth.ClientCertificate)
	})
}

func mockCreateToolchainClusterInNamespaceWithReadyCondition(t *testing.T, fakeClient *test.FakeClient, namespace string) {
	fakeClient.MockCreate = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.CreateOption) error {
		if obj, ok := obj.(*toolchainv1alpha1.ToolchainCluster); ok {
			if obj.GetNamespace() == namespace {
				fillStatusWithDetailsAndReadyCondition(t, obj)
			}
		}
		return fakeClient.Client.Create(ctx, obj, opts...)
	}
}

func mockCreateToolchainClusterWithReadyCondition(t *testing.T, fakeClient *test.FakeClient) {
	fakeClient.MockCreate = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.CreateOption) error {
		if obj, ok := obj.(*toolchainv1alpha1.ToolchainCluster); ok {
			fillStatusWithDetailsAndReadyCondition(t, obj)
		}
		return fakeClient.Client.Create(ctx, obj, opts...)
	}
}

func fillStatusWithDetailsAndReadyCondition(t *testing.T, obj *toolchainv1alpha1.ToolchainCluster) {
	var operatorNamespace string
	switch obj.GetNamespace() {
	case test.HostOperatorNs:
		operatorNamespace = test.MemberOperatorNs
	case test.MemberOperatorNs:
		operatorNamespace = test.HostOperatorNs
	default:
		// If we get here, there is a logic error in the test. Let's fail the test unequivocally.
		assert.Fail(t, "the mock create of ToolchainCluster only works in host operator namespace or member operator namespace but the creation of toolchain cluster was requested in another namespace", "host_ns", test.HostOperatorNs, "member_ns", test.MemberOperatorNs, "obj_ns", obj.GetNamespace())
	}
	obj.Status = toolchainv1alpha1.ToolchainClusterStatus{
		APIEndpoint:       "https://cool-server.com",
		OperatorNamespace: operatorNamespace,
		Conditions: []toolchainv1alpha1.Condition{
			{
				Type:   toolchainv1alpha1.ConditionReady,
				Status: corev1.ConditionTrue,
			},
		},
	}
}

func verifyToolchainClusterSecret(t *testing.T, fakeClient *test.FakeClient, saName, secretNamespace, ctxNamespace, tcName string) {
	secret := &corev1.Secret{}
	secretName := fmt.Sprintf("%s-%s", saName, tcName)
	require.NoError(t, fakeClient.Get(context.TODO(), runtimeclient.ObjectKey{Namespace: secretNamespace, Name: secretName}, secret))
	assert.NotEmpty(t, secret.Labels)
	assert.Equal(t, tcName, secret.Labels[toolchainv1alpha1.ToolchainClusterLabel])
	assert.NotEmpty(t, secret.StringData["token"])
	require.Equal(t, fmt.Sprintf("token-secret-for-%s", saName), secret.StringData["token"])
	assert.NotEmpty(t, secret.StringData["kubeconfig"])
	apiConfig, err := clientcmd.Load([]byte(secret.StringData["kubeconfig"]))
	require.NoError(t, err)
	require.False(t, clientcmdapi.IsConfigEmpty(apiConfig))
	assert.Equal(t, "https://cool-server.com", apiConfig.Clusters["cluster"].Server)
	assert.False(t, apiConfig.Clusters["cluster"].InsecureSkipTLSVerify) // by default the insecure flag is not being set
	assert.Equal(t, "cluster", apiConfig.Contexts["ctx"].Cluster)
	assert.Equal(t, ctxNamespace, apiConfig.Contexts["ctx"].Namespace)
	assert.NotEmpty(t, apiConfig.AuthInfos["auth"].Token)
	require.Equal(t, fmt.Sprintf("token-secret-for-%s", saName), apiConfig.AuthInfos["auth"].Token)
}

func newFakeClientsFromRestConfig(t *testing.T, initObjs ...runtimeclient.Object) (newClientFromRestConfigFunc, *test.FakeClient) {
	fakeClient := test.NewFakeClient(t, initObjs...)
	fakeClient.MockCreate = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.CreateOption) error {
		return fakeClient.Client.Create(ctx, obj, opts...)
	}
	fakeClient.MockUpdate = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.UpdateOption) error {
		return fakeClient.Client.Update(ctx, obj, opts...)
	}
	return func(cfg *rest.Config) (runtimeclient.Client, error) {
		assert.Contains(t, cfg.Host, "https")
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

func newRegisterMemberArgsWith(hostKubeconfig, memberKubeconfig string, skipTlsVerify bool) registerMemberArgs {
	args := defaultRegisterMemberArgs()
	args.hostKubeConfig = hostKubeconfig
	args.memberKubeConfig = memberKubeconfig
	args.skipTlsVerify = &skipTlsVerify
	args.waitForReadyTimeout = 1 * time.Second
	return args
}

func newRegisterMemberArgsWithSuffix(hostKubeconfig, memberKubeconfig string, skipTlsVerify bool, nameSuffix string) registerMemberArgs {
	args := defaultRegisterMemberArgs()
	args.hostKubeConfig = hostKubeconfig
	args.memberKubeConfig = memberKubeconfig
	args.skipTlsVerify = &skipTlsVerify
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
	args.skipTlsVerify = pointer.To(true)

	return args
}

func mockRestartReg(ctx *clicontext.CommandContext, clusterName string, _ ConfigFlagsAndClientGetterFunc) error {
	if clusterName == "host" && ctx != nil {
		return nil
	}
	return fmt.Errorf("cluster name is wrong")
}

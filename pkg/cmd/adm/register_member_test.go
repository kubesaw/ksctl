package adm

import (
	"context"
	"fmt"
	"strings"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/codeready-toolchain/toolchain-common/pkg/test/config"
	_ "github.com/kubesaw/ksctl/pkg/client"
	"github.com/kubesaw/ksctl/pkg/configuration"
	clicontext "github.com/kubesaw/ksctl/pkg/context"
	. "github.com/kubesaw/ksctl/pkg/test"

	"github.com/h2non/gock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/types"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	hostKubeconfig   = "/path/to/host-kubeconfig"
	memberKubeconfig = "/path/to/member-kubeconfig"
)

func TestRegisterMember(t *testing.T) {
	// given
	SetFileConfig(t, Host(), Member())
	gock.New(AddClusterScriptDomain).
		Get(AddClusterScriptPath).
		Persist().
		Reply(200)
	defer gock.OffAll()

	hostArgs := []string{"--type", "host", "--host-kubeconfig", "/path/to/host-kubeconfig", "--member-kubeconfig", "/path/to/member-kubeconfig", "--lets-encrypt"}
	memberArgs := []string{"--type", "member", "--host-kubeconfig", "/path/to/host-kubeconfig", "--member-kubeconfig", "/path/to/member-kubeconfig", "--lets-encrypt"}
	var counter int
	ocCommandCreator := NewCommandCreator(t, "echo", "bash",
		func(t *testing.T, args ...string) {
			if counter == 0 {
				AssertFirstArgPrefixRestEqual("(.*)/add-cluster-(.*)", hostArgs...)(t, args...)
			} else {
				AssertFirstArgPrefixRestEqual("(.*)/add-cluster-(.*)", memberArgs...)(t, args...)
			}
			counter++
		})
	hostDeploymentName := test.NamespacedName("toolchain-host-operator", "host-operator-controller-manager")
	deployment := newDeployment(hostDeploymentName, 1)
	deployment.Labels = map[string]string{"olm.owner.namespace": "toolchain-host-operator"}

	t.Run("When automatic approval is enabled", func(t *testing.T) {
		term := NewFakeTerminalWithResponse("Y")
		toolchainConfig := config.NewToolchainConfigObj(t, config.AutomaticApproval().Enabled(true))
		newClient, fakeClient := NewFakeClients(t, toolchainConfig, deployment)
		numberOfUpdateCalls := 0
		fakeClient.MockUpdate = whenDeploymentThenUpdated(t, fakeClient, hostDeploymentName, 1, &numberOfUpdateCalls)
		ctx := clicontext.NewCommandContext(term, newClient)
		counter = 0

		// when
		err := registerMemberCluster(ctx, ocCommandCreator, hostKubeconfig, memberKubeconfig)

		// then
		require.NoError(t, err)
		// on Linux, the output contains `Command to be called: bash /tmp/add-cluster-`
		// on macOS, the output contains something like `Command to be called: bash /var/folders/b8/wy8kq7_179l7yswz6gz6qx800000gp/T/add-cluster-369107288.sh`
		assert.Contains(t, term.Output(), "Command to be called: bash ")
		assert.Contains(t, term.Output(), "add-cluster-")
		assert.Contains(t, term.Output(), strings.Join(hostArgs, " "))
		assert.Contains(t, term.Output(), strings.Join(memberArgs, " "))
		assert.Equal(t, 2, counter)

		enabled := false
		toolchainConfig.Spec.Host.AutomaticApproval.Enabled = &enabled
		AssertToolchainConfigHasSpec(t, fakeClient, test.NamespacedName(toolchainConfig.Namespace, toolchainConfig.Name), toolchainConfig.Spec)
		assert.Contains(t, term.Output(), "!!! WARNING !!!")
		assert.Contains(t, term.Output(), "The automatic approval was disabled!")
		assert.Contains(t, term.Output(), "Configure the new member cluster in ToolchainConfig and apply the changes to the cluster.")

		AssertDeploymentHasReplicas(t, fakeClient, hostDeploymentName, 1)
		assert.Equal(t, 2, numberOfUpdateCalls)
	})

	t.Run("When toolchainConfig is not present", func(t *testing.T) {
		term := NewFakeTerminalWithResponse("Y")
		newClient, fakeClient := NewFakeClients(t, deployment)
		fakeClient.MockUpdate = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.UpdateOption) error {
			if _, ok := obj.(*toolchainv1alpha1.ToolchainConfig); ok {
				return fmt.Errorf("should not be called")
			}
			return fakeClient.Client.Update(ctx, obj, opts...)
		}
		ctx := clicontext.NewCommandContext(term, newClient)
		counter = 0

		// when
		err := registerMemberCluster(ctx, ocCommandCreator, hostKubeconfig, memberKubeconfig)

		// then
		require.NoError(t, err)
		assert.Equal(t, 2, counter)

		AssertToolchainConfigDoesNotExist(t, fakeClient, test.NamespacedName(test.HostOperatorNs, "config"))
		assert.Contains(t, term.Output(), "!!! WARNING !!!")
		assert.Contains(t, term.Output(), "The automatic approval was disabled!")
		assert.Contains(t, term.Output(), "Configure the new member cluster in ToolchainConfig and apply the changes to the cluster.")
	})

	t.Run("When automatic approval is disabled", func(t *testing.T) {
		term := NewFakeTerminalWithResponse("Y")
		toolchainConfig := config.NewToolchainConfigObj(t, config.AutomaticApproval().Enabled(false))
		newClient, fakeClient := NewFakeClients(t, toolchainConfig, deployment)
		fakeClient.MockUpdate = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.UpdateOption) error {
			if _, ok := obj.(*toolchainv1alpha1.ToolchainConfig); ok {
				return fmt.Errorf("should not be called")
			}
			return fakeClient.Client.Update(ctx, obj, opts...)
		}
		ctx := clicontext.NewCommandContext(term, newClient)
		counter = 0

		// when
		err := registerMemberCluster(ctx, ocCommandCreator, hostKubeconfig, memberKubeconfig)

		// then
		require.NoError(t, err)
		assert.Equal(t, 2, counter)

		enabled := false
		toolchainConfig.Spec.Host.AutomaticApproval.Enabled = &enabled
		AssertToolchainConfigHasSpec(t, fakeClient, test.NamespacedName(toolchainConfig.Namespace, toolchainConfig.Name), toolchainConfig.Spec)
		assert.Contains(t, term.Output(), "!!! WARNING !!!")
		assert.Contains(t, term.Output(), "The automatic approval was disabled!")
		assert.Contains(t, term.Output(), "Configure the new member cluster in ToolchainConfig and apply the changes to the cluster.")
	})

	t.Run("When there are two ToolchainConfigs", func(t *testing.T) {
		term := NewFakeTerminalWithResponse("Y")
		toolchainConfig := config.NewToolchainConfigObj(t, config.AutomaticApproval().Enabled(false))
		toolchainConfig2 := config.NewToolchainConfigObj(t, config.AutomaticApproval().Enabled(true))
		toolchainConfig2.Name = "config2"
		newClient, fakeClient := NewFakeClients(t, toolchainConfig, toolchainConfig2, deployment)
		fakeClient.MockUpdate = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.UpdateOption) error {
			return fmt.Errorf("should not be called")
		}
		ctx := clicontext.NewCommandContext(term, newClient)
		counter = 0

		// when
		err := registerMemberCluster(ctx, ocCommandCreator, hostKubeconfig, memberKubeconfig)

		// then
		require.Error(t, err)
		assert.Equal(t, 0, counter)
	})
}

func TestRunAddClusterScriptSuccess(t *testing.T) {
	// given
	SetFileConfig(t, Host(), Member())
	gock.New(AddClusterScriptDomain).
		Get(AddClusterScriptPath).
		Persist().
		Reply(200)
	defer gock.OffAll()
	term := NewFakeTerminalWithResponse("Y")

	for _, clusterType := range configuration.ClusterTypes {
		t.Run("for cluster name: "+clusterType.String(), func(t *testing.T) {
			// given

			expArgs := []string{"--type", clusterType.String(), "--host-kubeconfig", "/path/to/host-kubeconfig", "--member-kubeconfig", "/path/to/member-kubeconfig", "--lets-encrypt"}
			ocCommandCreator := NewCommandCreator(t, "echo", "bash",
				AssertFirstArgPrefixRestEqual("(.*)/add-cluster-(.*)", expArgs...))

			// when
			err := runAddClusterScript(term, ocCommandCreator, clusterType, hostKubeconfig, memberKubeconfig)

			// then
			require.NoError(t, err)
			// on Linux, the output contains `Command to be called: bash /tmp/add-cluster-`
			// on macOS, the output contains something like `Command to be called: bash /var/folders/b8/wy8kq7_179l7yswz6gz6qx800000gp/T/add-cluster-369107288.sh`
			assert.Contains(t, term.Output(), "Command to be called: bash ")
			assert.Contains(t, term.Output(), "add-cluster-")
			assert.Contains(t, term.Output(), strings.Join(expArgs, " "))
		})
	}
}

func TestRunAddClusterScriptFailed(t *testing.T) {
	// given
	SetFileConfig(t, Host(), Member())
	gock.New(AddClusterScriptDomain).
		Get(AddClusterScriptPath).
		Persist().
		Reply(404)
	defer gock.OffAll()

	for _, clusterType := range configuration.ClusterTypes {

		t.Run("for cluster name: "+clusterType.String(), func(t *testing.T) {
			// given
			expArgs := []string{"--type", clusterType.String(), "--host-kubeconfig", "/path/to/host-kubeconfig", "--member-kubeconfig", "/path/to/member-kubeconfig", "--lets-encrypt"}
			ocCommandCreator := NewCommandCreator(t, "echo", "bash",
				AssertFirstArgPrefixRestEqual("(.*)/add-cluster-(.*)", expArgs...))
			term := NewFakeTerminalWithResponse("Y")

			// when
			err := runAddClusterScript(term, ocCommandCreator, clusterType, hostKubeconfig, memberKubeconfig)

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

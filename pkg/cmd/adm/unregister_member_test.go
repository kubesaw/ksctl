package adm

import (
	"fmt"
	"testing"

	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	clicontext "github.com/kubesaw/ksctl/pkg/context"
	. "github.com/kubesaw/ksctl/pkg/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnregisterMemberWhenAnswerIsY(t *testing.T) {
	// given
	toolchainCluster := NewToolchainCluster(ToolchainClusterName("member-cool-server.com"))
	hostDeploymentName := test.NamespacedName("toolchain-host-operator", "host-operator-controller-manager")
	deployment := newDeployment(hostDeploymentName, 1)
	deployment.Labels = map[string]string{"kubesaw-control-plane": "kubesaw-controller-manager"}

	newClient, fakeClient := NewFakeClients(t, toolchainCluster, deployment)
	numberOfUpdateCalls := 0
	fakeClient.MockUpdate = whenDeploymentThenUpdated(t, fakeClient, hostDeploymentName, 1, &numberOfUpdateCalls)

	SetFileConfig(t, Host(), Member())
	term := NewFakeTerminalWithResponse("y")
	ctx := clicontext.NewCommandContext(term, newClient)

	// when
	err := UnregisterMemberCluster(ctx, "member1", func(ctx *clicontext.CommandContext, clusterName string) error {
		return nil
	})

	// then
	require.NoError(t, err)
	AssertToolchainClusterDoesNotExist(t, fakeClient, toolchainCluster)
	assert.Contains(t, term.Output(), "!!!  DANGER ZONE  !!!")
	assert.NotContains(t, term.Output(), "THIS COMMAND WILL CAUSE UNREGISTER MEMBER CLUSTER FORM HOST CLUSTER. MAKE SURE THERE IS NO USERS LEFT IN THE MEMBER CLUSTER BEFORE UNREGISTERING IT")
	assert.Contains(t, term.Output(), "Delete Member cluster stated above from the Host cluster?")
	assert.Contains(t, term.Output(), "The deletion of the Toolchain member cluster from the Host cluster has been triggered")
	assert.NotContains(t, term.Output(), "cool-token")
	AssertDeploymentHasReplicas(t, fakeClient, hostDeploymentName, 1)
}

func TestUnregisterMemberWhenRestartError(t *testing.T) {
	// given
	toolchainCluster := NewToolchainCluster(ToolchainClusterName("member-cool-server.com"))
	hostDeploymentName := test.NamespacedName("toolchain-host-operator", "host-operator-controller-manager")
	deployment := newDeployment(hostDeploymentName, 1)
	deployment.Labels = map[string]string{"kubesaw-control-plane": "kubesaw-controller-manager"}

	newClient, fakeClient := NewFakeClients(t, toolchainCluster, deployment)
	numberOfUpdateCalls := 0
	fakeClient.MockUpdate = whenDeploymentThenUpdated(t, fakeClient, hostDeploymentName, 1, &numberOfUpdateCalls)

	SetFileConfig(t, Host(), Member())
	term := NewFakeTerminalWithResponse("y")
	ctx := clicontext.NewCommandContext(term, newClient)

	// when
	err := UnregisterMemberCluster(ctx, "member1", func(ctx *clicontext.CommandContext, clusterName string) error {
		return fmt.Errorf("restart did not happen")
	})

	// then
	require.EqualError(t, err, "restart did not happen")
}

func TestUnregisterMemberCallsRestart(t *testing.T) {
	// given
	toolchainCluster := NewToolchainCluster(ToolchainClusterName("member-cool-server.com"))
	hostDeploymentName := test.NamespacedName("toolchain-host-operator", "host-operator-controller-manager")
	deployment := newDeployment(hostDeploymentName, 1)
	deployment.Labels = map[string]string{"kubesaw-control-plane": "kubesaw-controller-manager"}

	newClient, fakeClient := NewFakeClients(t, toolchainCluster, deployment)
	numberOfUpdateCalls := 0
	fakeClient.MockUpdate = whenDeploymentThenUpdated(t, fakeClient, hostDeploymentName, 1, &numberOfUpdateCalls)

	SetFileConfig(t, Host(), Member())
	term := NewFakeTerminalWithResponse("y")
	ctxAct := clicontext.NewCommandContext(term, newClient)

	// when
	err := UnregisterMemberCluster(ctxAct, "member1", func(ctx *clicontext.CommandContext, restartClusterName string) error {
		return mockRestart(ctx, restartClusterName)
	})

	// then
	require.NoError(t, err)
}

func TestUnregisterMemberWhenAnswerIsN(t *testing.T) {
	// given
	toolchainCluster := NewToolchainCluster(ToolchainClusterName("member-cool-server.com"))
	newClient, fakeClient := NewFakeClients(t, toolchainCluster)
	SetFileConfig(t, Host(), Member())
	term := NewFakeTerminalWithResponse("n")
	ctx := clicontext.NewCommandContext(term, newClient)

	// when
	err := UnregisterMemberCluster(ctx, "member1", func(ctx *clicontext.CommandContext, clusterName string) error {
		return nil
	})

	// then
	require.NoError(t, err)
	AssertToolchainClusterSpec(t, fakeClient, toolchainCluster)
	assert.Contains(t, term.Output(), "!!!  DANGER ZONE  !!!")
	assert.NotContains(t, term.Output(), "THIS COMMAND WILL CAUSE UNREGISTER MEMBER CLUSTER FORM HOST CLUSTER. MAKE SURE THERE IS NO USERS LEFT IN THE MEMBER CLUSTER BEFORE UNREGISTERING IT")
	assert.Contains(t, term.Output(), "Delete Member cluster stated above from the Host cluster?")
	assert.NotContains(t, term.Output(), "The deletion of the Toolchain member cluster from the Host cluster has been triggered")
	assert.NotContains(t, term.Output(), "cool-token")
}

func TestUnregisterMemberWhenNotFound(t *testing.T) {
	// given
	toolchainCluster := NewToolchainCluster(ToolchainClusterName("another-cool-server.com"))
	newClient, fakeClient := NewFakeClients(t, toolchainCluster)
	SetFileConfig(t, Host(), Member())
	term := NewFakeTerminalWithResponse("n")
	ctx := clicontext.NewCommandContext(term, newClient)

	// when
	err := UnregisterMemberCluster(ctx, "member1", func(ctx *clicontext.CommandContext, clusterName string) error {
		return nil
	})

	// then
	require.EqualError(t, err, "toolchainclusters.toolchain.dev.openshift.com \"member-cool-server.com\" not found")
	AssertToolchainClusterSpec(t, fakeClient, toolchainCluster)
	assert.NotContains(t, term.Output(), "!!!  DANGER ZONE  !!!")
	assert.NotContains(t, term.Output(), "THIS COMMAND WILL CAUSE UNREGISTER MEMBER CLUSTER FORM HOST CLUSTER. MAKE SURE THERE IS NO USERS LEFT IN THE MEMBER CLUSTER BEFORE UNREGISTERING IT")
	assert.NotContains(t, term.Output(), "Delete Member cluster stated above from the Host cluster?")
	assert.NotContains(t, term.Output(), "The deletion of the Toolchain member cluster from the Host cluster has been triggered")
	assert.NotContains(t, term.Output(), "cool-token")
}

func TestUnregisterMemberWhenUnknownClusterName(t *testing.T) {
	// given
	toolchainCluster := NewToolchainCluster(ToolchainClusterName("member-cool-server.com"))
	newClient, fakeClient := NewFakeClients(t, toolchainCluster)
	SetFileConfig(t, Host(), Member())
	term := NewFakeTerminalWithResponse("n")
	ctx := clicontext.NewCommandContext(term, newClient)

	// when
	err := UnregisterMemberCluster(ctx, "some", func(ctx *clicontext.CommandContext, clusterName string) error {
		return nil
	})

	// then
	require.Error(t, err)
	assert.Contains(t, err.Error(), "the provided cluster-name 'some' is not present in your ksctl.yaml file.")
	AssertToolchainClusterSpec(t, fakeClient, toolchainCluster)
	assert.NotContains(t, term.Output(), "!!!  DANGER ZONE  !!!")
	assert.NotContains(t, term.Output(), "THIS COMMAND WILL CAUSE UNREGISTER MEMBER CLUSTER FORM HOST CLUSTER. MAKE SURE THERE IS NO USERS LEFT IN THE MEMBER CLUSTER BEFORE UNREGISTERING IT")
	assert.NotContains(t, term.Output(), "Delete Member cluster stated above from the Host cluster?")
	assert.NotContains(t, term.Output(), "The deletion of the Toolchain member cluster from the Host cluster has been triggered")
	assert.NotContains(t, term.Output(), "cool-token")
}

func TestUnregisterMemberLacksPermissions(t *testing.T) {
	// given
	SetFileConfig(t, Host(NoToken()), Member(NoToken()))

	toolchainCluster := NewToolchainCluster(ToolchainClusterName("member-cool-server.com"))
	newClient, fakeClient := NewFakeClients(t, toolchainCluster)
	term := NewFakeTerminal()
	ctx := clicontext.NewCommandContext(term, newClient)

	// when
	err := UnregisterMemberCluster(ctx, "member1", func(ctx *clicontext.CommandContext, clusterName string) error {
		return nil
	})

	// then
	require.EqualError(t, err, "ksctl command failed: the token in your ksctl.yaml file is missing")
	AssertToolchainClusterSpec(t, fakeClient, toolchainCluster)
}

func mockRestart(ctx *clicontext.CommandContext, clusterName string) error {
	if clusterName == "host" && ctx != nil {
		return nil
	}
	return fmt.Errorf("cluster name is wrong")
}

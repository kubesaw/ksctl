package adm

import (
	"bytes"
	"os"
	"testing"

	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	clicontext "github.com/kubesaw/ksctl/pkg/context"
	"github.com/kubesaw/ksctl/pkg/ioutils"
	. "github.com/kubesaw/ksctl/pkg/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnregisterMemberWhenAnswerIsY(t *testing.T) {
	// given
	toolchainCluster := NewToolchainCluster(ToolchainClusterName("member-cool-server.com"))
	hostDeploymentName := test.NamespacedName("toolchain-host-operator", "host-operator-controller-manager")
	deployment := newDeployment(hostDeploymentName, 1)
	deployment.Labels = map[string]string{"olm.owner.namespace": "toolchain-host-operator"}

	newClient, fakeClient := NewFakeClients(t, toolchainCluster, deployment)
	numberOfUpdateCalls := 0
	fakeClient.MockUpdate = whenDeploymentThenUpdated(t, fakeClient, hostDeploymentName, 1, &numberOfUpdateCalls)

	SetFileConfig(t, Host(), Member())
	buffy := bytes.NewBuffer(nil)
	term := ioutils.NewTerminal(buffy, buffy, ioutils.WithDefaultConfirm(true))
	ctx := clicontext.NewCommandContext(term, newClient)

	// when
	err := UnregisterMemberCluster(ctx, "member1")

	// then
	require.NoError(t, err)
	AssertToolchainClusterDoesNotExist(t, fakeClient, toolchainCluster)
	assert.Contains(t, buffy.String(), "!!!  DANGER ZONE  !!!")
	assert.NotContains(t, buffy.String(), "THIS COMMAND WILL CAUSE UNREGISTER MEMBER CLUSTER FORM HOST CLUSTER. MAKE SURE THERE IS NO USERS LEFT IN THE MEMBER CLUSTER BEFORE UNREGISTERING IT")
	assert.Contains(t, buffy.String(), "The deletion of the Toolchain member cluster from the Host cluster has been triggered")
	assert.NotContains(t, buffy.String(), "cool-token")

	AssertDeploymentHasReplicas(t, fakeClient, hostDeploymentName, 1)
	assert.Equal(t, 2, numberOfUpdateCalls)
}

func TestUnregisterMemberWhenAnswerIsN(t *testing.T) {
	// given
	toolchainCluster := NewToolchainCluster(ToolchainClusterName("member-cool-server.com"))
	newClient, fakeClient := NewFakeClients(t, toolchainCluster)
	SetFileConfig(t, Host(), Member())
	buffy := bytes.NewBuffer(nil)
	term := ioutils.NewTerminal(buffy, buffy, ioutils.WithDefaultConfirm(false), ioutils.WithTee(os.Stdout))
	ctx := clicontext.NewCommandContext(term, newClient)

	// when
	err := UnregisterMemberCluster(ctx, "member1")

	// then
	require.NoError(t, err)
	AssertToolchainClusterSpec(t, fakeClient, toolchainCluster)
	assert.Contains(t, buffy.String(), "!!!  DANGER ZONE  !!!")
	assert.NotContains(t, buffy.String(), "THIS COMMAND WILL CAUSE UNREGISTER MEMBER CLUSTER FORM HOST CLUSTER. MAKE SURE THERE IS NO USERS LEFT IN THE MEMBER CLUSTER BEFORE UNREGISTERING IT")
	assert.NotContains(t, buffy.String(), "The deletion of the Toolchain member cluster from the Host cluster has been triggered")
	assert.NotContains(t, buffy.String(), "cool-token")
}

func TestUnregisterMemberWhenNotFound(t *testing.T) {
	// given
	toolchainCluster := NewToolchainCluster(ToolchainClusterName("another-cool-server.com"))
	newClient, fakeClient := NewFakeClients(t, toolchainCluster)
	SetFileConfig(t, Host(), Member())
	buffy := bytes.NewBuffer(nil)
	term := ioutils.NewTerminal(buffy, buffy)
	ctx := clicontext.NewCommandContext(term, newClient)

	// when
	err := UnregisterMemberCluster(ctx, "member1")

	// then
	require.EqualError(t, err, "toolchainclusters.toolchain.dev.openshift.com \"member-cool-server.com\" not found")
	AssertToolchainClusterSpec(t, fakeClient, toolchainCluster)
	assert.NotContains(t, buffy.String(), "!!!  DANGER ZONE  !!!")
	assert.NotContains(t, buffy.String(), "THIS COMMAND WILL CAUSE UNREGISTER MEMBER CLUSTER FORM HOST CLUSTER. MAKE SURE THERE IS NO USERS LEFT IN THE MEMBER CLUSTER BEFORE UNREGISTERING IT")
	assert.NotContains(t, buffy.String(), "Delete Member cluster stated above from the Host cluster?")
	assert.NotContains(t, buffy.String(), "The deletion of the Toolchain member cluster from the Host cluster has been triggered")
	assert.NotContains(t, buffy.String(), "cool-token")
}

func TestUnregisterMemberWhenUnknownClusterName(t *testing.T) {
	// given
	toolchainCluster := NewToolchainCluster(ToolchainClusterName("member-cool-server.com"))
	newClient, fakeClient := NewFakeClients(t, toolchainCluster)
	SetFileConfig(t, Host(), Member())
	buffy := bytes.NewBuffer(nil)
	term := ioutils.NewTerminal(buffy, buffy)
	ctx := clicontext.NewCommandContext(term, newClient)

	// when
	err := UnregisterMemberCluster(ctx, "some")

	// then
	require.Error(t, err)
	assert.Contains(t, err.Error(), "the provided cluster-name 'some' is not present in your ksctl.yaml file.")
	AssertToolchainClusterSpec(t, fakeClient, toolchainCluster)
	assert.NotContains(t, buffy.String(), "!!!  DANGER ZONE  !!!")
	assert.NotContains(t, buffy.String(), "THIS COMMAND WILL CAUSE UNREGISTER MEMBER CLUSTER FORM HOST CLUSTER. MAKE SURE THERE IS NO USERS LEFT IN THE MEMBER CLUSTER BEFORE UNREGISTERING IT")
	assert.NotContains(t, buffy.String(), "Delete Member cluster stated above from the Host cluster?")
	assert.NotContains(t, buffy.String(), "The deletion of the Toolchain member cluster from the Host cluster has been triggered")
	assert.NotContains(t, buffy.String(), "cool-token")
}

func TestUnregisterMemberLacksPermissions(t *testing.T) {
	// given
	SetFileConfig(t, Host(NoToken()), Member(NoToken()))

	toolchainCluster := NewToolchainCluster(ToolchainClusterName("member-cool-server.com"))
	newClient, fakeClient := NewFakeClients(t, toolchainCluster)
	buffy := bytes.NewBuffer(nil)
	term := ioutils.NewTerminal(buffy, buffy)
	ctx := clicontext.NewCommandContext(term, newClient)

	// when
	err := UnregisterMemberCluster(ctx, "member1")

	// then
	require.EqualError(t, err, "ksctl command failed: the token in your ksctl.yaml file is missing")
	AssertToolchainClusterSpec(t, fakeClient, toolchainCluster)
}

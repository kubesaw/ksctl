package adm

import (
	"fmt"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	clicontext "github.com/kubesaw/ksctl/pkg/context"
	. "github.com/kubesaw/ksctl/pkg/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestUnregisterMemberWhenAnswerIsY(t *testing.T) {
	// given
	toolchainCluster := NewToolchainCluster(ToolchainClusterName("member-cool-server.com"))
	noiseToolchainCluster := NewToolchainCluster(ToolchainClusterName("noise"))
	secret := newSecret(toolchainCluster)
	noiseSecret := newSecret(noiseToolchainCluster)

	newClient, fakeClient := NewFakeClients(t, noiseToolchainCluster, toolchainCluster, secret, noiseSecret)

	SetFileConfig(t, Host(), Member())
	term := NewFakeTerminalWithResponse("y")
	ctx := clicontext.NewCommandContext(term, newClient)

	// when
	err := UnregisterMemberCluster(ctx, "member1", func(_ *clicontext.CommandContext, _ string, _ ConfigFlagsAndClientGetterFunc) error {
		return nil
	})

	// then
	require.NoError(t, err)
	AssertToolchainClusterDoesNotExist(t, fakeClient, toolchainCluster)
	AssertObjectExists(t, fakeClient, client.ObjectKeyFromObject(noiseToolchainCluster), &toolchainv1alpha1.ToolchainCluster{})
	AssertObjectExists(t, fakeClient, client.ObjectKeyFromObject(noiseSecret), &v1.Secret{})
	assert.Contains(t, term.Output(), "!!!  DANGER ZONE  !!!")
	assert.NotContains(t, term.Output(), "THIS COMMAND WILL CAUSE UNREGISTER MEMBER CLUSTER FORM HOST CLUSTER. MAKE SURE THERE IS NO USERS LEFT IN THE MEMBER CLUSTER BEFORE UNREGISTERING IT")
	assert.Contains(t, term.Output(), "Delete Member cluster stated above from the Host cluster?")
	assert.Contains(t, term.Output(), "The deletion of the Member cluster from the Host cluster has been finished.")
	assert.NotContains(t, term.Output(), "cool-token")
}

func TestUnregisterMemberWhenSecretIsMissing(t *testing.T) {
	// given
	toolchainCluster := NewToolchainCluster(ToolchainClusterName("member-cool-server.com"))
	newClient, fakeClient := NewFakeClients(t, toolchainCluster)

	SetFileConfig(t, Host(), Member())
	term := NewFakeTerminalWithResponse("y")
	ctx := clicontext.NewCommandContext(term, newClient)

	// when
	err := UnregisterMemberCluster(ctx, "member1", func(_ *clicontext.CommandContext, _ string, _ ConfigFlagsAndClientGetterFunc) error {
		return nil
	})

	// then
	require.NoError(t, err)
	AssertToolchainClusterDoesNotExist(t, fakeClient, toolchainCluster)
	assert.NotContains(t, term.Output(), "cool-token")
}

func newSecret(tc *toolchainv1alpha1.ToolchainCluster) *v1.Secret {
	return &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      tc.Spec.SecretRef.Name,
			Namespace: test.HostOperatorNs,
		},
	}
}

func TestUnregisterMemberWhenRestartError(t *testing.T) {
	// given
	toolchainCluster := NewToolchainCluster(ToolchainClusterName("member-cool-server.com"))
	secret := newSecret(toolchainCluster)
	newClient, _ := NewFakeClients(t, toolchainCluster, secret)

	SetFileConfig(t, Host(), Member())
	term := NewFakeTerminalWithResponse("y")
	ctx := clicontext.NewCommandContext(term, newClient)

	// when
	err := UnregisterMemberCluster(ctx, "member1", func(_ *clicontext.CommandContext, _ string, _ ConfigFlagsAndClientGetterFunc) error {
		return fmt.Errorf("restart did not happen")
	})

	// then
	require.EqualError(t, err, "restart did not happen")
}

func TestUnregisterMemberCallsRestart(t *testing.T) {
	// given
	toolchainCluster := NewToolchainCluster(ToolchainClusterName("member-cool-server.com"))
	secret := newSecret(toolchainCluster)
	newClient, _ := NewFakeClients(t, toolchainCluster, secret)

	SetFileConfig(t, Host(), Member())
	term := NewFakeTerminalWithResponse("y")
	ctxAct := clicontext.NewCommandContext(term, newClient)
	called := 0
	// when
	err := UnregisterMemberCluster(ctxAct, "member1", func(ctx *clicontext.CommandContext, restartClusterName string, _ ConfigFlagsAndClientGetterFunc) error {
		called++
		return mockRestart(ctx, restartClusterName, getConfigFlagsAndClient)
	})

	// then
	require.NoError(t, err)
	assert.Equal(t, 1, called)
}

func TestUnregisterMemberWhenAnswerIsN(t *testing.T) {
	// given
	toolchainCluster := NewToolchainCluster(ToolchainClusterName("member-cool-server.com"))
	secret := newSecret(toolchainCluster)
	newClient, fakeClient := NewFakeClients(t, toolchainCluster, secret)
	SetFileConfig(t, Host(), Member())
	term := NewFakeTerminalWithResponse("n")
	ctx := clicontext.NewCommandContext(term, newClient)

	// when
	err := UnregisterMemberCluster(ctx, "member1", func(_ *clicontext.CommandContext, _ string, _ ConfigFlagsAndClientGetterFunc) error {
		return nil
	})

	// then
	require.NoError(t, err)
	AssertToolchainClusterSpec(t, fakeClient, toolchainCluster)
	AssertObjectExists(t, fakeClient, client.ObjectKeyFromObject(secret), &v1.Secret{})
	assert.Contains(t, term.Output(), "!!!  DANGER ZONE  !!!")
	assert.NotContains(t, term.Output(), "THIS COMMAND WILL CAUSE UNREGISTER MEMBER CLUSTER FORM HOST CLUSTER. MAKE SURE THERE IS NO USERS LEFT IN THE MEMBER CLUSTER BEFORE UNREGISTERING IT")
	assert.Contains(t, term.Output(), "Delete Member cluster stated above from the Host cluster?")
	assert.NotContains(t, term.Output(), "The deletion of the Member cluster from the Host cluster has been finished.")
	assert.NotContains(t, term.Output(), "cool-token")
}

func TestUnregisterMemberWhenNotFound(t *testing.T) {
	// given
	toolchainCluster := NewToolchainCluster(ToolchainClusterName("another-cool-server.com"))
	secret := newSecret(toolchainCluster)
	newClient, fakeClient := NewFakeClients(t, toolchainCluster, secret)
	SetFileConfig(t, Host(), Member())
	term := NewFakeTerminalWithResponse("n")
	ctx := clicontext.NewCommandContext(term, newClient)

	// when
	err := UnregisterMemberCluster(ctx, "member1", func(_ *clicontext.CommandContext, _ string, _ ConfigFlagsAndClientGetterFunc) error {
		return nil
	})

	// then
	require.EqualError(t, err, "toolchainclusters.toolchain.dev.openshift.com \"member-cool-server.com\" not found")
	AssertToolchainClusterSpec(t, fakeClient, toolchainCluster)
	AssertObjectExists(t, fakeClient, client.ObjectKeyFromObject(secret), &v1.Secret{})
	assert.NotContains(t, term.Output(), "!!!  DANGER ZONE  !!!")
	assert.NotContains(t, term.Output(), "THIS COMMAND WILL CAUSE UNREGISTER MEMBER CLUSTER FORM HOST CLUSTER. MAKE SURE THERE IS NO USERS LEFT IN THE MEMBER CLUSTER BEFORE UNREGISTERING IT")
	assert.NotContains(t, term.Output(), "Delete Member cluster stated above from the Host cluster?")
	assert.NotContains(t, term.Output(), "The deletion of the Member cluster from the Host cluster has been finished.")
	assert.NotContains(t, term.Output(), "cool-token")
}

func TestUnregisterMemberWhenUnknownClusterName(t *testing.T) {
	// given
	toolchainCluster := NewToolchainCluster(ToolchainClusterName("member-cool-server.com"))
	secret := newSecret(toolchainCluster)
	newClient, fakeClient := NewFakeClients(t, toolchainCluster, secret)
	SetFileConfig(t, Host(), Member())
	term := NewFakeTerminalWithResponse("n")
	ctx := clicontext.NewCommandContext(term, newClient)

	// when
	err := UnregisterMemberCluster(ctx, "some", func(_ *clicontext.CommandContext, _ string, _ ConfigFlagsAndClientGetterFunc) error {
		return nil
	})

	// then
	require.Error(t, err)
	assert.Contains(t, err.Error(), "the provided cluster-name 'some' is not present in your ksctl.yaml file.")
	AssertToolchainClusterSpec(t, fakeClient, toolchainCluster)
	AssertObjectExists(t, fakeClient, client.ObjectKeyFromObject(secret), &v1.Secret{})
	assert.NotContains(t, term.Output(), "!!!  DANGER ZONE  !!!")
	assert.NotContains(t, term.Output(), "THIS COMMAND WILL CAUSE UNREGISTER MEMBER CLUSTER FORM HOST CLUSTER. MAKE SURE THERE IS NO USERS LEFT IN THE MEMBER CLUSTER BEFORE UNREGISTERING IT")
	assert.NotContains(t, term.Output(), "Delete Member cluster stated above from the Host cluster?")
	assert.NotContains(t, term.Output(), "The deletion of the Member cluster from the Host cluster has been finished.")
	assert.NotContains(t, term.Output(), "cool-token")
}

func TestUnregisterMemberLacksPermissions(t *testing.T) {
	// given
	SetFileConfig(t, Host(NoToken()), Member(NoToken()))

	toolchainCluster := NewToolchainCluster(ToolchainClusterName("member-cool-server.com"))
	secret := newSecret(toolchainCluster)
	newClient, fakeClient := NewFakeClients(t, toolchainCluster, secret)
	term := NewFakeTerminal()
	ctx := clicontext.NewCommandContext(term, newClient)

	// when
	err := UnregisterMemberCluster(ctx, "member1", func(_ *clicontext.CommandContext, _ string, _ ConfigFlagsAndClientGetterFunc) error {
		return nil
	})

	// then
	require.EqualError(t, err, "ksctl command failed: the token in your ksctl.yaml file is missing")
	AssertToolchainClusterSpec(t, fakeClient, toolchainCluster)
	AssertObjectExists(t, fakeClient, client.ObjectKeyFromObject(secret), &v1.Secret{})
}

func mockRestart(ctx *clicontext.CommandContext, clusterName string, cfc ConfigFlagsAndClientGetterFunc) error {
	_, _, cfcerr := cfc(ctx, clusterName)
	if clusterName == "host" && ctx != nil && cfcerr == nil {
		return nil
	}

	return fmt.Errorf("cluster name is wrong")
}

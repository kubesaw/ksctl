package adm

import (
	"context"
	"testing"

	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/kubesaw/ksctl/pkg/configuration"
	clicontext "github.com/kubesaw/ksctl/pkg/context"
	. "github.com/kubesaw/ksctl/pkg/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
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
	term := NewFakeTerminalWithResponse("y")
	ctx := clicontext.NewCommandContext(term, newClient)

	// when
	err := UnregisterMemberCluster(ctx, "member1")

	// then
	require.NoError(t, err)
	AssertToolchainClusterDoesNotExist(t, fakeClient, toolchainCluster)
	assert.Contains(t, term.Output(), "!!!  DANGER ZONE  !!!")
	assert.NotContains(t, term.Output(), "THIS COMMAND WILL CAUSE UNREGISTER MEMBER CLUSTER FORM HOST CLUSTER. MAKE SURE THERE IS NO USERS LEFT IN THE MEMBER CLUSTER BEFORE UNREGISTERING IT")
	assert.Contains(t, term.Output(), "Delete Member cluster stated above from the Host cluster?")
	assert.Contains(t, term.Output(), "The deletion of the Toolchain member cluster from the Host cluster has been triggered")
	assert.NotContains(t, term.Output(), "cool-token")

	AssertDeploymentHasReplicas(t, fakeClient, hostDeploymentName, 1)
	assert.Equal(t, 2, numberOfUpdateCalls)
}

func TestUnregisterMemberWhenAnswerIsN(t *testing.T) {
	// given
	toolchainCluster := NewToolchainCluster(ToolchainClusterName("member-cool-server.com"))
	newClient, fakeClient := NewFakeClients(t, toolchainCluster)
	SetFileConfig(t, Host(), Member())
	term := NewFakeTerminalWithResponse("n")
	ctx := clicontext.NewCommandContext(term, newClient)

	// when
	err := UnregisterMemberCluster(ctx, "member1")

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
	err := UnregisterMemberCluster(ctx, "member1")

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
	err := UnregisterMemberCluster(ctx, "some")

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
	err := UnregisterMemberCluster(ctx, "member1")

	// then
	require.EqualError(t, err, "ksctl command failed: the token in your ksctl.yaml file is missing")
	AssertToolchainClusterSpec(t, fakeClient, toolchainCluster)
}

func TestRestartHostOperator(t *testing.T) {
	// given
	SetFileConfig(t, Host())
	term := NewFakeTerminalWithResponse("") // it should not read the input
	cfg, err := configuration.LoadClusterConfig(term, "host")
	require.NoError(t, err)
	namespacedName := types.NamespacedName{
		Namespace: "toolchain-host-operator",
		Name:      "host-operator-controller-manager",
	}

	t.Run("host deployment is present and restart successful", func(t *testing.T) {
		// given
		deployment := newDeployment(namespacedName, 1)
		deployment.Labels = map[string]string{"olm.owner.namespace": "toolchain-host-operator"}
		_, fakeClient := NewFakeClients(t, deployment)
		numberOfUpdateCalls := 0
		fakeClient.MockUpdate = requireDeploymentBeingUpdated(t, fakeClient, namespacedName, 1, &numberOfUpdateCalls)

		// when
		err := restartHostOperator(cfg)

		// then
		require.NoError(t, err)
		AssertDeploymentHasReplicas(t, fakeClient, namespacedName, 1)
		assert.Equal(t, 2, numberOfUpdateCalls)
	})

	t.Run("host deployment with the label is not present - restart fails", func(t *testing.T) {
		// given
		deployment := newDeployment(namespacedName, 1)
		_, fakeClient := NewFakeClients(t, deployment)
		numberOfUpdateCalls := 0
		fakeClient.MockUpdate = requireDeploymentBeingUpdated(t, fakeClient, namespacedName, 1, &numberOfUpdateCalls)

		// when
		err := restartHostOperator(cfg)

		// then
		require.Error(t, err)
		AssertDeploymentHasReplicas(t, fakeClient, namespacedName, 1)
		assert.Equal(t, 0, numberOfUpdateCalls)
	})

	t.Run("there are more deployments with the host operator label - restart fails", func(t *testing.T) {
		// given
		deployment := newDeployment(namespacedName, 1)
		deployment.Labels = map[string]string{"olm.owner.namespace": "toolchain-host-operator"}
		deployment2 := deployment.DeepCopy()
		deployment2.Name = "another"
		_, fakeClient := NewFakeClients(t, deployment, deployment2)
		numberOfUpdateCalls := 0
		fakeClient.MockUpdate = requireDeploymentBeingUpdated(t, fakeClient, namespacedName, 1, &numberOfUpdateCalls)

		// when
		err := restartHostOperator(cfg)

		// then
		require.Error(t, err)
		AssertDeploymentHasReplicas(t, fakeClient, namespacedName, 1)
		assert.Equal(t, 0, numberOfUpdateCalls)
	})
}

func newDeployment(namespacedName types.NamespacedName, replicas int32) *appsv1.Deployment { //nolint:unparam
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespacedName.Namespace,
			Name:      namespacedName.Name,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
		},
	}
}

func requireDeploymentBeingUpdated(t *testing.T, fakeClient *test.FakeClient, namespacedName types.NamespacedName, currentReplicas int32, numberOfUpdateCalls *int) func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.UpdateOption) error {
	return func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.UpdateOption) error {
		deployment, ok := obj.(*appsv1.Deployment)
		require.True(t, ok)
		checkDeploymentBeingUpdated(t, fakeClient, namespacedName, currentReplicas, numberOfUpdateCalls, deployment)
		return fakeClient.Client.Update(ctx, obj, opts...)
	}
}

func checkDeploymentBeingUpdated(t *testing.T, fakeClient *test.FakeClient, namespacedName types.NamespacedName, currentReplicas int32, numberOfUpdateCalls *int, deployment *appsv1.Deployment) {
	// on the first call, we should have a deployment with 3 replicas ("current") and request to scale down to 0 ("requested")
	// on the other calls, it's the opposite
	if *numberOfUpdateCalls == 0 {
		// check the current deployment's replicas field
		AssertDeploymentHasReplicas(t, fakeClient, namespacedName, currentReplicas)
		// check the requested deployment's replicas field
		assert.Equal(t, int32(0), *deployment.Spec.Replicas)
	} else {
		// check the current deployment's replicas field
		AssertDeploymentHasReplicas(t, fakeClient, namespacedName, 0)
		// check the requested deployment's replicas field
		assert.Equal(t, currentReplicas, *deployment.Spec.Replicas)
	}
	*numberOfUpdateCalls++
}

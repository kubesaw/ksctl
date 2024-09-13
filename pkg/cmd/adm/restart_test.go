package adm

import (
	"context"
	"fmt"
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

func TestRestartDeployment(t *testing.T) {
	// given
	SetFileConfig(t, Host(), Member())

	for _, clusterName := range []string{"host", "member1"} {
		clusterType := configuration.Host
		if clusterName != "host" {
			clusterType = configuration.Member
		}
		namespace := fmt.Sprintf("toolchain-%s-operator", clusterType)
		namespacedName := types.NamespacedName{
			Namespace: namespace,
			Name:      "cool-deployment",
		}
		term := NewFakeTerminalWithResponse("Y")

		t.Run("restart is successful for "+clusterName, func(t *testing.T) {
			// given
			deployment := newDeployment(namespacedName, 3)
			newClient, fakeClient := NewFakeClients(t, deployment)
			numberOfUpdateCalls := 0
			fakeClient.MockUpdate = requireDeploymentBeingUpdated(t, fakeClient, namespacedName, 3, &numberOfUpdateCalls)
			ctx := clicontext.NewCommandContext(term, newClient)

			// when
			err := restart(ctx, clusterName, "cool-deployment")

			// then
			require.NoError(t, err)
			AssertDeploymentHasReplicas(t, fakeClient, namespacedName, 3)
		})

		t.Run("list deployments when no deployment name is provided for "+clusterName, func(t *testing.T) {
			// given
			deployment := newDeployment(namespacedName, 3)
			newClient, fakeClient := NewFakeClients(t, deployment)
			numberOfUpdateCalls := 0
			fakeClient.MockUpdate = requireDeploymentBeingUpdated(t, fakeClient, namespacedName, 3, &numberOfUpdateCalls)
			term := NewFakeTerminalWithResponse("Y")
			ctx := clicontext.NewCommandContext(term, newClient)

			// when
			err := restart(ctx, clusterName)

			// then
			require.EqualError(t, err, "please mention one of the following operator names to restart: host | member-1 | member-2")
			AssertDeploymentHasReplicas(t, fakeClient, namespacedName, 3)
			assert.Equal(t, 0, numberOfUpdateCalls)
		})

		t.Run("restart fails - cannot get the deployment for "+clusterName, func(t *testing.T) {
			// given
			deployment := newDeployment(namespacedName, 3)
			newClient, fakeClient := NewFakeClients(t, deployment)
			numberOfUpdateCalls := 0
			fakeClient.MockUpdate = requireDeploymentBeingUpdated(t, fakeClient, namespacedName, 3, &numberOfUpdateCalls)
			fakeClient.MockGet = func(ctx context.Context, key runtimeclient.ObjectKey, obj runtimeclient.Object, opts ...runtimeclient.GetOption) error {
				return fmt.Errorf("some error")
			}
			ctx := clicontext.NewCommandContext(term, newClient)

			// when
			err := restart(ctx, clusterName, "cool-deployment")

			// then
			require.Error(t, err)
			fakeClient.MockGet = nil
			AssertDeploymentHasReplicas(t, fakeClient, namespacedName, 3)
			assert.Equal(t, 0, numberOfUpdateCalls)
		})

		t.Run("restart fails - deployment not found for "+clusterName, func(t *testing.T) {
			// given
			deployment := newDeployment(namespacedName, 3)
			newClient, fakeClient := NewFakeClients(t, deployment)
			numberOfUpdateCalls := 0
			fakeClient.MockUpdate = requireDeploymentBeingUpdated(t, fakeClient, namespacedName, 3, &numberOfUpdateCalls)
			term := NewFakeTerminalWithResponse("Y")
			ctx := clicontext.NewCommandContext(term, newClient)

			// when
			err := restart(ctx, clusterName, "wrong-deployment")

			// then
			require.NoError(t, err)
			AssertDeploymentHasReplicas(t, fakeClient, namespacedName, 3)
			assert.Equal(t, 0, numberOfUpdateCalls)
			assert.Contains(t, term.Output(), "ERROR: The given deployment 'wrong-deployment' wasn't found.")
		})
	}
}

func TestRestartDeploymentWithInsufficientPermissions(t *testing.T) {
	// given
	SetFileConfig(t, Host(NoToken()), Member(NoToken()))
	for _, clusterName := range []string{"host", "member1"} {
		// given
		clusterType := configuration.Host
		if clusterName != "host" {
			clusterType = configuration.Member
		}
		namespace := fmt.Sprintf("toolchain-%s-operator", clusterType)
		namespacedName := types.NamespacedName{
			Namespace: namespace,
			Name:      "cool-deployment",
		}
		deployment := newDeployment(namespacedName, 3)
		newClient, fakeClient := NewFakeClients(t, deployment)
		numberOfUpdateCalls := 0
		fakeClient.MockUpdate = requireDeploymentBeingUpdated(t, fakeClient, namespacedName, 3, &numberOfUpdateCalls)
		term := NewFakeTerminalWithResponse("Y")
		ctx := clicontext.NewCommandContext(term, newClient)

		// when
		err := restart(ctx, clusterName, "cool-deployment")

		// then
		require.Error(t, err)
		assert.Equal(t, 0, numberOfUpdateCalls)
		AssertDeploymentHasReplicas(t, fakeClient, namespacedName, 3)
	}
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
		deployment.Labels = map[string]string{"provider": "codeready-toolchain"}
		newClient, fakeClient := NewFakeClients(t, deployment)
		numberOfUpdateCalls := 0
		fakeClient.MockUpdate = requireDeploymentBeingUpdated(t, fakeClient, namespacedName, 1, &numberOfUpdateCalls)
		ctx := clicontext.NewCommandContext(term, newClient)

		// when
		err := restartDeployment(ctx, fakeClient, cfg.OperatorNamespace)

		// then
		require.NoError(t, err)
		AssertDeploymentHasReplicas(t, fakeClient, namespacedName, 1)
	})

	t.Run("host deployment with the label is not present - restart fails", func(t *testing.T) {
		// given
		deployment := newDeployment(namespacedName, 1)
		newClient, fakeClient := NewFakeClients(t, deployment)
		numberOfUpdateCalls := 0
		fakeClient.MockUpdate = requireDeploymentBeingUpdated(t, fakeClient, namespacedName, 1, &numberOfUpdateCalls)
		ctx := clicontext.NewCommandContext(term, newClient)

		// when
		err := restartDeployment(ctx, fakeClient, cfg.OperatorNamespace)

		// then
		require.NoError(t, err)

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

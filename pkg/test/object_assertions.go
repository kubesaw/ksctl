package test

import (
	"context"
	"testing"

	olmv1 "github.com/operator-framework/api/pkg/operators/v1"
	olmv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// AssertCatalogSourceHasSpec verifies that the there is a CatalogSource resource matching the expected namespace/name, and with the same specs.
func AssertCatalogSourceHasSpec(t *testing.T, fakeClient runtimeclient.Client, namespacedName types.NamespacedName, expected olmv1alpha1.CatalogSourceSpec) {
	actual := &olmv1alpha1.CatalogSource{}
	AssertObjectHasContent(t, fakeClient, namespacedName, actual, func() {
		assert.Equal(t, expected, actual.Spec)
	})
}

// AssertCatalogSourceExists verifies that the there is a CatalogSource resource matching the expected namespace/name
func AssertCatalogSourceExists(t *testing.T, fakeClient runtimeclient.Client, namespacedName types.NamespacedName) {
	actual := &olmv1alpha1.CatalogSource{}
	AssertObjectExists(t, fakeClient, namespacedName, actual)
}

// AssertOperatorGroupExists verifies that the there is a OperatorGroup resource matching the expected namespace/name
func AssertOperatorGroupExists(t *testing.T, fakeClient runtimeclient.Client, namespacedName types.NamespacedName) {
	actual := &olmv1.OperatorGroup{}
	AssertObjectExists(t, fakeClient, namespacedName, actual)
}

func AssertOperatorGroupHasLabels(t *testing.T, fakeClient runtimeclient.Client, namespacedName types.NamespacedName, expected map[string]string) {
	actual := &olmv1.OperatorGroup{}
	AssertObjectHasContent(t, fakeClient, namespacedName, actual, func() {
		assert.Equal(t, expected, actual.Labels)
	})
}

// AssertOperatorGroupDoesNotExist verifies that there is no OperatorGroup resource  with the given namespace/name
func AssertOperatorGroupDoesNotExist(t *testing.T, fakeClient runtimeclient.Client, namespacedName types.NamespacedName) {
	AssertObjectDoesNotExist(t, fakeClient, namespacedName, &olmv1.OperatorGroup{})
}

// AssertSubscriptionHasSpec verifies that the there is a Subscription resource matching the expected namespace/name, and with the same specs.
func AssertSubscriptionHasSpec(t *testing.T, fakeClient runtimeclient.Client, namespacedName types.NamespacedName, expected *olmv1alpha1.SubscriptionSpec) {
	actual := &olmv1alpha1.Subscription{}
	AssertObjectHasContent(t, fakeClient, namespacedName, actual, func() {
		assert.Equal(t, expected, actual.Spec)
	})
}

// AssertSubscriptionExists verifies that the there is a Subscription resource matching the expected namespace/name
func AssertSubscriptionExists(t *testing.T, fakeClient runtimeclient.Client, namespacedName types.NamespacedName) {
	actual := &olmv1alpha1.Subscription{}
	AssertObjectExists(t, fakeClient, namespacedName, actual)
}

// AssertSubscriptionDoesNotExist verifies that there is no Subscription resource  with the given namespace/name
func AssertSubscriptionDoesNotExist(t *testing.T, fakeClient runtimeclient.Client, namespacedName types.NamespacedName) {
	AssertObjectDoesNotExist(t, fakeClient, namespacedName, &olmv1alpha1.Subscription{})
}

// AssertDeploymentHasReplicas verifies that the there is a Deployment resource matching the expected namespace/name, and with the same spec.replicas.
func AssertDeploymentHasReplicas(t *testing.T, fakeClient runtimeclient.Client, namespacedName types.NamespacedName, replicas int32) {
	actual := &appsv1.Deployment{}
	AssertObjectHasContent(t, fakeClient, namespacedName, actual, func() {
		require.NotNil(t, actual.Spec.Replicas)
		assert.Equal(t, replicas, *actual.Spec.Replicas)
	})
}

// AssertObjectHasContent verifies that the there is a resource matching the expected namespace/name, and with the same specs.
func AssertObjectHasContent(t *testing.T, fakeClient runtimeclient.Client, namespacedName types.NamespacedName, actualResource runtimeclient.Object, contentAssertions ...func()) {
	err := fakeClient.Get(context.TODO(), namespacedName, actualResource)
	require.NoError(t, err)
	for _, assertContent := range contentAssertions {
		assertContent()
	}
}

// AssertObjectExists verifies that there is a resource of the given type and with the given namespace/name
func AssertObjectExists(t *testing.T, fakeClient runtimeclient.Client, namespacedName types.NamespacedName, resource runtimeclient.Object) {
	err := fakeClient.Get(context.TODO(), namespacedName, resource)
	require.NoError(t, err)
}

// AssertObjectDoesNotExist verifies that there is no resource of the given type and with the given namespace/name
func AssertObjectDoesNotExist(t *testing.T, fakeClient runtimeclient.Client, namespacedName types.NamespacedName, resource runtimeclient.Object) {
	err := fakeClient.Get(context.TODO(), namespacedName, resource)
	require.Error(t, err)
	require.True(t, apierrors.IsNotFound(err))
}

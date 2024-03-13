package test

import (
	"context"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"

	olmv1 "github.com/operator-framework/api/pkg/operators/v1"
	olmv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
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

// AssertCatalogSourceDoesNotExist verifies that there is no CatalogSource resource  with the given namespace/name
func AssertCatalogSourceDoesNotExist(t *testing.T, fakeClient runtimeclient.Client, namespacedName types.NamespacedName) {
	AssertObjectDoesNotExist(t, fakeClient, namespacedName, &olmv1alpha1.CatalogSource{})
}

// AssertOperatorGroupHasSpec verifies that the there is an OperatorGroup resource matching the expected namespace/name, and with the same specs.
func AssertOperatorGroupHasSpec(t *testing.T, fakeClient runtimeclient.Client, namespacedName types.NamespacedName, expected olmv1.OperatorGroupSpec) {
	actual := &olmv1.OperatorGroup{}
	AssertObjectHasContent(t, fakeClient, namespacedName, actual, func() {
		assert.Equal(t, expected, actual.Spec)
	})
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

// AssertToolchainConfigHasSpec verifies that there is an ToolchainConfig resource matching the expected namespace/name, and with the same spec.
func AssertToolchainConfigHasSpec(t *testing.T, fakeClient runtimeclient.Client, namespacedName types.NamespacedName, expected toolchainv1alpha1.ToolchainConfigSpec) {
	actual := &toolchainv1alpha1.ToolchainConfig{}
	AssertObjectHasContent(t, fakeClient, namespacedName, actual, func() {
		assert.Equal(t, expected, actual.Spec)
	})
}

// AssertToolchainConfigExists verifies that the there is an ToolchainConfig resource matching the expected namespace/name
func AssertToolchainConfigExists(t *testing.T, fakeClient runtimeclient.Client, namespacedName types.NamespacedName) {
	actual := &toolchainv1alpha1.ToolchainConfig{}
	AssertObjectExists(t, fakeClient, namespacedName, actual)
}

// AssertToolchainConfigDoesNotExist verifies that there is no ToolchainConfig resource  with the given namespace/name
func AssertToolchainConfigDoesNotExist(t *testing.T, fakeClient runtimeclient.Client, namespacedName types.NamespacedName) {
	AssertObjectDoesNotExist(t, fakeClient, namespacedName, &toolchainv1alpha1.ToolchainConfig{})
}

// AssertConfigMapHasData verifies that the there is a ConfigMap resource matching the expected namespace/name, and with the same data.
func AssertConfigMapHasData(t *testing.T, fakeClient runtimeclient.Client, namespacedName types.NamespacedName, expected map[string]string) {
	actual := &corev1.ConfigMap{}
	AssertObjectHasContent(t, fakeClient, namespacedName, actual, func() {
		assert.Equal(t, expected, actual.Data)
	})
}

// AssertConfigMapExists verifies that the there is a ConfigMap resource matching the expected namespace/name
func AssertConfigMapExists(t *testing.T, fakeClient runtimeclient.Client, namespacedName types.NamespacedName) {
	actual := &corev1.ConfigMap{}
	AssertObjectExists(t, fakeClient, namespacedName, actual)
}

// AssertConfigMapHasDataEntries verifies that the there is a ConfigMap resource matching the expected namespace/name, and with the given entries in its `data`.
func AssertConfigMapHasDataEntries(t *testing.T, fakeClient runtimeclient.Client, namespacedName types.NamespacedName, expectedEntries ...string) {
	actual := &corev1.ConfigMap{}
	err := fakeClient.Get(context.TODO(), namespacedName, actual)
	require.NoError(t, err)
	require.Len(t, actual.Data, len(expectedEntries))
	for _, e := range expectedEntries {
		assert.Contains(t, actual.Data, e)
	}
}

// AssertConfigMapDoesNotExist verifies that there is no ConfigMap resource  with the given namespace/name
func AssertConfigMapDoesNotExist(t *testing.T, fakeClient runtimeclient.Client, namespacedName types.NamespacedName) {
	AssertObjectDoesNotExist(t, fakeClient, namespacedName, &corev1.ConfigMap{})
}

// AssertSecretHasData verifies that the there is a Secret resource matching the expected namespace/name, and with the same data.
func AssertSecretHasData(t *testing.T, fakeClient runtimeclient.Client, namespacedName types.NamespacedName, expected map[string][]byte) {
	actual := &corev1.Secret{}
	AssertObjectHasContent(t, fakeClient, namespacedName, actual, func() {
		assert.Equal(t, expected, actual.Data)
	})
}

// AssertSecretExists verifies that the there is a Secret resource matching the expected namespace/name
func AssertSecretExists(t *testing.T, fakeClient runtimeclient.Client, namespacedName types.NamespacedName) {
	actual := &corev1.Secret{}
	AssertObjectExists(t, fakeClient, namespacedName, actual)
}

// AssertSecretHasDataEntries verifies that the there is a Secret resource matching the expected namespace/name, and with the given entries in its `data`.
func AssertSecretHasDataEntries(t *testing.T, fakeClient runtimeclient.Client, namespacedName types.NamespacedName, expectedEntries ...string) map[string][]byte {
	actual := &corev1.Secret{}
	err := fakeClient.Get(context.TODO(), namespacedName, actual)
	require.NoError(t, err)
	require.Len(t, actual.Data, len(expectedEntries))
	for _, e := range expectedEntries {
		assert.Contains(t, actual.Data, e)
	}
	return actual.Data
}

// AssertSecretDoesNotExist verifies that there is no Secret resource with the given namespace/name
func AssertSecretDoesNotExist(t *testing.T, fakeClient runtimeclient.Client, namespacedName types.NamespacedName) {
	AssertObjectDoesNotExist(t, fakeClient, namespacedName, &corev1.Secret{})
}

func AssertServiceAccountHasImagePullSecret(t *testing.T, fakeClient runtimeclient.Client, saNamespacedName types.NamespacedName, secretName string) {
	actual := &corev1.ServiceAccount{}
	err := fakeClient.Get(context.TODO(), saNamespacedName, actual)
	require.NoError(t, err)
	assert.Contains(t, actual.ImagePullSecrets, corev1.LocalObjectReference{Name: secretName})
}

// AssertDeploymentHasReplicas verifies that the there is a Deployment resource matching the expected namespace/name, and with the same spec.replicas.
func AssertDeploymentHasReplicas(t *testing.T, fakeClient runtimeclient.Client, namespacedName types.NamespacedName, replicas int32) {
	actual := &appsv1.Deployment{}
	AssertObjectHasContent(t, fakeClient, namespacedName, actual, func() {
		require.NotNil(t, actual.Spec.Replicas)
		assert.Equal(t, replicas, *actual.Spec.Replicas)
	})
}

// ObjectAssertion is a type of function that should assert a content of an object
type ObjectAssertion func(t *testing.T, fakeClient *test.FakeClient)

// ObjectExists checks that the given object exists and executes the given content assertion
func ObjectExists(namespace, name string, actualResource runtimeclient.Object, contentAssertion func(t *testing.T)) ObjectAssertion {
	return func(t *testing.T, fakeClient *test.FakeClient) {
		AssertObjectHasContent(t, fakeClient, test.NamespacedName(namespace, name), actualResource, func() {
			contentAssertion(t)
		})
	}
}

// ObjectDoesNotExists checks that the given object does not exist
func ObjectDoesNotExists(namespace, name string, actualResource runtimeclient.Object) ObjectAssertion {
	return func(t *testing.T, fakeClient *test.FakeClient) {
		AssertObjectDoesNotExist(t, fakeClient, test.NamespacedName(namespace, name), actualResource)
	}
}

// AssertObjects executes all given object assertions
func AssertObjects(t *testing.T, fakeClient *test.FakeClient, objectAssertions ...ObjectAssertion) {
	for _, assertObject := range objectAssertions {
		assertObject(t, fakeClient)
	}
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

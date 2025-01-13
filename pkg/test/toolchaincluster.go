package test

import (
	"context"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	v1 "k8s.io/api/core/v1"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewToolchainCluster(modifiers ...ToolchainClusterModifier) *toolchainv1alpha1.ToolchainCluster {
	toolchainCluster := &toolchainv1alpha1.ToolchainCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "member1",
			Namespace: test.HostOperatorNs,
		},
		Spec: toolchainv1alpha1.ToolchainClusterSpec{
			SecretRef: toolchainv1alpha1.LocalSecretReference{
				Name: "member1-secret",
			},
		},
		Status: toolchainv1alpha1.ToolchainClusterStatus{
			APIEndpoint: "https://api.member.com:6443",
		},
	}
	for _, modify := range modifiers {
		modify(toolchainCluster)
	}
	return toolchainCluster
}

func AssertToolchainClusterDoesNotExist(t *testing.T, fakeClient *test.FakeClient, toolchainCluster *toolchainv1alpha1.ToolchainCluster) {
	deletedCluster := &toolchainv1alpha1.ToolchainCluster{}
	err := fakeClient.Get(context.TODO(), test.NamespacedName(toolchainCluster.Namespace, toolchainCluster.Name), deletedCluster)
	require.True(t, apierrors.IsNotFound(err), "the ToolchainCluster should be deleted")

	err = fakeClient.Get(context.TODO(), test.NamespacedName(toolchainCluster.Namespace, toolchainCluster.Spec.SecretRef.Name), &v1.Secret{})
	require.True(t, apierrors.IsNotFound(err), "the ToolchainCluster secret should be deleted")
}

func AssertToolchainClusterSpec(t *testing.T, fakeClient *test.FakeClient, expectedToolchainCluster *toolchainv1alpha1.ToolchainCluster) {
	foundCluster := &toolchainv1alpha1.ToolchainCluster{}
	err := fakeClient.Get(context.TODO(), test.NamespacedName(expectedToolchainCluster.Namespace, expectedToolchainCluster.Name), foundCluster)
	require.NoError(t, err)
	assert.Equal(t, expectedToolchainCluster.Spec, foundCluster.Spec)
}

type ToolchainClusterModifier func(toolchainCluster *toolchainv1alpha1.ToolchainCluster)

func ToolchainClusterName(name string) ToolchainClusterModifier {
	return func(toolchainCluster *toolchainv1alpha1.ToolchainCluster) {
		toolchainCluster.Name = name
		toolchainCluster.Spec.SecretRef.Name = name + "-secret"
	}
}

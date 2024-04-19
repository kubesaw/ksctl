package test

import (
	"context"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/states"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"

	uuid "github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewUserSignup(modifiers ...UserSignupModifier) *toolchainv1alpha1.UserSignup {
	signup := &toolchainv1alpha1.UserSignup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      uuid.NewString(),
			Namespace: test.HostOperatorNs,
			Labels: map[string]string{
				toolchainv1alpha1.UserSignupUserEmailHashLabelKey: "fd2addbd8d82f0d2dc088fa122377eaa",
				toolchainv1alpha1.UserSignupUserPhoneHashLabelKey: "354365c1e4a37b74ed5b12fdeeno",
			},
		},
		Spec: toolchainv1alpha1.UserSignupSpec{
			IdentityClaims: toolchainv1alpha1.IdentityClaimsEmbedded{
				PreferredUsername: "foo@redhat.com",
				PropagatedClaims: toolchainv1alpha1.PropagatedClaims{
					Email: "foo@redhat.com",
				},
			},
		},
	}
	states.SetVerificationRequired(signup, true)
	for _, modify := range modifiers {
		modify(signup)
	}
	return signup
}

func AssertUserSignupSpec(t *testing.T, fakeClient *test.FakeClient, expectedUserSignup *toolchainv1alpha1.UserSignup) {
	updatedSignup := &toolchainv1alpha1.UserSignup{}
	err := fakeClient.Get(context.TODO(), test.NamespacedName(expectedUserSignup.Namespace, expectedUserSignup.Name), updatedSignup)
	require.NoError(t, err)
	if len(expectedUserSignup.Spec.States) == 0 {
		expectedUserSignup.Spec.States = nil
	}
	assert.Equal(t, expectedUserSignup.Spec, updatedSignup.Spec)
}

func AssertUserSignupDoesNotExist(t *testing.T, fakeClient *test.FakeClient, userSignup *toolchainv1alpha1.UserSignup) {
	deletedUserSignup := &toolchainv1alpha1.UserSignup{}
	err := fakeClient.Get(context.TODO(), test.NamespacedName(userSignup.Namespace, userSignup.Name), deletedUserSignup)
	require.True(t, apierrors.IsNotFound(err), "the UserSignup should be deleted")
}

func UserSignupCompleteCondition(status corev1.ConditionStatus, reason string) toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:   toolchainv1alpha1.UserSignupComplete,
		Status: status,
		Reason: reason,
	}
}

type UserSignupModifier func(userSignup *toolchainv1alpha1.UserSignup)

func UserSignupCompliantUsername(username string) UserSignupModifier {
	return func(userSignup *toolchainv1alpha1.UserSignup) {
		userSignup.Status.CompliantUsername = username
	}
}

func UserSignupTargetCluster(cluster string) UserSignupModifier {
	return func(userSignup *toolchainv1alpha1.UserSignup) {
		userSignup.Spec.TargetCluster = cluster
	}
}

func UserSignupDeactivated(deactivated bool) UserSignupModifier {
	return func(userSignup *toolchainv1alpha1.UserSignup) {
		states.SetDeactivated(userSignup, deactivated)
	}
}

func UserSignupStatusComplete(status corev1.ConditionStatus, reason string) UserSignupModifier {
	return func(userSignup *toolchainv1alpha1.UserSignup) {
		userSignup.Status.Conditions = []toolchainv1alpha1.Condition{UserSignupCompleteCondition(status, reason)}
	}
}

func UserSignupSetLabel(key, value string) UserSignupModifier {
	return func(userSignup *toolchainv1alpha1.UserSignup) {
		userSignup.Labels[key] = value
	}
}

func UserSignupRemoveLabel(key string) UserSignupModifier {
	return func(userSignup *toolchainv1alpha1.UserSignup) {
		delete(userSignup.Labels, key)
	}
}

func UserSignupAutomaticallyApproved(_ bool) UserSignupModifier {
	return func(userSignup *toolchainv1alpha1.UserSignup) {
		userSignup.Labels[toolchainv1alpha1.StateLabelKey] = string(toolchainv1alpha1.UserSignupStateLabelValueApproved)
		userSignup.Spec.States = nil
	}
}

func UserSignupApprovedByAdmin(_ bool) UserSignupModifier {
	return func(userSignup *toolchainv1alpha1.UserSignup) {
		userSignup.Labels[toolchainv1alpha1.StateLabelKey] = string(toolchainv1alpha1.UserSignupStateLabelValueApproved)
		states.SetApprovedManually(userSignup, true)
	}
}

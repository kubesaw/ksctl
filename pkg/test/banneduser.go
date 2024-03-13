package test

import (
	"context"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func AssertBannedUser(t *testing.T, fakeClient *test.FakeClient, userSignup *toolchainv1alpha1.UserSignup) {
	bannedUsers := &toolchainv1alpha1.BannedUserList{}
	err := fakeClient.List(context.TODO(), bannedUsers, runtimeclient.InNamespace(userSignup.Namespace))
	require.NoError(t, err)
	require.Len(t, bannedUsers.Items, 1)
	assert.Equal(t, userSignup.Spec.IdentityClaims.Email, bannedUsers.Items[0].Spec.Email)
	assert.Equal(t, userSignup.Labels[toolchainv1alpha1.UserSignupUserEmailHashLabelKey], bannedUsers.Items[0].Labels[toolchainv1alpha1.BannedUserEmailHashLabelKey])
	assert.Equal(t, userSignup.Labels[toolchainv1alpha1.UserSignupUserPhoneHashLabelKey], bannedUsers.Items[0].Labels[toolchainv1alpha1.BannedUserPhoneNumberHashLabelKey])
}

func AssertNoBannedUser(t *testing.T, fakeClient *test.FakeClient, userSignup *toolchainv1alpha1.UserSignup) {
	bannedUsers := &toolchainv1alpha1.BannedUserList{}
	err := fakeClient.List(context.TODO(), bannedUsers, runtimeclient.InNamespace(userSignup.Namespace))
	require.NoError(t, err)
	require.Empty(t, bannedUsers.Items)
}

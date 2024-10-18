package cmd_test

import (
	"bytes"
	"context"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/codeready-toolchain/toolchain-common/pkg/test/masteruserrecord"
	"github.com/kubesaw/ksctl/pkg/cmd"
	clicontext "github.com/kubesaw/ksctl/pkg/context"
	"github.com/kubesaw/ksctl/pkg/ioutils"
	. "github.com/kubesaw/ksctl/pkg/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPromoteUserCmdWhenAnswerIsY(t *testing.T) {
	// given
	mur := masteruserrecord.NewMasterUserRecord(t, "testmur", masteruserrecord.TierName("deactivate30"))
	newClient, fakeClient := NewFakeClients(t, mur, newUserTier("deactivate180"))
	SetFileConfig(t, Host())
	buffy := bytes.NewBuffer(nil)
	term := ioutils.NewTerminal(buffy, buffy, ioutils.WithDefaultConfirm(true))
	ctx := clicontext.NewCommandContext(term, newClient)

	// when
	err := cmd.PromoteUser(ctx, mur.Name, "deactivate180")

	// then
	require.NoError(t, err)
	mur.Spec.TierName = "deactivate180" // mur should be changed to deactivate180 tier
	assertMasterUserRecordSpec(t, fakeClient, mur)
	// assert.Contains(t, buffy.String(), "promote the MasterUserRecord 'testmur' to the 'deactivate180' user tier?")
	assert.Contains(t, buffy.String(), "Successfully promoted MasterUserRecord")
	assert.NotContains(t, buffy.String(), "cool-token")
}

func TestPromoteUserCmdWhenAnswerIsN(t *testing.T) {
	// given
	mur := masteruserrecord.NewMasterUserRecord(t, "testmur", masteruserrecord.TierName("deactivate30"))
	newClient, fakeClient := NewFakeClients(t, mur, newUserTier("deactivate180"))
	SetFileConfig(t, Host())
	buffy := bytes.NewBuffer(nil)
	term := ioutils.NewTerminal(buffy, buffy, ioutils.WithDefaultConfirm(false))
	ctx := clicontext.NewCommandContext(term, newClient)

	// when
	err := cmd.PromoteUser(ctx, mur.Name, "deactivate180")

	// then
	require.NoError(t, err)
	assertMasterUserRecordSpec(t, fakeClient, mur) // mur should be unchanged
	// assert.Contains(t, buffy.String(), "promote the MasterUserRecord 'testmur' to the 'deactivate180' user tier?")
	assert.NotContains(t, buffy.String(), "Successfully promoted MasterUserRecord")
	assert.NotContains(t, buffy.String(), "cool-token")
}

func TestPromoteUserCmdWhenMasterUserRecordNotFound(t *testing.T) {
	// given
	mur := masteruserrecord.NewMasterUserRecord(t, "testmur", masteruserrecord.TierName("deactivate30"))
	newClient, fakeClient := NewFakeClients(t, mur, newUserTier("deactivate180"))
	SetFileConfig(t, Host())
	buffy := bytes.NewBuffer(nil)
	term := ioutils.NewTerminal(buffy, buffy)
	ctx := clicontext.NewCommandContext(term, newClient)

	// when
	err := cmd.PromoteUser(ctx, "another", "deactivate180") // attempt to promote a mur that does not exist

	// then
	require.EqualError(t, err, "masteruserrecords.toolchain.dev.openshift.com \"another\" not found")
	assertMasterUserRecordSpec(t, fakeClient, mur) // unrelated mur should be unchanged
	assert.NotContains(t, buffy.String(), "promote the MasterUserRecord 'another' to the 'deactivate180' user tier?")
	assert.NotContains(t, buffy.String(), "Successfully promoted MasterUserRecord")
	assert.NotContains(t, buffy.String(), "cool-token")
}

func TestPromoteUserCmdWhenUserTierNotFound(t *testing.T) {
	// given
	mur := masteruserrecord.NewMasterUserRecord(t, "testmur", masteruserrecord.TierName("deactivate30"))
	newClient, fakeClient := NewFakeClients(t, mur)
	SetFileConfig(t, Host())
	buffy := bytes.NewBuffer(nil)
	term := ioutils.NewTerminal(buffy, buffy)
	ctx := clicontext.NewCommandContext(term, newClient)

	// when
	err := cmd.PromoteUser(ctx, mur.Name, "deactivate180")

	// then
	require.EqualError(t, err, "usertiers.toolchain.dev.openshift.com \"deactivate180\" not found")
	assertMasterUserRecordSpec(t, fakeClient, mur) // mur should be unchanged
	assert.NotContains(t, buffy.String(), "promote the MasterUserRecord 'another' to the 'deactivate180' user tier?")
	assert.NotContains(t, buffy.String(), "Successfully promoted MasterUserRecord")
	assert.NotContains(t, buffy.String(), "cool-token")
}

func assertMasterUserRecordSpec(t *testing.T, fakeClient *test.FakeClient, expectedMasterUserRecord *toolchainv1alpha1.MasterUserRecord) {
	updatedMasterUserRecord := &toolchainv1alpha1.MasterUserRecord{}
	err := fakeClient.Get(context.TODO(), test.NamespacedName(expectedMasterUserRecord.Namespace, expectedMasterUserRecord.Name), updatedMasterUserRecord)
	require.NoError(t, err)
	assert.Equal(t, expectedMasterUserRecord.Spec, updatedMasterUserRecord.Spec)
}

func newUserTier(name string) *toolchainv1alpha1.UserTier {
	userTier := &toolchainv1alpha1.UserTier{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: test.HostOperatorNs,
		},
		Spec: toolchainv1alpha1.UserTierSpec{
			DeactivationTimeoutDays: 180,
		},
	}
	return userTier
}

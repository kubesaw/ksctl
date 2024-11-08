package cmd_test

import (
	"bytes"
	"testing"

	"github.com/codeready-toolchain/toolchain-common/pkg/test/masteruserrecord"

	"github.com/kubesaw/ksctl/pkg/cmd"
	clicontext "github.com/kubesaw/ksctl/pkg/context"
	"github.com/kubesaw/ksctl/pkg/ioutils"
	. "github.com/kubesaw/ksctl/pkg/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDisableUserCmdWhenAnswerIsY(t *testing.T) {
	// given
	// this mur will be disabled
	mur1 := masteruserrecord.NewMasterUserRecord(t, "alice", masteruserrecord.TierName("deactivate30"))
	newClient, fakeClient := NewFakeClients(t, mur1)
	SetFileConfig(t, Host())
	buffy := bytes.NewBuffer(nil)
	term := ioutils.NewTerminal(buffy, buffy, ioutils.WithDefaultConfirm(true))
	ctx := clicontext.NewCommandContext(term, newClient)

	// when
	err := cmd.DisableUser(ctx, mur1.Name)

	// then
	require.NoError(t, err)
	// check if mur was disabled
	mur1.Spec.Disabled = true
	assertMasterUserRecordSpec(t, fakeClient, mur1)
	assert.Contains(t, buffy.String(), "!!!  DANGER ZONE  !!!")
	// assert.Contains(t, buffy.String(), "Are you sure that you want to disable the MasterUserRecord above?")
	assert.Contains(t, buffy.String(), "MasterUserRecord has been disabled")
	assert.NotContains(t, buffy.String(), "cool-token")
}

func TestDisableUserCmdWhenAnswerIsN(t *testing.T) {
	// given
	mur := masteruserrecord.NewMasterUserRecord(t, "alice", masteruserrecord.TierName("deactivate30"))
	newClient, fakeClient := NewFakeClients(t, mur)
	SetFileConfig(t, Host())
	buffy := bytes.NewBuffer(nil)
	term := ioutils.NewTerminal(buffy, buffy, ioutils.WithDefaultConfirm(false))
	ctx := clicontext.NewCommandContext(term, newClient)

	// when
	err := cmd.DisableUser(ctx, mur.Name)

	// then
	require.NoError(t, err)
	assertMasterUserRecordSpec(t, fakeClient, mur)
	assert.Contains(t, buffy.String(), "!!!  DANGER ZONE  !!!")
	// assert.Contains(t, buffy.String(), "Are you sure that you want to disable the MasterUserRecord above?")
	assert.NotContains(t, buffy.String(), "MasterUserRecord has been disabled")
	assert.NotContains(t, buffy.String(), "cool-token")
}

func TestDisableUserCmdWhenNotFound(t *testing.T) {
	// given
	mur := masteruserrecord.NewMasterUserRecord(t, "alice", masteruserrecord.TierName("deactivate30"))
	newClient, fakeClient := NewFakeClients(t, mur)
	SetFileConfig(t, Host())
	buffy := bytes.NewBuffer(nil)
	term := ioutils.NewTerminal(buffy, buffy, ioutils.WithDefaultConfirm(false))
	ctx := clicontext.NewCommandContext(term, newClient)

	// when
	err := cmd.DisableUser(ctx, "some")

	// then
	require.EqualError(t, err, "masteruserrecords.toolchain.dev.openshift.com \"some\" not found")
	assertMasterUserRecordSpec(t, fakeClient, mur)
	assert.NotContains(t, buffy.String(), "!!!  DANGER ZONE  !!!")
	// assert.NotContains(t, buffy.String(), "Are you sure that you want to disable the MasterUserRecord above?")
	assert.NotContains(t, buffy.String(), "MasterUserRecord has been disabled")
	assert.NotContains(t, buffy.String(), "cool-token")
}

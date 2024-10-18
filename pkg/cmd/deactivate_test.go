package cmd_test

import (
	"bytes"
	"testing"

	"github.com/codeready-toolchain/toolchain-common/pkg/states"

	"github.com/kubesaw/ksctl/pkg/cmd"
	clicontext "github.com/kubesaw/ksctl/pkg/context"
	"github.com/kubesaw/ksctl/pkg/ioutils"
	. "github.com/kubesaw/ksctl/pkg/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeactivateCmdWhenAnswerIsY(t *testing.T) {
	// given
	userSignup := NewUserSignup()
	newClient, fakeClient := NewFakeClients(t, userSignup)
	SetFileConfig(t, Host())
	buffy := bytes.NewBuffer(nil)
	term := ioutils.NewTerminal(buffy, buffy, ioutils.WithDefaultConfirm(true))
	ctx := clicontext.NewCommandContext(term, newClient)

	// when
	err := cmd.Deactivate(ctx, userSignup.Name)

	// then
	require.NoError(t, err)
	states.SetDeactivated(userSignup, true)
	AssertUserSignupSpec(t, fakeClient, userSignup)
	assert.Contains(t, buffy.String(), "!!!  DANGER ZONE  !!!")
	// assert.Contains(t, buffy.String(), "Are you sure that you want to deactivate the UserSignup above?")
	assert.Contains(t, buffy.String(), "UserSignup has been deactivated")
	assert.NotContains(t, buffy.String(), "cool-token")
}

func TestDeactivateCmdWhenAnswerIsN(t *testing.T) {
	// given
	userSignup := NewUserSignup()
	newClient, fakeClient := NewFakeClients(t, userSignup)
	SetFileConfig(t, Host())
	buffy := bytes.NewBuffer(nil)
	term := ioutils.NewTerminal(buffy, buffy, ioutils.WithDefaultConfirm(false))
	ctx := clicontext.NewCommandContext(term, newClient)

	// when
	err := cmd.Deactivate(ctx, userSignup.Name)

	// then
	require.NoError(t, err)
	AssertUserSignupSpec(t, fakeClient, userSignup)
	assert.Contains(t, buffy.String(), "!!!  DANGER ZONE  !!!")
	// assert.Contains(t, buffy.String(), "Are you sure that you want to deactivate the UserSignup above?")
	assert.NotContains(t, buffy.String(), "UserSignup has been deactivated")
	assert.NotContains(t, buffy.String(), "cool-token")
}

func TestDeactivateCmdWhenNotFound(t *testing.T) {
	// given
	userSignup := NewUserSignup()
	newClient, fakeClient := NewFakeClients(t, userSignup)
	SetFileConfig(t, Host())
	buffy := bytes.NewBuffer(nil)
	term := ioutils.NewTerminal(buffy, buffy)
	ctx := clicontext.NewCommandContext(term, newClient)

	// when
	err := cmd.Deactivate(ctx, "some")

	// then
	require.EqualError(t, err, "usersignups.toolchain.dev.openshift.com \"some\" not found")
	AssertUserSignupSpec(t, fakeClient, userSignup)
	assert.NotContains(t, buffy.String(), "!!!  DANGER ZONE  !!!")
	// assert.NotContains(t, buffy.String(), "Are you sure that you want to deactivate the UserSignup above?")
	assert.NotContains(t, buffy.String(), "UserSignup has been deactivated")
	assert.NotContains(t, buffy.String(), "cool-token")
}

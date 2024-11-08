package cmd_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/kubesaw/ksctl/pkg/cmd"
	clicontext "github.com/kubesaw/ksctl/pkg/context"
	"github.com/kubesaw/ksctl/pkg/ioutils"
	. "github.com/kubesaw/ksctl/pkg/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func TestDeleteCmdWhenAnswerIsY(t *testing.T) {
	// given
	userSignup := NewUserSignup()
	newClient, fakeClient := NewFakeClients(t, userSignup)
	SetFileConfig(t, Host())
	buffy := bytes.NewBuffer(nil)
	term := ioutils.NewTerminal(buffy, buffy, ioutils.WithDefaultConfirm(true))
	ctx := clicontext.NewCommandContext(term, newClient)

	// when
	err := cmd.Delete(ctx, userSignup.Name)

	// then
	require.NoError(t, err)
	AssertUserSignupDoesNotExist(t, fakeClient, userSignup)
	assert.Contains(t, buffy.String(), "!!!  DANGER ZONE  !!!")
	assert.Contains(t, buffy.String(), "This command should be executed after a GDPR request")
	// assert.Contains(t, buffy.String(), "Are you sure that you want to delete the UserSignup above?")
	assert.Contains(t, buffy.String(), "The deletion of the UserSignup has been triggered")
	assert.NotContains(t, buffy.String(), "cool-token")
}

func TestDeleteCmdWhenAnswerIsN(t *testing.T) {
	// given
	userSignup := NewUserSignup()
	newClient, fakeClient := NewFakeClients(t, userSignup)
	SetFileConfig(t, Host())
	buffy := bytes.NewBuffer(nil)
	term := ioutils.NewTerminal(buffy, buffy, ioutils.WithDefaultConfirm(false))
	ctx := clicontext.NewCommandContext(term, newClient)

	// when
	err := cmd.Delete(ctx, userSignup.Name)

	// then
	require.NoError(t, err)
	AssertUserSignupSpec(t, fakeClient, userSignup)
	assert.Contains(t, buffy.String(), "!!!  DANGER ZONE  !!!")
	assert.Contains(t, buffy.String(), "This command should be executed after a GDPR request")
	// assert.Contains(t, buffy.String(), "Are you sure that you want to delete the UserSignup above?")
	assert.NotContains(t, buffy.String(), "The deletion of the UserSignup has been triggered")
	assert.NotContains(t, buffy.String(), "cool-token")
}

func TestDeleteCmdWhenNotFound(t *testing.T) {
	// given
	userSignup := NewUserSignup()
	newClient, fakeClient := NewFakeClients(t, userSignup)
	SetFileConfig(t, Host())
	buffy := bytes.NewBuffer(nil)
	term := ioutils.NewTerminal(buffy, buffy)
	ctx := clicontext.NewCommandContext(term, newClient)

	// when
	err := cmd.Delete(ctx, "some")

	// then
	require.EqualError(t, err, "usersignups.toolchain.dev.openshift.com \"some\" not found")
	AssertUserSignupSpec(t, fakeClient, userSignup)
	assert.NotContains(t, buffy.String(), "!!!  DANGER ZONE  !!!")
	// assert.NotContains(t, buffy.String(), "Are you sure that you want to delete the UserSignup above?")
	assert.NotContains(t, buffy.String(), "The deletion of the UserSignup has been triggered")
	assert.NotContains(t, buffy.String(), "cool-token")
}

func TestDeleteLacksPermissions(t *testing.T) {
	// given
	SetFileConfig(t, Host(NoToken()))

	userSignup := NewUserSignup()
	newClient, fakeClient := NewFakeClients(t, userSignup)
	buffy := bytes.NewBuffer(nil)
	term := ioutils.NewTerminal(buffy, buffy)
	ctx := clicontext.NewCommandContext(term, newClient)

	// when
	err := cmd.Delete(ctx, userSignup.Name)

	// then
	require.EqualError(t, err, "ksctl command failed: the token in your ksctl.yaml file is missing")
	AssertUserSignupSpec(t, fakeClient, userSignup)
}

func TestDeleteHasPropagationPolicy(t *testing.T) {
	// given
	userSignup := NewUserSignup()
	newClient, fakeClient := NewFakeClients(t, userSignup)
	SetFileConfig(t, Host())

	deleted := false
	fakeClient.MockDelete = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.DeleteOption) error {
		deleted = true
		require.Len(t, opts, 1)
		deleteOptions, ok := opts[0].(*runtimeclient.DeleteOptions)
		require.True(t, ok)
		require.NotNil(t, deleteOptions)
		require.NotNil(t, deleteOptions.PropagationPolicy)
		assert.Equal(t, metav1.DeletePropagationForeground, *deleteOptions.PropagationPolicy)
		return nil
	}
	buffy := bytes.NewBuffer(nil)
	term := ioutils.NewTerminal(buffy, buffy, ioutils.WithDefaultConfirm(true))
	ctx := clicontext.NewCommandContext(term, newClient)

	// when
	err := cmd.Delete(ctx, userSignup.Name)

	// then
	require.NoError(t, err)
	require.True(t, deleted)
}

package cmd_test

import (
	"context"
	"testing"

	"github.com/kubesaw/ksctl/pkg/cmd"
	clicontext "github.com/kubesaw/ksctl/pkg/context"
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
	term := NewFakeTerminalWithResponse("y")
	ctx := clicontext.NewCommandContext(term, newClient)

	// when
	err := cmd.Delete(ctx, userSignup.Name)

	// then
	require.NoError(t, err)
	AssertUserSignupDoesNotExist(t, fakeClient, userSignup)
	assert.Contains(t, term.Output(), "!!!  DANGER ZONE  !!!")
	assert.Contains(t, term.Output(), "THIS COMMAND SHOULD BE EXECUTED BASED ON GDPR REQUEST.")
	assert.Contains(t, term.Output(), "Are you sure that you want to delete the UserSignup above?")
	assert.Contains(t, term.Output(), "The deletion of the UserSignup has been triggered")
	assert.NotContains(t, term.Output(), "cool-token")
}

func TestDeleteCmdWhenAnswerIsN(t *testing.T) {
	// given
	userSignup := NewUserSignup()
	newClient, fakeClient := NewFakeClients(t, userSignup)
	SetFileConfig(t, Host())
	term := NewFakeTerminalWithResponse("n")
	ctx := clicontext.NewCommandContext(term, newClient)

	// when
	err := cmd.Delete(ctx, userSignup.Name)

	// then
	require.NoError(t, err)
	AssertUserSignupSpec(t, fakeClient, userSignup)
	assert.Contains(t, term.Output(), "!!!  DANGER ZONE  !!!")
	assert.Contains(t, term.Output(), "THIS COMMAND SHOULD BE EXECUTED BASED ON GDPR REQUEST.")
	assert.Contains(t, term.Output(), "Are you sure that you want to delete the UserSignup above?")
	assert.NotContains(t, term.Output(), "The deletion of the UserSignup has been triggered")
	assert.NotContains(t, term.Output(), "cool-token")
}

func TestDeleteCmdWhenNotFound(t *testing.T) {
	// given
	userSignup := NewUserSignup()
	newClient, fakeClient := NewFakeClients(t, userSignup)
	SetFileConfig(t, Host())
	term := NewFakeTerminalWithResponse("n")
	ctx := clicontext.NewCommandContext(term, newClient)

	// when
	err := cmd.Delete(ctx, "some")

	// then
	require.EqualError(t, err, "usersignups.toolchain.dev.openshift.com \"some\" not found")
	AssertUserSignupSpec(t, fakeClient, userSignup)
	assert.NotContains(t, term.Output(), "!!!  DANGER ZONE  !!!")
	assert.NotContains(t, term.Output(), "Are you sure that you want to delete the UserSignup above?")
	assert.NotContains(t, term.Output(), "The deletion of the UserSignup has been triggered")
	assert.NotContains(t, term.Output(), "cool-token")
}

func TestDeleteLacksPermissions(t *testing.T) {
	// given
	SetFileConfig(t, Host(NoToken()))

	userSignup := NewUserSignup()
	newClient, fakeClient := NewFakeClients(t, userSignup)
	term := NewFakeTerminal()
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
	term := NewFakeTerminalWithResponse("y")

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
	ctx := clicontext.NewCommandContext(term, newClient)

	// when
	err := cmd.Delete(ctx, userSignup.Name)

	// then
	require.NoError(t, err)
	require.True(t, deleted)
}

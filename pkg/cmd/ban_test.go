package cmd_test

import (
	"context"
	"fmt"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/kubesaw/ksctl/pkg/cmd"
	clicontext "github.com/kubesaw/ksctl/pkg/context"
	. "github.com/kubesaw/ksctl/pkg/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func TestBanCmdWhenAnswerIsY(t *testing.T) {
	// given
	userSignup := NewUserSignup()
	newClient, fakeClient := NewFakeClients(t, userSignup)
	SetFileConfig(t, Host())
	term := NewFakeTerminalWithResponse("y")
	ctx := clicontext.NewCommandContext(term, newClient)

	// when
	err := cmd.Ban(ctx, userSignup.Name)

	// then
	require.NoError(t, err)
	AssertBannedUser(t, fakeClient, userSignup)
	assert.Contains(t, term.Output(), "!!!  DANGER ZONE  !!!")
	assert.Contains(t, term.Output(), "Are you sure that you want to ban the user with the UserSignup by creating BannedUser resource that are both above?")
	assert.Contains(t, term.Output(), "UserSignup has been banned")
	assert.NotContains(t, term.Output(), "cool-token")

	t.Run("don't ban twice", func(t *testing.T) {
		// given
		term := NewFakeTerminalWithResponse("y")
		ctx := clicontext.NewCommandContext(term, newClient)

		// when
		err := cmd.Ban(ctx, userSignup.Name)

		// then
		require.NoError(t, err)
		AssertBannedUser(t, fakeClient, userSignup)
		assert.NotContains(t, term.Output(), "!!!  DANGER ZONE  !!!")
		assert.Contains(t, term.Output(), "The user was already banned - there is a BannedUser resource with the same labels already present")
	})
}

func TestBanCmdWhenAnswerIsN(t *testing.T) {
	// given
	userSignup := NewUserSignup()
	newClient, fakeClient := NewFakeClients(t, userSignup)
	SetFileConfig(t, Host())
	term := NewFakeTerminalWithResponse("n")
	ctx := clicontext.NewCommandContext(term, newClient)

	// when
	err := cmd.Ban(ctx, userSignup.Name)

	// then
	require.NoError(t, err)
	AssertNoBannedUser(t, fakeClient, userSignup)
	assert.Contains(t, term.Output(), "!!!  DANGER ZONE  !!!")
	assert.Contains(t, term.Output(), "Are you sure that you want to ban the user with the UserSignup by creating BannedUser resource that are both above?")
	assert.NotContains(t, term.Output(), "UserSignup has been banned")
	assert.NotContains(t, term.Output(), "cool-token")
}

func TestBanCmdWhenNotFound(t *testing.T) {
	// given
	userSignup := NewUserSignup()
	newClient, fakeClient := NewFakeClients(t, userSignup)
	SetFileConfig(t, Host())
	term := NewFakeTerminalWithResponse("n")
	ctx := clicontext.NewCommandContext(term, newClient)

	// when
	err := cmd.Ban(ctx, "some")

	// then
	require.EqualError(t, err, "usersignups.toolchain.dev.openshift.com \"some\" not found")
	AssertNoBannedUser(t, fakeClient, userSignup)
	assert.NotContains(t, term.Output(), "!!!  DANGER ZONE  !!!")
	assert.NotContains(t, term.Output(), "Are you sure that you want to ban the user with the UserSignup by creating BannedUser resource that are both above?")
	assert.NotContains(t, term.Output(), "UserSignup has been banned")
	assert.NotContains(t, term.Output(), "cool-token")
}

func TestCreateBannedUser(t *testing.T) {
	// given
	SetFileConfig(t, Host())

	t.Run("BannedUser creation is successful", func(t *testing.T) {
		//given
		userSignup := NewUserSignup()
		newClient, fakeClient := NewFakeClients(t, userSignup)
		term := NewFakeTerminal()
		ctx := clicontext.NewCommandContext(term, newClient)

		// when
		err := cmd.CreateBannedUser(ctx, userSignup.Name, func(signup *toolchainv1alpha1.UserSignup, bannedUser *toolchainv1alpha1.BannedUser) (bool, error) {
			return true, nil
		})

		// then
		require.NoError(t, err)
		AssertBannedUser(t, fakeClient, userSignup)
	})

	t.Run("BannedUser should not be created", func(t *testing.T) {
		//given
		userSignup := NewUserSignup()
		newClient, fakeClient := NewFakeClients(t, userSignup)
		term := NewFakeTerminal()
		ctx := clicontext.NewCommandContext(term, newClient)

		// when
		err := cmd.CreateBannedUser(ctx, userSignup.Name, func(signup *toolchainv1alpha1.UserSignup, bannedUser *toolchainv1alpha1.BannedUser) (bool, error) {
			return false, nil
		})

		// then
		require.NoError(t, err)
		AssertNoBannedUser(t, fakeClient, userSignup)
	})

	t.Run("confirmation func returns error", func(t *testing.T) {
		//given
		userSignup := NewUserSignup()
		newClient, fakeClient := NewFakeClients(t, userSignup)
		term := NewFakeTerminal()
		ctx := clicontext.NewCommandContext(term, newClient)

		// when
		err := cmd.CreateBannedUser(ctx, userSignup.Name, func(signup *toolchainv1alpha1.UserSignup, bannedUser *toolchainv1alpha1.BannedUser) (bool, error) {
			return false, fmt.Errorf("some error")
		})

		// then
		require.EqualError(t, err, "some error")
		AssertNoBannedUser(t, fakeClient, userSignup)
	})

	t.Run("get of UserSignup fails", func(t *testing.T) {
		//given
		userSignup := NewUserSignup()
		newClient, fakeClient := NewFakeClients(t, userSignup)
		fakeClient.MockGet = func(ctx context.Context, key runtimeclient.ObjectKey, obj runtimeclient.Object, opts ...runtimeclient.GetOption) error {
			return fmt.Errorf("some error")
		}
		term := NewFakeTerminal()
		ctx := clicontext.NewCommandContext(term, newClient)

		// when
		err := cmd.CreateBannedUser(ctx, userSignup.Name, func(signup *toolchainv1alpha1.UserSignup, bannedUser *toolchainv1alpha1.BannedUser) (bool, error) {
			return true, nil
		})

		// then
		require.EqualError(t, err, "some error")
		AssertNoBannedUser(t, fakeClient, userSignup)
	})

	t.Run("creation of BannedUser fails", func(t *testing.T) {
		//given
		userSignup := NewUserSignup()
		newClient, fakeClient := NewFakeClients(t, userSignup)
		fakeClient.MockCreate = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.CreateOption) error {
			return fmt.Errorf("some error")
		}
		term := NewFakeTerminal()
		ctx := clicontext.NewCommandContext(term, newClient)

		// when
		err := cmd.CreateBannedUser(ctx, userSignup.Name, func(signup *toolchainv1alpha1.UserSignup, bannedUser *toolchainv1alpha1.BannedUser) (bool, error) {
			return true, nil
		})

		// then
		require.EqualError(t, err, "some error")
		AssertNoBannedUser(t, fakeClient, userSignup)
	})

	t.Run("client creation fails", func(t *testing.T) {
		//given
		userSignup := NewUserSignup()
		fakeClient := test.NewFakeClient(t, userSignup)
		term := NewFakeTerminal()
		newClient := func(token, apiEndpoint string) (runtimeclient.Client, error) {
			return nil, fmt.Errorf("some error")
		}
		ctx := clicontext.NewCommandContext(term, newClient)

		// when
		err := cmd.CreateBannedUser(ctx, userSignup.Name, func(signup *toolchainv1alpha1.UserSignup, bannedUser *toolchainv1alpha1.BannedUser) (bool, error) {
			return true, nil
		})

		// then
		require.EqualError(t, err, "some error")
		AssertNoBannedUser(t, fakeClient, userSignup)
	})
}

func TestCreateBannedUserLacksPermissions(t *testing.T) {
	// given
	SetFileConfig(t, Host(NoToken()))

	userSignup := NewUserSignup()
	newClient, fakeClient := NewFakeClients(t, userSignup)
	term := NewFakeTerminal()
	ctx := clicontext.NewCommandContext(term, newClient)

	// when
	err := cmd.CreateBannedUser(ctx, userSignup.Name, func(signup *toolchainv1alpha1.UserSignup, bannedUser *toolchainv1alpha1.BannedUser) (bool, error) {
		return true, nil
	})

	// then
	require.EqualError(t, err, "ksctl command failed: the token in your ksctl.yaml file is missing")
	AssertUserSignupSpec(t, fakeClient, userSignup)
}

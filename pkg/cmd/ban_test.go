package cmd_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/charmbracelet/huh"
	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/kubesaw/ksctl/pkg/cmd"
	clicontext "github.com/kubesaw/ksctl/pkg/context"
	. "github.com/kubesaw/ksctl/pkg/test"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const banReason = "ban reason"

var banReasonInput = input('b', 'a', 'n', ' ', 'r', 'e', 'a', 's', 'o', 'n')

func TestBanCmdWhenAnswerIsY(t *testing.T) {
	// given
	userSignup := NewUserSignup()
	newClient, fakeClient := NewFakeClients(t, userSignup)
	SetFileConfig(t, Host())
	term := NewFakeTerminalWithResponse("y")
	ctx := clicontext.NewCommandContext(term, newClient)

	// when
	err := cmd.Ban(ctx, func(form *huh.Form) error {
		form.Init()
		form.Update(banReasonInput)
		return nil
	}, userSignup.Name)

	// then
	require.NoError(t, err)
	AssertBannedUser(t, fakeClient, userSignup, false, banReason)
	assert.Contains(t, term.Output(), "!!!  DANGER ZONE  !!!")
	assert.Contains(t, term.Output(), "Are you sure that you want to ban the user with the UserSignup by creating BannedUser resource that are both above?")
	assert.Contains(t, term.Output(), "UserSignup has been banned")
	assert.NotContains(t, term.Output(), "cool-token")

	t.Run("don't ban twice", func(t *testing.T) {
		// given
		term := NewFakeTerminalWithResponse("y")
		ctx := clicontext.NewCommandContext(term, newClient)

		// when
		err := cmd.Ban(ctx, func(form *huh.Form) error {
			form.Init()
			form.Update(banReasonInput)
			return nil
		}, userSignup.Name)

		// then
		require.NoError(t, err)
		AssertBannedUser(t, fakeClient, userSignup, false, banReason)
		assert.NotContains(t, term.Output(), "!!!  DANGER ZONE  !!!")
		assert.Contains(t, term.Output(), "The user was already banned - there is a BannedUser resource with the same labels already present")
	})
}

func input(runes ...rune) tea.KeyMsg {
	return tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: runes,
	}
}

func TestBanCmdWhenAnswerIsN(t *testing.T) {
	// given
	userSignup := NewUserSignup()
	newClient, fakeClient := NewFakeClients(t, userSignup)
	SetFileConfig(t, Host())
	term := NewFakeTerminalWithResponse("n")
	ctx := clicontext.NewCommandContext(term, newClient)

	// when
	err := cmd.Ban(ctx, func(form *huh.Form) error {
		form.Init()
		form.Update(banReasonInput)
		return nil
	}, userSignup.Name)

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
	err := cmd.Ban(ctx, func(form *huh.Form) error {
		form.Init()
		form.Update(banReasonInput)
		return nil
	}, "some")

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
		err := cmd.CreateBannedUser(ctx, userSignup.Name, banReason, func(signup *toolchainv1alpha1.UserSignup, bannedUser *toolchainv1alpha1.BannedUser) (bool, error) {
			return true, nil
		})

		// then
		require.NoError(t, err)
		AssertBannedUser(t, fakeClient, userSignup, false, banReason)
	})

	t.Run("BannedUser should not be created", func(t *testing.T) {
		//given
		userSignup := NewUserSignup()
		newClient, fakeClient := NewFakeClients(t, userSignup)
		term := NewFakeTerminal()
		ctx := clicontext.NewCommandContext(term, newClient)

		// when
		err := cmd.CreateBannedUser(ctx, userSignup.Name, banReason, func(signup *toolchainv1alpha1.UserSignup, bannedUser *toolchainv1alpha1.BannedUser) (bool, error) {
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
		err := cmd.CreateBannedUser(ctx, userSignup.Name, banReason, func(signup *toolchainv1alpha1.UserSignup, bannedUser *toolchainv1alpha1.BannedUser) (bool, error) {
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
		err := cmd.CreateBannedUser(ctx, userSignup.Name, banReason, func(signup *toolchainv1alpha1.UserSignup, bannedUser *toolchainv1alpha1.BannedUser) (bool, error) {
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
		err := cmd.CreateBannedUser(ctx, userSignup.Name, banReason, func(signup *toolchainv1alpha1.UserSignup, bannedUser *toolchainv1alpha1.BannedUser) (bool, error) {
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
		err := cmd.CreateBannedUser(ctx, userSignup.Name, banReason, func(signup *toolchainv1alpha1.UserSignup, bannedUser *toolchainv1alpha1.BannedUser) (bool, error) {
			return true, nil
		})

		// then
		require.EqualError(t, err, "some error")
		AssertNoBannedUser(t, fakeClient, userSignup)
	})

	t.Run("GetBannedUser call fails", func(t *testing.T) {
		//given
		userSignup := NewUserSignup()
		newClient, fakeClient := NewFakeClients(t, userSignup)
		term := NewFakeTerminal()
		ctx := clicontext.NewCommandContext(term, newClient)

		fakeClient.MockList = func(ctx context.Context, list runtimeclient.ObjectList, opts ...runtimeclient.ListOption) error {
			return errors.New("something went wrong listing the banned users")
		}

		// when
		err := cmd.CreateBannedUser(ctx, userSignup.Name, banReason, func(signup *toolchainv1alpha1.UserSignup, bannedUser *toolchainv1alpha1.BannedUser) (bool, error) {
			return true, nil
		})

		// then
		require.Error(t, err, "something went wrong listing the banned users")
	})
	t.Run("NewBannedUser call fails", func(t *testing.T) {
		//given
		userSignup := NewUserSignup()
		userSignup.Labels = nil
		newClient, fakeClient := NewFakeClients(t, userSignup)
		term := NewFakeTerminal()
		ctx := clicontext.NewCommandContext(term, newClient)

		// when
		err := cmd.CreateBannedUser(ctx, userSignup.Name, banReason, func(signup *toolchainv1alpha1.UserSignup, bannedUser *toolchainv1alpha1.BannedUser) (bool, error) {
			return true, nil
		})

		// then
		require.Error(t, err, "userSignup doesn't have UserSignupUserEmailHashLabelKey")
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
	err := cmd.CreateBannedUser(ctx, userSignup.Name, banReason, func(signup *toolchainv1alpha1.UserSignup, bannedUser *toolchainv1alpha1.BannedUser) (bool, error) {
		return true, nil
	})

	// then
	require.EqualError(t, err, "ksctl command failed: the token in your ksctl.yaml file is missing")
	AssertUserSignupSpec(t, fakeClient, userSignup)
}

func newBanReasonConfigMap(key, value string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ban-reason-config",
			Namespace: test.HostOperatorNs,
		},
		Data: map[string]string{
			key: value,
		},
	}
}

// TestBanConfigMapProcessing tests whether the configmap content is empty
func TestBanConfigMapProcessing(t *testing.T) {
	t.Run("empty ConfigMap content", func(t *testing.T) {
		// given
		userSignup := NewUserSignup()
		emptyConfigMap := newBanReasonConfigMap("menu.json", "[]") // Empty array
		newClient, fakeClient := NewFakeClients(t, userSignup, emptyConfigMap)
		SetFileConfig(t, Host())
		term := NewFakeTerminalWithResponse("y")
		ctx := clicontext.NewCommandContext(term, newClient)

		// when
		err := cmd.Ban(ctx, func(form *huh.Form) error {
			form.Init()
			form.Update(banReasonInput)
			return nil
		}, userSignup.Name)

		// then
		require.NoError(t, err)
		assert.Contains(t, term.Output(), banReason)
		AssertBannedUser(t, fakeClient, userSignup, false, "ban reason")
	})

	t.Run("ConfigMap with no menu.json key falls back to manual input", func(t *testing.T) {
		// given
		userSignup := NewUserSignup()
		configMapWithoutMenu := newBanReasonConfigMap("other-key", "some-value") // No Menu.json key
		newClient, fakeClient := NewFakeClients(t, userSignup, configMapWithoutMenu)
		SetFileConfig(t, Host())
		term := NewFakeTerminalWithResponse("y")
		ctx := clicontext.NewCommandContext(term, newClient)

		// when
		err := cmd.Ban(ctx, func(form *huh.Form) error {
			form.Init()
			form.Update(banReasonInput)
			return nil
		}, userSignup.Name)

		// then
		require.NoError(t, err)
		AssertBannedUser(t, fakeClient, userSignup, false, banReason)
	})

	t.Run("invalid JSON in ConfigMap falls back to manual input", func(t *testing.T) {
		// given
		userSignup := NewUserSignup()
		configMapWithBadJSON := newBanReasonConfigMap("menu.json", "[{invalid json}")
		newClient, fakeClient := NewFakeClients(t, userSignup, configMapWithBadJSON)
		SetFileConfig(t, Host())
		term := NewFakeTerminalWithResponse("y")
		ctx := clicontext.NewCommandContext(term, newClient)

		// when
		err := cmd.Ban(ctx, func(form *huh.Form) error {
			form.Init()
			form.Update(banReasonInput)
			return nil
		}, userSignup.Name)

		// then - should succeed and fall back to manual input
		require.NoError(t, err)
		assert.Contains(t, term.Output(), "Checking for available reasons from ConfigMap...")
		AssertBannedUser(t, fakeClient, userSignup, false, banReason)
	})

}

func TestBanWithValidConfigMap(t *testing.T) {
	t.Run("Ban function interactive mode output messages", func(t *testing.T) {

		// given
		userSignup := NewUserSignup()
		validConfigMap := newBanReasonConfigMap("menu.json", `[{"kind":"workload","description":"Select workload","options":["container","vm"]}, 
				{"kind":"behaviorClassification","description":"Select behavior","options":["crypto mining","ddos"]},
				{"kind":"detectionMechanism","description":"How was this detected","options":["GD","WA"]}]`)
		newClient, fakeClient := NewFakeClients(t, userSignup, validConfigMap)
		SetFileConfig(t, Host())
		term := NewFakeTerminalWithResponse("y")
		ctx := clicontext.NewCommandContext(term, newClient)

		// when
		err := cmd.Ban(ctx, func(form *huh.Form) error {
			form.Init()
			form.Update(tea.KeyMsg{Type: tea.KeyDown})
			form.Update(tea.KeyMsg{Type: tea.KeyEnter})
			return nil
		}, userSignup.Name)

		// then
		require.NoError(t, err)
		assert.Contains(t, term.Output(), "Opening interactive menu...")
		AssertBannedUser(t, fakeClient, userSignup, true, `{"workload":"vm","behaviorClassification":"ddos","detectionMechanism":"WA"}`)
	})
}

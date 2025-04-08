package cmd_test

import (
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/hash"
	"github.com/kubesaw/ksctl/pkg/cmd"
	"github.com/kubesaw/ksctl/pkg/configuration"
	clicontext "github.com/kubesaw/ksctl/pkg/context"
	"github.com/kubesaw/ksctl/pkg/ioutils"
	. "github.com/kubesaw/ksctl/pkg/test"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestUnbanCommand(t *testing.T) {
	t.Run("fails with no parameters", func(t *testing.T) {
		cmd := cmd.NewUnbanCommand()
		_, err := cmd.ExecuteC()
		require.Error(t, err)
		require.Contains(t, err.Error(), "accepts 1 arg(s), received 0")
	})
	t.Run("fails with more than 1 parameter", func(t *testing.T) {
		cmd := cmd.NewUnbanCommand()
		cmd.SetArgs([]string{"a", "b"})
		_, err := cmd.ExecuteC()
		require.Error(t, err)
		require.Contains(t, err.Error(), "accepts 1 arg(s), received 2")
	})
	t.Run("runs with exactly 1 parameter", func(t *testing.T) {
		cmd := cmd.NewUnbanCommand()
		cmd.SetArgs([]string{"me@home"})
		commandRan := false
		cmd.RunE = func(cmd *cobra.Command, args []string) error {
			require.Len(t, args, 1)
			assert.Equal(t, "me@home", args[0])
			commandRan = true
			return nil
		}
		_, err := cmd.ExecuteC()
		require.NoError(t, err)
		assert.True(t, commandRan)
	})
}

func TestUnbanWhenNoneExists(t *testing.T) {
	// given
	SetFileConfig(t, Host())
	term := NewFakeTerminalWithResponse("y")
	newClient, _ := NewFakeClients(t)
	ctx := clicontext.NewCommandContext(term, newClient)

	// when
	err := cmd.Unban(ctx, "me@home")

	// then
	require.NoError(t, err)
	output := term.Output()
	assert.NotContains(t, output, "User successfully unbanned")
	assert.Contains(t, output, "No banned user with given email found.")
}

func TestUnban(t *testing.T) {
	// given
	SetFileConfig(t, Host())
	term := NewFakeTerminalWithResponse("y")
	bannedUser := newBannedUser(t, "me@work", false, term)
	newClient, _ := NewFakeClients(t, bannedUser)
	ctx := clicontext.NewCommandContext(term, newClient)

	// when
	err := cmd.Unban(ctx, "me@work")

	// then
	require.NoError(t, err)
	output := term.Output()
	assert.Contains(t, output, "User successfully unbanned")
}

func newBannedUser(t *testing.T, email string, inconsistent bool, term ioutils.Terminal) *toolchainv1alpha1.BannedUser {
emailToUse := email
if inconsistent {
    emailToUse += ".not"
}

	cfg, err := configuration.LoadClusterConfig(term, configuration.HostName)
	require.NoError(t, err)

	return &toolchainv1alpha1.BannedUser{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "asdf",
			Namespace: cfg.OperatorNamespace,
			Labels: map[string]string{
				toolchainv1alpha1.BannedUserEmailHashLabelKey: hash.EncodeString(email),
			},
		},
		Spec: toolchainv1alpha1.BannedUserSpec{
			Email:  emailToUse,
			Reason: "laughs and giggles",
		},
	}
}

package cmd_test

import (
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/hash"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/kubesaw/ksctl/pkg/cmd"
	clicontext "github.com/kubesaw/ksctl/pkg/context"
	. "github.com/kubesaw/ksctl/pkg/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestCheckBanned(t *testing.T) {
	t.Run("fails with wrong number of parameters", func(t *testing.T) {
		// given
		newClient, _ := NewFakeClients(t)
		SetFileConfig(t, Host())
		term := NewFakeTerminal()
		ctx := clicontext.NewCommandContext(term, newClient)

		test := func(t *testing.T, mur, signup, email string) {
			// when
			err := cmd.CheckBanned(ctx, mur, signup, email)

			// then
			require.Error(t, err)
			assert.Equal(t, "exactly 1 of --mur, --signup or --email must be specified", err.Error())
		}

		t.Run("no parameters", func(t *testing.T) {
			test(t, "", "", "")
		})
		t.Run("mur+signup", func(t *testing.T) {
			test(t, "mur", "signup", "")
		})
		t.Run("mur+email", func(t *testing.T) {
			test(t, "mur", "", "email")
		})
		t.Run("signup+email", func(t *testing.T) {
			test(t, "", "signup", "email")
		})
		t.Run("mur+signup+email", func(t *testing.T) {
			test(t, "mur", "signup", "email")
		})
	})
	t.Run("finds the banned user by the MUR name", func(t *testing.T) {
		t.Run("user is banned", func(t *testing.T) {
			// given
			mur := &toolchainv1alpha1.MasterUserRecord{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "theUser",
					Namespace: test.HostOperatorNs,
				},
				Spec: toolchainv1alpha1.MasterUserRecordSpec{
					PropagatedClaims: toolchainv1alpha1.PropagatedClaims{
						Email: "me@home",
					},
				},
			}
			bannedUser := &toolchainv1alpha1.BannedUser{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "arbitrary-name",
					Namespace: test.HostOperatorNs,
					Labels: map[string]string{
						toolchainv1alpha1.BannedUserEmailHashLabelKey: hash.EncodeString("me@home"),
					},
				},
				Spec: toolchainv1alpha1.BannedUserSpec{
					Email: "me@home",
				},
			}

			newClient, _ := NewFakeClients(t, mur, bannedUser)
			SetFileConfig(t, Host())
			term := NewFakeTerminal()
			ctx := clicontext.NewCommandContext(term, newClient)

			// when
			err := cmd.CheckBanned(ctx, "theUser", "", "")

			// then
			require.NoError(t, err)
			assert.Equal(t, term.Output(), "User is banned.\n")
		})
		t.Run("user is not banned", func(t *testing.T) {
			// given
			mur := &toolchainv1alpha1.MasterUserRecord{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "theUser",
					Namespace: test.HostOperatorNs,
				},
				Spec: toolchainv1alpha1.MasterUserRecordSpec{
					PropagatedClaims: toolchainv1alpha1.PropagatedClaims{
						Email: "me@home",
					},
				},
			}

			newClient, _ := NewFakeClients(t, mur)
			SetFileConfig(t, Host())
			term := NewFakeTerminal()
			ctx := clicontext.NewCommandContext(term, newClient)

			// when
			err := cmd.CheckBanned(ctx, "theUser", "", "")

			// then
			require.NoError(t, err)
			assert.Equal(t, term.Output(), "User is NOT banned.\n")
		})
		t.Run("mur is not found", func(t *testing.T) {
			// given
			newClient, _ := NewFakeClients(t)
			SetFileConfig(t, Host())
			term := NewFakeTerminal()
			ctx := clicontext.NewCommandContext(term, newClient)

			// when
			err := cmd.CheckBanned(ctx, "theUser", "", "")

			// then
			require.NoError(t, err)
			assert.Equal(t, term.Output(), "User not found.\n")
		})
	})
	t.Run("finds the banned user by the UserSignup name", func(t *testing.T) {
		t.Run("user is banned", func(t *testing.T) {
			// given
			signup := &toolchainv1alpha1.UserSignup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "theUser",
					Namespace: test.HostOperatorNs,
				},
				Spec: toolchainv1alpha1.UserSignupSpec{
					IdentityClaims: toolchainv1alpha1.IdentityClaimsEmbedded{
						PropagatedClaims: toolchainv1alpha1.PropagatedClaims{
							Email: "me@home",
						},
					},
				},
			}
			bannedUser := &toolchainv1alpha1.BannedUser{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "arbitrary-name",
					Namespace: test.HostOperatorNs,
					Labels: map[string]string{
						toolchainv1alpha1.BannedUserEmailHashLabelKey: hash.EncodeString("me@home"),
					},
				},
				Spec: toolchainv1alpha1.BannedUserSpec{
					Email: "me@home",
				},
			}

			newClient, _ := NewFakeClients(t, signup, bannedUser)
			SetFileConfig(t, Host())
			term := NewFakeTerminal()
			ctx := clicontext.NewCommandContext(term, newClient)

			// when
			err := cmd.CheckBanned(ctx, "", "theUser", "")

			// then
			require.NoError(t, err)
			assert.Equal(t, term.Output(), "User is banned.\n")
		})
		t.Run("user is not banned", func(t *testing.T) {
			// given
			signup := &toolchainv1alpha1.UserSignup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "theUser",
					Namespace: test.HostOperatorNs,
				},
				Spec: toolchainv1alpha1.UserSignupSpec{
					IdentityClaims: toolchainv1alpha1.IdentityClaimsEmbedded{
						PropagatedClaims: toolchainv1alpha1.PropagatedClaims{
							Email: "me@home",
						},
					},
				},
			}

			newClient, _ := NewFakeClients(t, signup)
			SetFileConfig(t, Host())
			term := NewFakeTerminal()
			ctx := clicontext.NewCommandContext(term, newClient)

			// when
			err := cmd.CheckBanned(ctx, "", "theUser", "")

			// then
			require.NoError(t, err)
			assert.Equal(t, term.Output(), "User is NOT banned.\n")
		})
		t.Run("usersignup is not found", func(t *testing.T) {
			// given
			newClient, _ := NewFakeClients(t)
			SetFileConfig(t, Host())
			term := NewFakeTerminal()
			ctx := clicontext.NewCommandContext(term, newClient)

			// when
			err := cmd.CheckBanned(ctx, "", "theUser", "")

			// then
			require.NoError(t, err)
			assert.Equal(t, term.Output(), "User not found.\n")
		})
	})
	t.Run("finds the banned user by the email", func(t *testing.T) {
		t.Run("user is banned", func(t *testing.T) {
			// given
			bannedUser := &toolchainv1alpha1.BannedUser{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "arbitrary-name",
					Namespace: test.HostOperatorNs,
					Labels: map[string]string{
						toolchainv1alpha1.BannedUserEmailHashLabelKey: hash.EncodeString("me@home"),
					},
				},
				Spec: toolchainv1alpha1.BannedUserSpec{
					Email: "me@home",
				},
			}

			newClient, _ := NewFakeClients(t, bannedUser)
			SetFileConfig(t, Host())
			term := NewFakeTerminal()
			ctx := clicontext.NewCommandContext(term, newClient)

			// when
			err := cmd.CheckBanned(ctx, "", "", "me@home")

			// then
			require.NoError(t, err)
			assert.Equal(t, term.Output(), "User is banned.\n")
		})
		t.Run("user is not banned", func(t *testing.T) {
			// given

			newClient, _ := NewFakeClients(t)
			SetFileConfig(t, Host())
			term := NewFakeTerminal()
			ctx := clicontext.NewCommandContext(term, newClient)

			// when
			err := cmd.CheckBanned(ctx, "", "", "me@home")

			// then
			require.NoError(t, err)
			assert.Equal(t, term.Output(), "User is NOT banned.\n")
		})
	})
}

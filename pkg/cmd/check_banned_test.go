package cmd_test

import (
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/hash"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/kubesaw/ksctl/pkg/cmd"
	clicontext "github.com/kubesaw/ksctl/pkg/context"
	. "github.com/kubesaw/ksctl/pkg/test"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestCheckBanned(t *testing.T) {
	t.Run("fails with wrong number of parameters", func(t *testing.T) {
		// given
		test := func(t *testing.T, mur, signup, email, expectedErr string) {
			// given
			c := cmd.NewCheckBannedCommand()
			args := []string{}
			if mur != "" {
				args = append(args, "--mur", mur)
			}
			if signup != "" {
				args = append(args, "--signup", signup)
			}
			if email != "" {
				args = append(args, "--email", email)
			}
			c.RunE = func(cmd *cobra.Command, args []string) error {
				assert.Fail(t, "validation should have failed")
				return nil
			}
			c.SetArgs(args)

			// when
			err := c.Execute()

			// then
			require.Error(t, err)
			assert.Equal(t, expectedErr, err.Error())
		}

		t.Run("no parameters", func(t *testing.T) {
			test(t, "", "", "", "at least one of the flags in the group [mur signup email] is required")
		})
		t.Run("mur+signup", func(t *testing.T) {
			test(t, "mur", "signup", "", "if any flags in the group [mur signup email] are set none of the others can be; [mur signup] were all set")
		})
		t.Run("mur+email", func(t *testing.T) {
			test(t, "mur", "", "email", "if any flags in the group [mur signup email] are set none of the others can be; [email mur] were all set")
		})
		t.Run("signup+email", func(t *testing.T) {
			test(t, "", "signup", "email", "if any flags in the group [mur signup email] are set none of the others can be; [email signup] were all set")
		})
		t.Run("mur+signup+email", func(t *testing.T) {
			test(t, "mur", "signup", "email", "if any flags in the group [mur signup email] are set none of the others can be; [email mur signup] were all set")
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
			assert.Contains(t, term.Output(), "User is banned.\n")
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
			assert.Equal(t, "User is NOT banned.\n", term.Output())
		})
		t.Run("mur is not found, no usersignup with compliant username", func(t *testing.T) {
			// given
			newClient, _ := NewFakeClients(t)
			SetFileConfig(t, Host())
			term := NewFakeTerminal()
			ctx := clicontext.NewCommandContext(term, newClient)

			// when
			err := cmd.CheckBanned(ctx, "theUser", "", "")

			// then
			require.NoError(t, err)
			assert.Equal(t, "User not found.\n", term.Output())
		})
		t.Run("mur is not found, usersignup with compliant username exists", func(t *testing.T) {
			userSignup := &toolchainv1alpha1.UserSignup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "usersignup",
					Namespace: test.HostOperatorNs,
				},
				Spec: toolchainv1alpha1.UserSignupSpec{
					IdentityClaims: toolchainv1alpha1.IdentityClaimsEmbedded{
						PropagatedClaims: toolchainv1alpha1.PropagatedClaims{
							Email: "me@home",
						},
					},
				},
				Status: toolchainv1alpha1.UserSignupStatus{
					CompliantUsername: "theUser",
				},
			}

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

				newClient, _ := NewFakeClients(t, userSignup, bannedUser)
				SetFileConfig(t, Host())
				term := NewFakeTerminal()
				ctx := clicontext.NewCommandContext(term, newClient)

				// when
				err := cmd.CheckBanned(ctx, "theUser", "", "")

				// then
				require.NoError(t, err)
				assert.Contains(t, term.Output(), "User is banned.\n")
			})
			t.Run("user is not banned", func(t *testing.T) {
				// given
				newClient, _ := NewFakeClients(t, userSignup)
				SetFileConfig(t, Host())
				term := NewFakeTerminal()
				ctx := clicontext.NewCommandContext(term, newClient)

				// when
				err := cmd.CheckBanned(ctx, "theUser", "", "")

				// then
				require.NoError(t, err)
				assert.Equal(t, "User is NOT banned.\n", term.Output())
			})
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
			assert.Contains(t, term.Output(), "User is banned.\n")
		})
		t.Run("usersignup is not found", func(t *testing.T) {
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
				assert.Equal(t, "User is NOT banned.\n", term.Output())
			})
			// given
			newClient, _ := NewFakeClients(t)
			SetFileConfig(t, Host())
			term := NewFakeTerminal()
			ctx := clicontext.NewCommandContext(term, newClient)

			// when
			err := cmd.CheckBanned(ctx, "", "theUser", "")

			// then
			require.NoError(t, err)
			assert.Equal(t, "User not found.\n", term.Output())
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
			assert.Contains(t, term.Output(), "User is banned.\n")
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
			assert.Equal(t, "User is NOT banned.\n", term.Output())
		})
	})
}

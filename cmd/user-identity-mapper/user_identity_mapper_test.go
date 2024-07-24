package main_test

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	useridentitymapper "github.com/kubesaw/ksctl/cmd/user-identity-mapper"

	"github.com/charmbracelet/log"
	userv1 "github.com/openshift/api/user/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func TestUserIdentityMapper(t *testing.T) {

	// given
	s := scheme.Scheme
	err := userv1.Install(s)
	require.NoError(t, err)
	user1 := &userv1.User{
		ObjectMeta: metav1.ObjectMeta{
			Name: "user1",
			Labels: map[string]string{
				"provider": "sandbox-sre",
			},
		},
	}
	identity1 := &userv1.Identity{
		ObjectMeta: metav1.ObjectMeta{
			Name: "identity1",
			Labels: map[string]string{
				"provider": "sandbox-sre",
				"username": "user1",
			},
		},
	}
	user2 := &userv1.User{
		ObjectMeta: metav1.ObjectMeta{
			Name: "user2",
			Labels: map[string]string{
				"provider": "sandbox-sre",
			},
		},
	}
	identity2 := &userv1.Identity{
		ObjectMeta: metav1.ObjectMeta{
			Name: "identity2",
			Labels: map[string]string{
				"provider": "sandbox-sre",
				"username": "user2",
			},
		},
	}
	user3 := &userv1.User{
		ObjectMeta: metav1.ObjectMeta{
			Name: "user3",
			// not managed by sandbox-sre
		},
	}
	identity3 := &userv1.Identity{
		ObjectMeta: metav1.ObjectMeta{
			Name: "identity3",
			Labels: map[string]string{
				"provider": "sandbox-sre",
				"username": "user3",
			},
		},
	}

	t.Run("success", func(t *testing.T) {
		// given
		out := new(bytes.Buffer)
		logger := log.New(out)
		cl := test.NewFakeClient(t, user1, identity1, user2, identity2, user3, identity3)

		// when
		err := useridentitymapper.CreateUserIdentityMappings(context.TODO(), logger, cl)

		// then
		require.NoError(t, err)
		assert.NotContains(t, out.String(), "unable to list identities")
		uim := &userv1.UserIdentityMapping{}
		// `user1` and `user2` are not managed by sandbox (ie, labelled with `provider: sandbox-sre`), hence the `UserIdentityMappings` exist
		require.NoError(t, cl.Get(context.TODO(), types.NamespacedName{Name: identity1.Name}, uim))
		assert.Equal(t, identity1.Name, uim.Identity.Name)
		assert.Equal(t, user1.Name, uim.User.Name)
		require.NoError(t, cl.Get(context.TODO(), types.NamespacedName{Name: identity2.Name}, uim))
		assert.Equal(t, identity2.Name, uim.Identity.Name)
		assert.Equal(t, user2.Name, uim.User.Name)
	})

	t.Run("failures", func(t *testing.T) {

		t.Run("user and identities not labelled", func(t *testing.T) {
			// given
			out := new(bytes.Buffer)
			logger := log.New(out)
			cl := test.NewFakeClient(t, user3, identity3)

			// when
			err := useridentitymapper.CreateUserIdentityMappings(context.TODO(), logger, cl)

			// then
			require.NoError(t, err)
			assert.NotContains(t, out.String(), "unable to list identities")
			// `user3` is not managed by sandbox (ie, not labelled with `provider: sandbox-sre`), , hence the `UserIdentityMappings` does not exist
			require.EqualError(t, cl.Get(context.TODO(), types.NamespacedName{Name: identity3.Name}, &userv1.UserIdentityMapping{}), `useridentitymappings.user.openshift.io "identity3" not found`)
		})

		t.Run("missing identity", func(t *testing.T) {
			// given
			out := new(bytes.Buffer)
			logger := log.New(out)
			cl := test.NewFakeClient(t, user1)

			// when
			err := useridentitymapper.CreateUserIdentityMappings(context.TODO(), logger, cl)

			// then
			require.NoError(t, err)
			assert.Contains(t, out.String(), `no identity associated with user "user1"`)
			require.EqualError(t, cl.Get(context.TODO(), types.NamespacedName{Name: identity1.Name}, &userv1.UserIdentityMapping{}), `useridentitymappings.user.openshift.io "identity1" not found`)
		})

		t.Run("cannot list users", func(t *testing.T) {
			// given
			out := new(bytes.Buffer)
			logger := log.New(out)
			cl := test.NewFakeClient(t, user1, identity1)
			cl.MockList = func(ctx context.Context, list runtimeclient.ObjectList, opts ...runtimeclient.ListOption) error {
				if _, ok := list.(*userv1.UserList); ok {
					return fmt.Errorf("mock error")
				}
				return cl.Client.List(ctx, list, opts...)
			}
			// when
			err := useridentitymapper.CreateUserIdentityMappings(context.TODO(), logger, cl)

			// then
			require.EqualError(t, err, "unable to list users: mock error")
		})

		t.Run("cannot list identities", func(t *testing.T) {
			// given
			out := new(bytes.Buffer)
			logger := log.New(out)
			cl := test.NewFakeClient(t, user1, identity1, user2, identity2)
			cl.MockList = func(ctx context.Context, list runtimeclient.ObjectList, opts ...runtimeclient.ListOption) error {
				if _, ok := list.(*userv1.IdentityList); ok {
					return fmt.Errorf("mock error")
				}
				return cl.Client.List(ctx, list, opts...)
			}

			// when
			err := useridentitymapper.CreateUserIdentityMappings(context.TODO(), logger, cl)

			// then
			require.EqualError(t, err, "unable to list identities: mock error")
		})

		t.Run("cannot create user-identity mapping", func(t *testing.T) {
			// given
			out := new(bytes.Buffer)
			logger := log.New(out)
			cl := test.NewFakeClient(t, user1, identity1, user2, identity2)
			cl.MockCreate = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.CreateOption) error {
				if _, ok := obj.(*userv1.UserIdentityMapping); ok {
					return fmt.Errorf("mock error")
				}
				return cl.Client.Create(ctx, obj, opts...)
			}

			// when
			err := useridentitymapper.CreateUserIdentityMappings(context.TODO(), logger, cl)

			// then
			require.EqualError(t, err, `unable to create identity mapping for username "user1" and identity "identity1": mock error`)
		})
	})
}

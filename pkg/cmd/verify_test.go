package cmd_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/states"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/kubesaw/ksctl/pkg/cmd"
	"github.com/kubesaw/ksctl/pkg/configuration"
	clicontext "github.com/kubesaw/ksctl/pkg/context"
	. "github.com/kubesaw/ksctl/pkg/test"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func TestVerifyCmd(t *testing.T) {
	for name, tc := range map[string]struct {
		args        []string
		expectedErr string
	}{
		"fails when neither name nor email is set": {
			expectedErr: "you must specify one of 'name' and `email` flags",
		},
		"fails when both name and email are set": {
			args:        []string{"--name", "someone", "--email", "someone@example.com"},
			expectedErr: "you cannot specify both 'name' and `email` flags",
		},
		"runs with name flag": {
			args: []string{"--name", "someone"},
		},
		"runs with email flag": {
			args: []string{"--email", "someone@example.com"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			// given
			c := cmd.NewVerifyCmd()
			c.SetArgs(tc.args)
			commandRan := false
			c.RunE = func(_ *cobra.Command, args []string) error {
				if tc.expectedErr != "" {
					assert.Fail(t, "validation should have failed")
					return nil
				}
				require.Empty(t, args)
				commandRan = true
				return nil
			}

			// when
			err := c.Execute()

			// then
			if tc.expectedErr != "" {
				require.EqualError(t, err, tc.expectedErr)
				return
			}
			require.NoError(t, err)
			assert.True(t, commandRan)
		})
	}
}

func TestVerify(t *testing.T) {

	t.Run("when answer is Y", func(t *testing.T) {
		// given
		userSignup := newUserSignupToVerify()
		newClient, fakeClient := NewFakeClients(t, userSignup)
		SetFileConfig(t, Host())
		term := NewFakeTerminalWithResponse("Y")
		ctx := clicontext.NewCommandContext(term, newClient)

		// when
		err := cmd.Verify(ctx, dummyGet(userSignup))

		// then
		require.NoError(t, err)
		AssertUserSignupSpec(t, fakeClient, userSignup)
		assertVerifiedTimestamp(t, fakeClient, userSignup)
		output := term.Output()
		assert.Contains(t, output, "Are you sure that you want to verify the UserSignup above?")
		assert.Contains(t, output, "UserSignup has been verified")
		assert.NotContains(t, output, "cool-token")
	})

	t.Run("when answer is N", func(t *testing.T) {
		// given
		userSignup := newUserSignupToVerify()
		newClient, fakeClient := NewFakeClients(t, userSignup)
		SetFileConfig(t, Host())
		term := NewFakeTerminalWithResponse("n")
		ctx := clicontext.NewCommandContext(term, newClient)

		// when
		err := cmd.Verify(ctx, dummyGet(userSignup))

		// then
		require.NoError(t, err)
		AssertUserSignupSpec(t, fakeClient, userSignup)
		assertNoVerifiedTimestamp(t, fakeClient, userSignup)
		output := term.Output()
		assert.Contains(t, output, "Are you sure that you want to verify the UserSignup above?")
		assert.NotContains(t, output, "UserSignup has been verified")
		assert.NotContains(t, output, "cool-token")
	})

	t.Run("clears rejected and verification-required states", func(t *testing.T) {
		// given
		userSignup := newUserSignupToVerify()
		states.SetRejected(userSignup, true)
		states.SetVerificationRequired(userSignup, true)
		newClient, fakeClient := NewFakeClients(t, userSignup)
		SetFileConfig(t, Host())
		term := NewFakeTerminalWithResponse("Y")
		ctx := clicontext.NewCommandContext(term, newClient)

		// when
		err := cmd.Verify(ctx, dummyGet(userSignup))

		// then
		require.NoError(t, err)
		states.SetVerificationRequired(userSignup, false)
		states.SetRejected(userSignup, false)
		AssertUserSignupSpec(t, fakeClient, userSignup)
		assertVerifiedTimestamp(t, fakeClient, userSignup)
	})

	t.Run("when getting usersignup failed", func(t *testing.T) {
		// given
		userSignup := newUserSignupToVerify()
		newClient, fakeClient := NewFakeClients(t, userSignup)
		SetFileConfig(t, Host())
		term := NewFakeTerminalWithResponse("Y")
		ctx := clicontext.NewCommandContext(term, newClient)

		// when
		err := cmd.Verify(ctx, func(configuration.ClusterConfig, runtimeclient.Client) (*toolchainv1alpha1.UserSignup, error) {
			return nil, fmt.Errorf("mock error")
		})

		// then
		require.EqualError(t, err, "mock error")
		AssertUserSignupSpec(t, fakeClient, userSignup)
		assertNoVerifiedTimestamp(t, fakeClient, userSignup)
		output := term.Output()
		assert.NotContains(t, output, "Are you sure that you want to verify the UserSignup above?")
		assert.NotContains(t, output, "UserSignup has been verified")
		assert.NotContains(t, output, "cool-token")
	})
}

func newUserSignupToVerify() *toolchainv1alpha1.UserSignup {
	userSignup := NewUserSignup()
	userSignup.Annotations = map[string]string{}
	return userSignup
}

func assertVerifiedTimestamp(t *testing.T, fakeClient *test.FakeClient, userSignup *toolchainv1alpha1.UserSignup) {
	t.Helper()
	updated := &toolchainv1alpha1.UserSignup{}
	err := fakeClient.Get(context.TODO(), test.NamespacedName(userSignup.Namespace, userSignup.Name), updated)
	require.NoError(t, err)
	ts, found := updated.Annotations[toolchainv1alpha1.UserSignupVerifiedTimestampAnnotationKey]
	require.True(t, found)
	parsed, err := time.Parse(time.RFC3339, ts)
	require.NoError(t, err)
	assert.WithinDuration(t, time.Now(), parsed, 5*time.Second)
}

func assertNoVerifiedTimestamp(t *testing.T, fakeClient *test.FakeClient, userSignup *toolchainv1alpha1.UserSignup) {
	t.Helper()
	updated := &toolchainv1alpha1.UserSignup{}
	err := fakeClient.Get(context.TODO(), test.NamespacedName(userSignup.Namespace, userSignup.Name), updated)
	require.NoError(t, err)
	_, found := updated.Annotations[toolchainv1alpha1.UserSignupVerifiedTimestampAnnotationKey]
	assert.False(t, found)
}

package cmd_test

import (
	"context"
	"fmt"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/states"
	"github.com/kubesaw/ksctl/pkg/cmd"
	"github.com/kubesaw/ksctl/pkg/configuration"
	clicontext "github.com/kubesaw/ksctl/pkg/context"
	. "github.com/kubesaw/ksctl/pkg/test"
	"k8s.io/apimachinery/pkg/api/errors"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func TestApprove(t *testing.T) {

	t.Run("when answer is Y", func(t *testing.T) {
		// given
		userSignup := NewUserSignup()
		newClient, newRESTClient, fakeClient := NewFakeClients(t, userSignup)
		SetFileConfig(t, Host())
		term := NewFakeTerminalWithResponse("Y")
		ctx := clicontext.NewCommandContext(term, newClient, newRESTClient)

		// when
		err := cmd.Approve(ctx, dummyGet(userSignup), false, "")

		// then
		require.NoError(t, err)
		states.SetApprovedManually(userSignup, true)
		states.SetVerificationRequired(userSignup, false)
		states.SetDeactivated(userSignup, false)
		AssertUserSignupSpec(t, fakeClient, userSignup)
		output := term.Output()
		assert.Contains(t, output, "Are you sure that you want to approve the UserSignup above?")
		assert.Contains(t, output, "UserSignup has been approved")
		assert.NotContains(t, output, "cool-token")
	})

	t.Run("when answer is N", func(t *testing.T) {
		// given
		userSignup := NewUserSignup()
		newClient, newRESTClient, fakeClient := NewFakeClients(t, userSignup)
		SetFileConfig(t, Host())
		term := NewFakeTerminalWithResponse("n")
		ctx := clicontext.NewCommandContext(term, newClient, newRESTClient)

		// when
		err := cmd.Approve(ctx, dummyGet(userSignup), false, "")

		// then
		require.NoError(t, err)
		AssertUserSignupSpec(t, fakeClient, userSignup)
		output := term.Output()
		assert.Contains(t, output, "Are you sure that you want to approve the UserSignup above?")
		assert.NotContains(t, output, "UserSignup has been approved")
		assert.NotContains(t, output, "cool-token")
	})

	t.Run("reactivate deactivated user", func(t *testing.T) {
		// given
		userSignup := NewUserSignup(UserSignupDeactivated(true))
		newClient, newRESTClient, fakeClient := NewFakeClients(t, userSignup)
		SetFileConfig(t, Host())
		term := NewFakeTerminalWithResponse("Y")
		ctx := clicontext.NewCommandContext(term, newClient, newRESTClient)

		// when
		err := cmd.Approve(ctx, dummyGet(userSignup), false, "")

		// then
		require.NoError(t, err)
		states.SetApprovedManually(userSignup, true)
		states.SetVerificationRequired(userSignup, false)
		states.SetDeactivated(userSignup, false)
		AssertUserSignupSpec(t, fakeClient, userSignup)
	})

	t.Run("user is already active - automatically approved", func(t *testing.T) {
		// given
		userSignup := NewUserSignup(UserSignupAutomaticallyApproved(true))
		newClient, newRESTClient, fakeClient := NewFakeClients(t, userSignup)
		SetFileConfig(t, Host())
		term := NewFakeTerminalWithResponse("Y")
		ctx := clicontext.NewCommandContext(term, newClient, newRESTClient)

		// when
		err := cmd.Approve(ctx, dummyGet(userSignup), false, "")

		// then
		require.EqualError(t, err, fmt.Sprintf(`UserSignup "%s" is already approved`, userSignup.Name))
		AssertUserSignupSpec(t, fakeClient, userSignup)
	})

	t.Run("user is already active - approved by admin", func(t *testing.T) {
		// given
		userSignup := NewUserSignup(UserSignupApprovedByAdmin(true))
		newClient, newRESTClient, fakeClient := NewFakeClients(t, userSignup)
		SetFileConfig(t, Host())
		term := NewFakeTerminalWithResponse("Y")
		ctx := clicontext.NewCommandContext(term, newClient, newRESTClient)

		// when
		err := cmd.Approve(ctx, dummyGet(userSignup), false, "")

		// then
		require.EqualError(t, err, fmt.Sprintf(`UserSignup "%s" is already approved`, userSignup.Name))
		states.SetApprovedManually(userSignup, true) // there's an explicit `spec.state` entry when manually approved
		AssertUserSignupSpec(t, fakeClient, userSignup)
	})

	t.Run("when usersignup is already approved", func(t *testing.T) {
		// given
		userSignup := NewUserSignup(UserSignupAutomaticallyApproved(true))
		newClient, newRESTClient, fakeClient := NewFakeClients(t, userSignup)
		SetFileConfig(t, Host())
		term := NewFakeTerminalWithResponse("Y")
		ctx := clicontext.NewCommandContext(term, newClient, newRESTClient)

		// when
		err := cmd.Approve(ctx, dummyGet(userSignup), false, "")

		// then
		require.EqualError(t, err, fmt.Sprintf(`UserSignup "%s" is already approved`, userSignup.Name))
		AssertUserSignupSpec(t, fakeClient, userSignup)
		output := term.Output()
		assert.NotContains(t, output, "Are you sure that you want to approve the UserSignup above?")
		assert.NotContains(t, output, "UserSignup has been approved")
		assert.NotContains(t, output, "cool-token")
	})

	t.Run("when getting usersignup failed", func(t *testing.T) {
		// given
		userSignup := NewUserSignup()
		newClient, newRESTClient, fakeClient := NewFakeClients(t, userSignup)
		SetFileConfig(t, Host())
		term := NewFakeTerminalWithResponse("Y")
		ctx := clicontext.NewCommandContext(term, newClient, newRESTClient)

		// when
		err := cmd.Approve(ctx, func(configuration.ClusterConfig, runtimeclient.Client) (*toolchainv1alpha1.UserSignup, error) {
			return nil, fmt.Errorf("mock error")
		}, false, "")

		// then
		require.EqualError(t, err, "mock error")
		AssertUserSignupSpec(t, fakeClient, userSignup)
		output := term.Output()
		assert.NotContains(t, output, "Are you sure that you want to approve the UserSignup above?")
		assert.NotContains(t, output, "UserSignup has been approved")
		assert.NotContains(t, output, "cool-token")
	})

	t.Run("with phone check variations", func(t *testing.T) {

		t.Run("when usersignup has phone hash", func(t *testing.T) {
			// given
			userSignup := NewUserSignup()
			newClient, newRESTClient, fakeClient := NewFakeClients(t, userSignup)
			SetFileConfig(t, Host())
			term := NewFakeTerminalWithResponse("Y")
			ctx := clicontext.NewCommandContext(term, newClient, newRESTClient)

			// when
			err := cmd.Approve(ctx, dummyGet(userSignup), false, "")

			// then
			require.NoError(t, err)
			states.SetApprovedManually(userSignup, true)
			states.SetVerificationRequired(userSignup, false)
			states.SetDeactivated(userSignup, false)
			AssertUserSignupSpec(t, fakeClient, userSignup)
			output := term.Output()
			assert.Contains(t, output, "Are you sure that you want to approve the UserSignup above?")
			assert.Contains(t, output, "UserSignup has been approved")
			assert.NotContains(t, output, "cool-token")
		})

		t.Run("when usersignup doesn't have phone hash but skip phone verification flag is set", func(t *testing.T) {
			// given
			userSignup := NewUserSignup(UserSignupRemoveLabel(toolchainv1alpha1.UserSignupUserPhoneHashLabelKey))
			newClient, newRESTClient, fakeClient := NewFakeClients(t, userSignup)
			SetFileConfig(t, Host())
			term := NewFakeTerminalWithResponse("Y")
			ctx := clicontext.NewCommandContext(term, newClient, newRESTClient)

			// when
			err := cmd.Approve(ctx, dummyGet(userSignup), true, "")

			// then
			require.NoError(t, err)
			states.SetApprovedManually(userSignup, true)
			states.SetVerificationRequired(userSignup, false)
			states.SetDeactivated(userSignup, false)
			AssertUserSignupSpec(t, fakeClient, userSignup)
			output := term.Output()
			assert.Contains(t, output, "Are you sure that you want to approve the UserSignup above?")
			assert.Contains(t, output, "UserSignup has been approved")
			assert.NotContains(t, output, "cool-token")
		})

		t.Run("when usersignup doesn't have phone hash", func(t *testing.T) {
			// given
			userSignup := NewUserSignup(UserSignupRemoveLabel(toolchainv1alpha1.UserSignupUserPhoneHashLabelKey))
			newClient, newRESTClient, fakeClient := NewFakeClients(t, userSignup)
			SetFileConfig(t, Host())
			term := NewFakeTerminalWithResponse("Y")
			ctx := clicontext.NewCommandContext(term, newClient, newRESTClient)

			// when
			err := cmd.Approve(ctx, dummyGet(userSignup), false, "")

			// then
			require.EqualError(t, err, fmt.Sprintf(`UserSignup "%s" is missing a phone hash label - the user may not have provided a phone number for verification. In most cases, the user should be asked to attempt the phone verification process. For exceptions, skip this check using the --skip-phone-check parameter`, userSignup.Name))
			AssertUserSignupSpec(t, fakeClient, userSignup)
			output := term.Output()
			assert.NotContains(t, output, "Are you sure that you want to approve the UserSignup above?")
			assert.NotContains(t, output, "UserSignup has been approved")
			assert.NotContains(t, output, "cool-token")
		})
	})

	t.Run("with targetCluster variations", func(t *testing.T) {
		t.Run("when targetCluster is valid", func(t *testing.T) {
			// given
			userSignup := NewUserSignup()
			newClient, newRESTClient, fakeClient := NewFakeClients(t, userSignup)
			SetFileConfig(t,
				Host(),
				Member(ClusterName("member1"), ServerName("m1.devcluster.openshift.com")),
				Member(ClusterName("member2"), ServerName("m2.devcluster.openshift.com")))
			term := NewFakeTerminalWithResponse("Y")
			ctx := clicontext.NewCommandContext(term, newClient, newRESTClient)

			// when
			err := cmd.Approve(ctx, dummyGet(userSignup), false, "member1")

			// then
			require.NoError(t, err)
			states.SetApprovedManually(userSignup, true)
			states.SetVerificationRequired(userSignup, false)
			states.SetDeactivated(userSignup, false)
			// check the expected target cluster matches with the actual one
			userSignup.Spec.TargetCluster = "member-m1.devcluster.openshift.com"
			AssertUserSignupSpec(t, fakeClient, userSignup)
			output := term.Output()
			assert.Contains(t, output, "Are you sure that you want to approve the UserSignup above?")
			assert.Contains(t, output, "UserSignup has been approved")
			assert.NotContains(t, output, "cool-token")
		})

		t.Run("when targetCluster is invalid", func(t *testing.T) {
			// given
			userSignup := NewUserSignup()
			newClient, newRESTClient, _ := NewFakeClients(t, userSignup)
			SetFileConfig(t,
				Host(),
				Member(ClusterName("member1"), ServerName("m1.devcluster.openshift.com")),
				Member(ClusterName("member2"), ServerName("m2.devcluster.openshift.com")))
			term := NewFakeTerminalWithResponse("Y")
			ctx := clicontext.NewCommandContext(term, newClient, newRESTClient)

			// when
			err := cmd.Approve(ctx, dummyGet(userSignup), false, "non-existent-member")

			// then
			require.Error(t, err)
			assert.Contains(t, err.Error(), "the provided cluster-name 'non-existent-member' is not present in your ksctl.yaml file")
		})
	})
}

func dummyGet(userSignup *toolchainv1alpha1.UserSignup) cmd.LookupUserSignup {
	return func(configuration.ClusterConfig, runtimeclient.Client) (*toolchainv1alpha1.UserSignup, error) {
		return userSignup, nil
	}
}

func TestLookupUserSignupByName(t *testing.T) {

	t.Run("when user is found", func(t *testing.T) {
		userSignup := NewUserSignup()
		_, _, fakeClient := NewFakeClients(t, userSignup)
		SetFileConfig(t, Host())
		term := NewFakeTerminalWithResponse("Y")
		cfg, err := configuration.LoadClusterConfig(term, "host")
		require.NoError(t, err)

		// when
		result, err := cmd.ByName(userSignup.Name)(cfg, fakeClient)

		// then
		require.NoError(t, err)
		assert.Equal(t, userSignup.Name, result.Name) // comparing the resource names is enough (no need to deal with kind, group/version, etc.)
	})

	t.Run("when user is unknown", func(t *testing.T) {
		_, _, fakeClient := NewFakeClients(t)
		SetFileConfig(t, Host())
		term := NewFakeTerminalWithResponse("Y")
		cfg, err := configuration.LoadClusterConfig(term, "host")
		require.NoError(t, err)

		// when
		_, err = cmd.ByName("unknown")(cfg, fakeClient)

		// then
		require.Error(t, err)
		assert.True(t, errors.IsNotFound(err))
	})

	t.Run("when error occurrs", func(t *testing.T) {
		userSignup := NewUserSignup()
		_, _, fakeClient := NewFakeClients(t, userSignup)
		fakeClient.MockGet = func(ctx context.Context, key runtimeclient.ObjectKey, obj runtimeclient.Object, opts ...runtimeclient.GetOption) error {
			return fmt.Errorf("mock error")
		}
		SetFileConfig(t, Host())
		term := NewFakeTerminalWithResponse("Y")
		cfg, err := configuration.LoadClusterConfig(term, "host")
		require.NoError(t, err)

		// when
		_, err = cmd.ByName(userSignup.Name)(cfg, fakeClient)

		// then
		require.EqualError(t, err, "mock error")
	})
}

func TestLookupUserSignupByEmailAddress(t *testing.T) {

	t.Run("when user is found", func(t *testing.T) {
		userSignup := NewUserSignup()
		_, _, fakeClient := NewFakeClients(t, userSignup)
		SetFileConfig(t, Host())
		term := NewFakeTerminalWithResponse("Y")
		cfg, err := configuration.LoadClusterConfig(term, "host")
		require.NoError(t, err)

		// when
		result, err := cmd.ByEmailAddress(userSignup.Spec.IdentityClaims.Email)(cfg, fakeClient)

		// then
		require.NoError(t, err)
		assert.Equal(t, userSignup.Name, result.Name) // comparing the resource names is enough (no need to deal with kind, group/version, etc.)
	})

	t.Run("when no match found", func(t *testing.T) {
		userSignup := NewUserSignup()
		_, _, fakeClient := NewFakeClients(t, userSignup)
		SetFileConfig(t, Host())
		term := NewFakeTerminalWithResponse("Y")
		cfg, err := configuration.LoadClusterConfig(term, "host")
		require.NoError(t, err)

		// when
		_, err = cmd.ByEmailAddress("unknown@redhat.com")(cfg, fakeClient)

		// then
		require.EqualError(t, err, "expected a single match with the email address, but found 0")
	})

	t.Run("when too many matches found", func(t *testing.T) {
		userSignup1 := NewUserSignup()
		userSignup2 := NewUserSignup() // same email address as userSignup1
		_, _, fakeClient := NewFakeClients(t, userSignup1, userSignup2)
		SetFileConfig(t, Host())
		term := NewFakeTerminalWithResponse("Y")
		cfg, err := configuration.LoadClusterConfig(term, "host")
		require.NoError(t, err)

		// when
		_, err = cmd.ByEmailAddress(userSignup1.Spec.IdentityClaims.Email)(cfg, fakeClient)

		// then
		require.EqualError(t, err, "expected a single match with the email address, but found 2")
	})

	t.Run("when error occurrs", func(t *testing.T) {
		userSignup := NewUserSignup()
		_, _, fakeClient := NewFakeClients(t, userSignup)
		fakeClient.MockList = func(ctx context.Context, list runtimeclient.ObjectList, opts ...runtimeclient.ListOption) error {
			return fmt.Errorf("mock error")
		}
		SetFileConfig(t, Host())
		term := NewFakeTerminalWithResponse("Y")
		cfg, err := configuration.LoadClusterConfig(term, "host")
		require.NoError(t, err)

		// when
		_, err = cmd.ByEmailAddress(userSignup.Spec.IdentityClaims.Email)(cfg, fakeClient)

		// then
		require.EqualError(t, err, "mock error")
	})
}

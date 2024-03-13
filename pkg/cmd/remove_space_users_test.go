package cmd_test

import (
	"context"
	"fmt"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/spacebinding"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/codeready-toolchain/toolchain-common/pkg/test/masteruserrecord"
	"github.com/kubesaw/ksctl/pkg/cmd"
	clicontext "github.com/kubesaw/ksctl/pkg/context"
	. "github.com/kubesaw/ksctl/pkg/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func TestRemoveSpaceUsers(t *testing.T) {

	t.Run("when answer is Y", func(t *testing.T) {
		t.Run("when both spacebindings are deleted", func(t *testing.T) {
			// given
			space := newSpace()
			mur1 := masteruserrecord.NewMasterUserRecord(t, "alice", masteruserrecord.TierName("deactivate30"))
			sb1 := spacebinding.NewSpaceBinding(mur1, space, "alice")
			mur2 := masteruserrecord.NewMasterUserRecord(t, "bob", masteruserrecord.TierName("deactivate30"))
			sb2 := spacebinding.NewSpaceBinding(mur2, space, "bob")
			newClient, newRESTClient, fakeClient := NewFakeClients(t, space, sb1, sb2)

			SetFileConfig(t, Host())
			term := NewFakeTerminalWithResponse("Y")
			ctx := clicontext.NewCommandContext(term, newClient, newRESTClient)

			// when
			err := cmd.RemoveSpaceUsers(ctx, "testspace", []string{"alice", "bob"})

			// then
			require.NoError(t, err)
			output := term.Output()
			assertSpaceBindingsRemaining(t, fakeClient, []string{}) // should be deleted
			assert.Contains(t, output, "Are you sure that you want to remove users from the above Space?")
			assert.Contains(t, output, "SpaceBinding(s) successfully deleted")
			assert.NotContains(t, output, "cool-token")
		})

		t.Run("when only one spacebinding is deleted", func(t *testing.T) {
			// given
			space := newSpace()
			mur1 := masteruserrecord.NewMasterUserRecord(t, "alice", masteruserrecord.TierName("deactivate30"))
			sb1 := spacebinding.NewSpaceBinding(mur1, space, "alice")
			mur2 := masteruserrecord.NewMasterUserRecord(t, "bob", masteruserrecord.TierName("deactivate30"))
			sb2 := spacebinding.NewSpaceBinding(mur2, space, "bob")
			newClient, newRESTClient, fakeClient := NewFakeClients(t, space, sb1, sb2)

			SetFileConfig(t, Host())
			term := NewFakeTerminalWithResponse("Y")
			ctx := clicontext.NewCommandContext(term, newClient, newRESTClient)

			// when
			err := cmd.RemoveSpaceUsers(ctx, "testspace", []string{"alice"})

			// then
			require.NoError(t, err)
			output := term.Output()
			assertSpaceBindingsRemaining(t, fakeClient, []string{"bob"}) // one should remain
			assert.Contains(t, output, "Are you sure that you want to remove users from the above Space?")
			assert.Contains(t, output, "SpaceBinding(s) successfully deleted")
			assert.NotContains(t, output, "cool-token")
		})
	})

	t.Run("when answer is N", func(t *testing.T) {
		// given
		space := newSpace()
		mur1 := masteruserrecord.NewMasterUserRecord(t, "alice", masteruserrecord.TierName("deactivate30"))
		sb1 := spacebinding.NewSpaceBinding(mur1, space, "alice")
		mur2 := masteruserrecord.NewMasterUserRecord(t, "bob", masteruserrecord.TierName("deactivate30"))
		sb2 := spacebinding.NewSpaceBinding(mur2, space, "bob")
		newClient, newRESTClient, fakeClient := NewFakeClients(t, space, sb1, sb2)

		SetFileConfig(t, Host())
		term := NewFakeTerminalWithResponse("N")
		ctx := clicontext.NewCommandContext(term, newClient, newRESTClient)

		// when
		err := cmd.RemoveSpaceUsers(ctx, "testspace", []string{"alice", "bob"})

		// then
		require.NoError(t, err)
		output := term.Output()
		assertSpaceBindingsRemaining(t, fakeClient, []string{"alice", "bob"}) // should not be deleted
		assert.Contains(t, output, "Are you sure that you want to remove users from the above Space?")
		assert.NotContains(t, output, "SpaceBinding(s) successfully deleted")
		assert.NotContains(t, output, "cool-token")
	})

	t.Run("when space not found", func(t *testing.T) {
		// given
		space := newSpace()
		mur1 := masteruserrecord.NewMasterUserRecord(t, "alice", masteruserrecord.TierName("deactivate30"))
		sb1 := spacebinding.NewSpaceBinding(mur1, space, "alice")
		mur2 := masteruserrecord.NewMasterUserRecord(t, "bob", masteruserrecord.TierName("deactivate30"))
		sb2 := spacebinding.NewSpaceBinding(mur2, space, "bob")
		newClient, newRESTClient, fakeClient := NewFakeClients(t, sb1, sb2) // no space

		SetFileConfig(t, Host())
		term := NewFakeTerminalWithResponse("N")
		ctx := clicontext.NewCommandContext(term, newClient, newRESTClient)

		// when
		err := cmd.RemoveSpaceUsers(ctx, "testspace", []string{"alice", "bob"})

		// then
		require.EqualError(t, err, `spaces.toolchain.dev.openshift.com "testspace" not found`)
		output := term.Output()
		assertSpaceBindingsRemaining(t, fakeClient, []string{"alice", "bob"}) // should not be deleted
		assert.NotContains(t, output, "Are you sure that you want to remove users from the above Space?")
		assert.NotContains(t, output, "SpaceBinding(s) successfully deleted")
		assert.NotContains(t, output, "cool-token")
	})

	t.Run("when mur not found", func(t *testing.T) {
		// given
		space := newSpace()
		mur1 := masteruserrecord.NewMasterUserRecord(t, "alice", masteruserrecord.TierName("deactivate30"))
		sb1 := spacebinding.NewSpaceBinding(mur1, space, "alice")
		mur2 := masteruserrecord.NewMasterUserRecord(t, "bob", masteruserrecord.TierName("deactivate30"))
		sb2 := spacebinding.NewSpaceBinding(mur2, space, "bob")
		newClient, newRESTClient, fakeClient := NewFakeClients(t, space, sb1, sb2)

		SetFileConfig(t, Host())
		term := NewFakeTerminalWithResponse("N")
		ctx := clicontext.NewCommandContext(term, newClient, newRESTClient)

		// when
		err := cmd.RemoveSpaceUsers(ctx, "testspace", []string{"alice", "notexist"})

		// then
		require.EqualError(t, err, `no SpaceBinding found for Space 'testspace' and MasterUserRecord 'notexist'`)
		output := term.Output()
		assertSpaceBindingsRemaining(t, fakeClient, []string{"alice", "bob"}) // should not be deleted
		assert.NotContains(t, output, "Are you sure that you want to remove users from the above Space?")
		assert.NotContains(t, output, "SpaceBinding(s) successfully deleted")
		assert.NotContains(t, output, "cool-token")
	})

	t.Run("client get error", func(t *testing.T) {
		// given
		space := newSpace()
		mur1 := masteruserrecord.NewMasterUserRecord(t, "alice", masteruserrecord.TierName("deactivate30"))
		sb1 := spacebinding.NewSpaceBinding(mur1, space, "alice")
		mur2 := masteruserrecord.NewMasterUserRecord(t, "bob", masteruserrecord.TierName("deactivate30"))
		sb2 := spacebinding.NewSpaceBinding(mur2, space, "bob")
		newClient, newRESTClient, fakeClient := NewFakeClients(t, space, sb1, sb2)
		fakeClient.MockGet = func(ctx context.Context, key runtimeclient.ObjectKey, obj runtimeclient.Object, opts ...runtimeclient.GetOption) error {
			return fmt.Errorf("client error")
		}

		SetFileConfig(t, Host())
		term := NewFakeTerminalWithResponse("Y")
		ctx := clicontext.NewCommandContext(term, newClient, newRESTClient)

		// when
		err := cmd.RemoveSpaceUsers(ctx, "testspace", []string{"alice", "bob"})

		// then
		require.EqualError(t, err, "client error")
		output := term.Output()
		assertSpaceBindingsRemaining(t, fakeClient, []string{"alice", "bob"})
		assert.NotContains(t, output, "Are you sure that you want to remove users from the above Space?")
		assert.NotContains(t, output, "SpaceBinding(s) successfully deleted")
		assert.NotContains(t, output, "cool-token")
	})
}

func assertSpaceBindingsRemaining(t *testing.T, fakeClient *test.FakeClient, expectedMurs []string) {

	// list all SpaceBindings for the given space
	allSpaceBindings := &toolchainv1alpha1.SpaceBindingList{}
	err := fakeClient.List(context.TODO(), allSpaceBindings, runtimeclient.InNamespace(test.HostOperatorNs), runtimeclient.MatchingLabels{
		toolchainv1alpha1.SpaceBindingSpaceLabelKey: "testspace",
	})
	require.NoError(t, err)

	// verify the expected number of SpaceBindings were created
	assert.Len(t, allSpaceBindings.Items, len(expectedMurs))

	// check that any expected MURs still have SpaceBindings
	var checked int
	for _, expectedMur := range expectedMurs {
		for _, sb := range allSpaceBindings.Items {
			if sb.Labels[toolchainv1alpha1.SpaceBindingMasterUserRecordLabelKey] == expectedMur {
				require.Equal(t, expectedMur, sb.Labels[toolchainv1alpha1.SpaceBindingMasterUserRecordLabelKey])
				require.Equal(t, expectedMur, sb.Spec.MasterUserRecord)
				require.Equal(t, "testspace", sb.Spec.Space)
				checked++
			}
		}
	}
	if checked != len(expectedMurs) {
		require.Fail(t, "some expected murs were not found")
	}
}

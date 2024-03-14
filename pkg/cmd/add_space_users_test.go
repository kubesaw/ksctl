package cmd_test

import (
	"context"
	"fmt"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/codeready-toolchain/toolchain-common/pkg/test/masteruserrecord"
	"github.com/kubesaw/ksctl/pkg/cmd"
	clicontext "github.com/kubesaw/ksctl/pkg/context"
	. "github.com/kubesaw/ksctl/pkg/test"

	uuid "github.com/satori/go.uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func TestAddSpaceUsers(t *testing.T) {

	t.Run("when answer is Y", func(t *testing.T) {
		// given
		mur1 := masteruserrecord.NewMasterUserRecord(t, "alice", masteruserrecord.TierName("deactivate30"))
		mur2 := masteruserrecord.NewMasterUserRecord(t, "bob", masteruserrecord.TierName("deactivate30"))
		newClient, newRESTClient, fakeClient := initAddSpaceUsersTest(t, mur1, mur2)

		SetFileConfig(t, Host())
		term := NewFakeTerminalWithResponse("Y")
		ctx := clicontext.NewCommandContext(term, newClient, newRESTClient)

		// when
		err := cmd.AddSpaceUsers(ctx, "testspace", "admin", []string{"alice", "bob"})

		// then
		require.NoError(t, err)
		output := term.Output()
		assertSpaceBindings(t, fakeClient, []string{"alice", "bob"}, "admin")
		assert.Contains(t, output, "Are you sure that you want to add users to the above Space?")
		assert.Contains(t, output, "SpaceBinding(s) successfully created")
		assert.NotContains(t, output, "cool-token")
	})

	t.Run("when a non-default role is specified", func(t *testing.T) {
		// given
		mur1 := masteruserrecord.NewMasterUserRecord(t, "alice", masteruserrecord.TierName("deactivate30"))
		mur2 := masteruserrecord.NewMasterUserRecord(t, "bob", masteruserrecord.TierName("deactivate30"))
		newClient, newRESTClient, fakeClient := initAddSpaceUsersTest(t, mur1, mur2)

		SetFileConfig(t, Host())
		term := NewFakeTerminalWithResponse("Y")
		ctx := clicontext.NewCommandContext(term, newClient, newRESTClient)

		// when
		err := cmd.AddSpaceUsers(ctx, "testspace", "viewer", []string{"alice", "bob"})

		// then
		require.NoError(t, err)
		output := term.Output()
		assertSpaceBindings(t, fakeClient, []string{"alice", "bob"}, "viewer")
		assert.Contains(t, output, "Are you sure that you want to add users to the above Space?")
		assert.Contains(t, output, "SpaceBinding(s) successfully created")
		assert.NotContains(t, output, "cool-token")
	})

	t.Run("when answer is N", func(t *testing.T) {
		// given
		mur1 := masteruserrecord.NewMasterUserRecord(t, "alice", masteruserrecord.TierName("deactivate30"))
		mur2 := masteruserrecord.NewMasterUserRecord(t, "bob", masteruserrecord.TierName("deactivate30"))
		newClient, newRESTClient, fakeClient := initAddSpaceUsersTest(t, mur1, mur2)

		SetFileConfig(t, Host())
		term := NewFakeTerminalWithResponse("N")
		ctx := clicontext.NewCommandContext(term, newClient, newRESTClient)

		// when
		err := cmd.AddSpaceUsers(ctx, "testspace", "admin", []string{"alice", "bob"})

		// then
		require.NoError(t, err)
		output := term.Output()
		assertSpaceBindings(t, fakeClient, []string{}, "") // no spacebindings expected
		assert.Contains(t, output, "Are you sure that you want to add users to the above Space?")
		assert.NotContains(t, output, "SpaceBinding(s) successfully created")
		assert.NotContains(t, output, "cool-token")
	})

	t.Run("when space not found", func(t *testing.T) {
		// given
		mur1 := masteruserrecord.NewMasterUserRecord(t, "alice", masteruserrecord.TierName("deactivate30"))
		mur2 := masteruserrecord.NewMasterUserRecord(t, "bob", masteruserrecord.TierName("deactivate30"))
		newClient, newRESTClient, fakeClient := NewFakeClients(t, mur1, mur2) // no space

		SetFileConfig(t, Host())
		term := NewFakeTerminalWithResponse("N")
		ctx := clicontext.NewCommandContext(term, newClient, newRESTClient)

		// when
		err := cmd.AddSpaceUsers(ctx, "testspace", "admin", []string{"alice", "bob"})

		// then
		require.EqualError(t, err, `spaces.toolchain.dev.openshift.com "testspace" not found`)
		output := term.Output()
		assertSpaceBindings(t, fakeClient, []string{}, "") // no spacebindings expected
		assert.NotContains(t, output, "Are you sure that you want to add users to the above Space?")
		assert.NotContains(t, output, "SpaceBinding(s) successfully created")
		assert.NotContains(t, output, "cool-token")
	})

	t.Run("when first mur not found", func(t *testing.T) {
		// given
		newClient, newRESTClient, fakeClient := initAddSpaceUsersTest(t) // no murs

		SetFileConfig(t, Host())
		term := NewFakeTerminalWithResponse("N")
		ctx := clicontext.NewCommandContext(term, newClient, newRESTClient)

		// when
		err := cmd.AddSpaceUsers(ctx, "testspace", "admin", []string{"alice", "bob"})

		// then
		require.EqualError(t, err, `masteruserrecords.toolchain.dev.openshift.com "alice" not found`)
		output := term.Output()
		assertSpaceBindings(t, fakeClient, []string{}, "") // no spacebindings expected
		assert.NotContains(t, output, "Are you sure that you want to add users to the above Space?")
		assert.NotContains(t, output, "SpaceBinding(s) successfully created")
		assert.NotContains(t, output, "cool-token")
	})

	t.Run("when second mur not found", func(t *testing.T) {
		// given
		mur1 := masteruserrecord.NewMasterUserRecord(t, "alice", masteruserrecord.TierName("deactivate30"))
		newClient, newRESTClient, fakeClient := initAddSpaceUsersTest(t, mur1) // mur2 missing

		SetFileConfig(t, Host())
		term := NewFakeTerminalWithResponse("N")
		ctx := clicontext.NewCommandContext(term, newClient, newRESTClient)

		// when
		err := cmd.AddSpaceUsers(ctx, "testspace", "admin", []string{"alice", "bob"})

		// then
		require.EqualError(t, err, `masteruserrecords.toolchain.dev.openshift.com "bob" not found`)
		output := term.Output()
		assertSpaceBindings(t, fakeClient, []string{}, "") // no spacebindings expected
		assert.NotContains(t, output, "Are you sure that you want to add users to the above Space?")
		assert.NotContains(t, output, "SpaceBinding(s) successfully created")
		assert.NotContains(t, output, "cool-token")
	})

	t.Run("when role is invalid", func(t *testing.T) {
		// given
		mur1 := masteruserrecord.NewMasterUserRecord(t, "alice", masteruserrecord.TierName("deactivate30"))
		newClient, newRESTClient, fakeClient := initAddSpaceUsersTest(t, mur1) // mur2 missing

		SetFileConfig(t, Host())
		term := NewFakeTerminalWithResponse("N")
		ctx := clicontext.NewCommandContext(term, newClient, newRESTClient)

		// when
		err := cmd.AddSpaceUsers(ctx, "testspace", "badrole", []string{"alice", "bob"}) // invalid role

		// then
		require.Contains(t, err.Error(), "invalid role 'badrole' for space 'testspace' - the following are valid roles:")
		require.Contains(t, err.Error(), "\nadmin\n")
		require.Contains(t, err.Error(), "\nviewer\n")
		output := term.Output()
		assertSpaceBindings(t, fakeClient, []string{}, "") // no spacebindings expected
		assert.NotContains(t, output, "Are you sure that you want to add users to the above Space?")
		assert.NotContains(t, output, "SpaceBinding(s) successfully created")
		assert.NotContains(t, output, "cool-token")
	})

	t.Run("client get error", func(t *testing.T) {
		// given
		mur1 := masteruserrecord.NewMasterUserRecord(t, "alice", masteruserrecord.TierName("deactivate30"))
		mur2 := masteruserrecord.NewMasterUserRecord(t, "bob", masteruserrecord.TierName("deactivate30"))
		newClient, newRESTClient, fakeClient := initAddSpaceUsersTest(t, mur1, mur2)
		fakeClient.MockGet = func(ctx context.Context, key runtimeclient.ObjectKey, obj runtimeclient.Object, opts ...runtimeclient.GetOption) error {
			return fmt.Errorf("client error")
		}

		SetFileConfig(t, Host())
		term := NewFakeTerminalWithResponse("Y")
		ctx := clicontext.NewCommandContext(term, newClient, newRESTClient)

		// when
		err := cmd.AddSpaceUsers(ctx, "testspace", "admin", []string{"alice", "bob"})

		// then
		require.EqualError(t, err, "client error")
		output := term.Output()
		assertSpaceBindings(t, fakeClient, []string{}, "") // no spacebindings expected
		assert.NotContains(t, output, "Are you sure that you want to add users to the above Space?")
		assert.NotContains(t, output, "SpaceBinding(s) successfully created")
		assert.NotContains(t, output, "cool-token")
	})
}

func initAddSpaceUsersTest(t *testing.T, murs ...*toolchainv1alpha1.MasterUserRecord) (clicontext.NewClientFunc, clicontext.NewRESTClientFunc, *test.FakeClient) {
	space := newSpace()
	nsTemplateTier := newNSTemplateTier("base")
	roles := make(map[string]toolchainv1alpha1.NSTemplateTierSpaceRole)
	roles["admin"] = toolchainv1alpha1.NSTemplateTierSpaceRole{
		TemplateRef: uuid.NewV4().String(),
	}
	roles["viewer"] = toolchainv1alpha1.NSTemplateTierSpaceRole{
		TemplateRef: uuid.NewV4().String(),
	}
	nsTemplateTier.Spec.SpaceRoles = roles
	objs := []runtime.Object{space, nsTemplateTier}
	for _, mur := range murs {
		objs = append(objs, mur)
	}
	newClient, newRESTClient, fakeClient := NewFakeClients(t, objs...)
	return newClient, newRESTClient, fakeClient
}

func assertSpaceBindings(t *testing.T, fakeClient *test.FakeClient, expectedMurs []string, expectedRole string) {

	// list all SpaceBindings for the given space
	allSpaceBindings := &toolchainv1alpha1.SpaceBindingList{}
	err := fakeClient.List(context.TODO(), allSpaceBindings, runtimeclient.InNamespace(test.HostOperatorNs), runtimeclient.MatchingLabels{
		toolchainv1alpha1.SpaceBindingSpaceLabelKey: "testspace",
	})
	require.NoError(t, err)

	// verify the expected number of SpaceBindings were created
	assert.Len(t, allSpaceBindings.Items, len(expectedMurs))

	// check that all expected MURs have SpaceBindings
	var checked int
	for _, expectedMur := range expectedMurs {
		for _, sb := range allSpaceBindings.Items {
			if sb.Labels[toolchainv1alpha1.SpaceBindingMasterUserRecordLabelKey] == expectedMur {
				require.Equal(t, "testcreator", sb.Labels[toolchainv1alpha1.SpaceCreatorLabelKey])
				require.Equal(t, expectedMur, sb.Labels[toolchainv1alpha1.SpaceBindingMasterUserRecordLabelKey])
				require.Equal(t, expectedMur, sb.Spec.MasterUserRecord)
				require.Equal(t, "testspace", sb.Spec.Space)
				require.Equal(t, expectedRole, sb.Spec.SpaceRole)
				checked++
			}
		}
	}
	if checked != len(expectedMurs) {
		require.Fail(t, "some expected murs were not found")
	}
}

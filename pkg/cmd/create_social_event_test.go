package cmd_test

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/kubesaw/ksctl/pkg/cmd"
	clicontext "github.com/kubesaw/ksctl/pkg/context"
	"github.com/kubesaw/ksctl/pkg/ioutils"
	. "github.com/kubesaw/ksctl/pkg/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func TestCreateSocialEvent(t *testing.T) {

	spaceTier := newNSTemplateTier("base")
	userTier := newUserTier("deactivate30")
	SetFileConfig(t, Host(), Member())

	t.Run("success", func(t *testing.T) {

		t.Run("1-day event without description", func(t *testing.T) {
			// given
			newClient, fakeClient := NewFakeClients(t, userTier, spaceTier)
			buffy := bytes.NewBuffer(nil)
			term := ioutils.NewTerminal(buffy, buffy)
			ctx := clicontext.NewCommandContext(term, newClient)
			startDate := "2022-06-21" // summer 🏝
			endDate := "2022-06-21"   // ends same day
			description := ""
			maxAttendees := 10
			// when
			err := cmd.CreateSocialEvent(ctx, startDate, endDate, description, userTier.Name, spaceTier.Name, maxAttendees, "member-1")

			// then
			require.NoError(t, err)
			assert.Contains(t, buffy.String(), "Social Event successfully created")
			// verify that the SocialEvent was created
			ses := &toolchainv1alpha1.SocialEventList{}
			err = fakeClient.List(context.TODO(), ses, runtimeclient.InNamespace(test.HostOperatorNs))
			require.NoError(t, err)
			require.Len(t, ses.Items, 1)
			event := ses.Items[0]
			assert.Equal(t, startDate, event.Spec.StartTime.Format("2006-01-02"))
			assert.Equal(t, endDate, event.Spec.EndTime.Format("2006-01-02"))
			assert.Equal(t, userTier.Name, event.Spec.UserTier)
			assert.Equal(t, spaceTier.Name, event.Spec.SpaceTier)
			assert.Equal(t, maxAttendees, event.Spec.MaxAttendees)
			assert.Equal(t, "member-cool-server.com", event.Spec.TargetCluster)
			assert.Empty(t, event.Spec.Description)
		})

		t.Run("2-day event", func(t *testing.T) {
			// given
			newClient, fakeClient := NewFakeClients(t, userTier, spaceTier)
			buffy := bytes.NewBuffer(nil)
			term := ioutils.NewTerminal(buffy, buffy)
			ctx := clicontext.NewCommandContext(term, newClient)
			startDate := "2022-06-21" // summer 🏝
			endDate := "2022-06-22"
			description := "summer workshop"
			maxAttendees := 10
			// when
			err := cmd.CreateSocialEvent(ctx, startDate, endDate, description, userTier.Name, spaceTier.Name, maxAttendees, "")

			// then
			require.NoError(t, err)
			// verify that the SocialEvent was created
			ses := &toolchainv1alpha1.SocialEventList{}
			err = fakeClient.List(context.TODO(), ses, runtimeclient.InNamespace(test.HostOperatorNs))
			require.NoError(t, err)
			require.Len(t, ses.Items, 1)
			event := ses.Items[0]
			assert.Equal(t, description, event.Spec.Description)
			assert.Empty(t, event.Spec.TargetCluster)
			// no need to re-verify other fields, test above already took care of them
		})
	})

	t.Run("failures", func(t *testing.T) {

		t.Run("invalid start date", func(t *testing.T) {
			// given
			newClient, _ := NewFakeClients(t, userTier, spaceTier)
			buffy := bytes.NewBuffer(nil)
			term := ioutils.NewTerminal(buffy, buffy)
			ctx := clicontext.NewCommandContext(term, newClient)
			startDate := "2022-06-xx" // invalid!
			endDate := "2022-06-22"
			description := "summer workshop"
			maxAttendees := 10
			// when
			err := cmd.CreateSocialEvent(ctx, startDate, endDate, description, userTier.Name, spaceTier.Name, maxAttendees, "")

			// then
			require.Error(t, err)
			assert.Contains(t, err.Error(), "start date is invalid: '2022-06-xx' (expected YYYY-MM-DD)")
		})

		t.Run("invalid end date", func(t *testing.T) {
			// given
			newClient, _ := NewFakeClients(t, userTier, spaceTier)
			buffy := bytes.NewBuffer(nil)
			term := ioutils.NewTerminal(buffy, buffy)
			ctx := clicontext.NewCommandContext(term, newClient)
			startDate := "2022-06-21"
			endDate := "2022-06-32" // invalid value!
			description := "summer workshop"
			maxAttendees := 10
			// when
			err := cmd.CreateSocialEvent(ctx, startDate, endDate, description, userTier.Name, spaceTier.Name, maxAttendees, "")

			// then
			require.Error(t, err)
			assert.Contains(t, err.Error(), "end date is invalid: '2022-06-32' (expected YYYY-MM-DD)")
		})

		t.Run("end date before start date", func(t *testing.T) {
			// given
			newClient, _ := NewFakeClients(t, userTier, spaceTier)
			buffy := bytes.NewBuffer(nil)
			term := ioutils.NewTerminal(buffy, buffy)
			ctx := clicontext.NewCommandContext(term, newClient)
			startDate := "2022-06-21"
			endDate := "2022-06-11" // before start date!
			description := "summer workshop"
			maxAttendees := 10
			// when
			err := cmd.CreateSocialEvent(ctx, startDate, endDate, description, userTier.Name, spaceTier.Name, maxAttendees, "")

			// then
			require.Error(t, err)
			assert.Contains(t, err.Error(), "end date is not after start date")
		})

		t.Run("usertier does not exist", func(t *testing.T) {
			// given
			newClient, _ := NewFakeClients(t, spaceTier) // no user tier
			buffy := bytes.NewBuffer(nil)
			term := ioutils.NewTerminal(buffy, buffy)
			ctx := clicontext.NewCommandContext(term, newClient)
			startDate := "2022-06-21"
			endDate := "2022-06-22"
			description := "summer workshop"
			maxAttendees := 10
			// when
			err := cmd.CreateSocialEvent(ctx, startDate, endDate, description, userTier.Name, spaceTier.Name, maxAttendees, "")

			// then
			require.Error(t, err)
			assert.Contains(t, err.Error(), fmt.Sprintf("UserTier '%s' does not exist", userTier.Name))
		})

		t.Run("nstemplatetier does not exist", func(t *testing.T) {
			// given
			newClient, _ := NewFakeClients(t, userTier) // no space tier
			buffy := bytes.NewBuffer(nil)
			term := ioutils.NewTerminal(buffy, buffy)
			ctx := clicontext.NewCommandContext(term, newClient)
			startDate := "2022-06-21"
			endDate := "2022-06-22"
			description := "summer workshop"
			maxAttendees := 10
			// when
			err := cmd.CreateSocialEvent(ctx, startDate, endDate, description, userTier.Name, spaceTier.Name, maxAttendees, "")

			// then
			require.Error(t, err)
			assert.Contains(t, err.Error(), fmt.Sprintf("NSTemplateTier '%s' does not exist", spaceTier.Name))
		})

		t.Run("invalid target cluster", func(t *testing.T) {
			// given
			newClient, _ := NewFakeClients(t, userTier, spaceTier)
			buffy := bytes.NewBuffer(nil)
			term := ioutils.NewTerminal(buffy, buffy)
			ctx := clicontext.NewCommandContext(term, newClient)
			startDate := "2022-06-21" // summer 🏝
			endDate := "2022-06-21"   // ends same day
			maxAttendees := 10

			t.Run("unknown target cluster", func(t *testing.T) {
				// when
				err := cmd.CreateSocialEvent(ctx, startDate, endDate, "", userTier.Name, spaceTier.Name, maxAttendees, "unknown-cluster")

				// then
				require.EqualError(t, err, "the provided cluster-name 'unknown-cluster' is not present in your ksctl.yaml file. The available cluster names are\n------------------------\nhost\nmember-1\n------------------------")
			})

			t.Run("not a member target cluster", func(t *testing.T) {
				// when
				err := cmd.CreateSocialEvent(ctx, startDate, endDate, "", userTier.Name, spaceTier.Name, maxAttendees, "host")

				// then
				require.EqualError(t, err, "expected target cluster to have clusterType 'member', actual: 'host'")
			})
		})
	})
}

package cmd_test

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	testspace "github.com/codeready-toolchain/toolchain-common/pkg/test/space"
	testusersignup "github.com/codeready-toolchain/toolchain-common/pkg/test/usersignup"
	"github.com/kubesaw/ksctl/pkg/cmd"
	clicontext "github.com/kubesaw/ksctl/pkg/context"
	"github.com/kubesaw/ksctl/pkg/ioutils"
	. "github.com/kubesaw/ksctl/pkg/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func TestRetarget(t *testing.T) {
	userSignup := testusersignup.NewUserSignup(testusersignup.WithName("john"))

	t.Run("retarget success", func(t *testing.T) {
		// given
		space := testspace.NewSpace(test.HostOperatorNs, "john-dev", testspace.WithCreatorLabel("john"))
		newClient, fakeClient := prepareRetargetSpace(t, space, userSignup)
		buffy := bytes.NewBuffer(nil)
		term := ioutils.NewTerminal(buffy, buffy, ioutils.WithDefaultConfirm(true))
		ctx := clicontext.NewCommandContext(term, newClient)

		// when
		err := cmd.Retarget(ctx, space.Name, "member2")

		// then
		require.NoError(t, err)
		testspace.AssertThatSpace(t, space.Namespace, space.Name, fakeClient).HasSpecTargetCluster("member-m2.devcluster.openshift.com")
		assert.Contains(t, buffy.String(), "!!!  DANGER ZONE  !!!")
		// assert.Contains(t, buffy.String(), fmt.Sprintf("Are you sure that you want to retarget the Space '%s' owned (created) by UserSignup '%s' to cluster 'member2'?", space.Name, userSignup.Name))
		assert.Contains(t, buffy.String(), "Space to be retargeted")
		assert.Contains(t, buffy.String(), fmt.Sprintf("Owned (created) by UserSignup '%s' with spec", userSignup.Name))
		assert.Contains(t, buffy.String(), "Space has been patched to the 'member2' target cluster")
		assert.Contains(t, buffy.String(), "Space has been retargeted to the 'member2' cluster")
		assert.NotContains(t, buffy.String(), "cool-token")
	})

	t.Run("retarget fail", func(t *testing.T) {
		t.Run("no space found", func(t *testing.T) {
			// given
			newClient, _ := prepareRetargetSpace(t) // no usersignup created
			buffy := bytes.NewBuffer(nil)
			term := ioutils.NewTerminal(buffy, buffy, ioutils.WithDefaultConfirm(true))
			ctx := clicontext.NewCommandContext(term, newClient)

			// when
			err := cmd.Retarget(ctx, "space-that-doesnt-exist", "member1")

			// then
			require.EqualError(t, err, `spaces.toolchain.dev.openshift.com "space-that-doesnt-exist" not found`)
		})

		t.Run("space already targeted to the provided target cluster", func(t *testing.T) {
			// given
			space := testspace.NewSpace(test.HostOperatorNs, "john-dev", testspace.WithCreatorLabel("john"), testspace.WithSpecTargetCluster("member-m2.devcluster.openshift.com"))
			newClient, _ := prepareRetargetSpace(t, space, userSignup)
			buffy := bytes.NewBuffer(nil)
			term := ioutils.NewTerminal(buffy, buffy, ioutils.WithDefaultConfirm(true))
			ctx := clicontext.NewCommandContext(term, newClient)

			// when
			err := cmd.Retarget(ctx, space.Name, "member2")

			// then
			require.EqualError(t, err, fmt.Sprintf(`the Space '%s' is already targeted to cluster '%s'`, space.Name, "member2"))
		})

		t.Run("failed to get member cluster config", func(t *testing.T) {
			// given
			space := testspace.NewSpace(test.HostOperatorNs, "john-dev")
			newClient, _ := prepareRetargetSpace(t, space)
			buffy := bytes.NewBuffer(nil)
			term := ioutils.NewTerminal(buffy, buffy, ioutils.WithDefaultConfirm(true))
			ctx := clicontext.NewCommandContext(term, newClient)

			// when
			err := cmd.Retarget(ctx, space.Name, "non-existent-member") // bad member name

			// then
			require.Error(t, err)
			assert.Contains(t, err.Error(), "the provided cluster-name 'non-existent-member' is not present in your ksctl.yaml file")
		})

		t.Run("setting target cluster failed", func(t *testing.T) {
			// given
			space := testspace.NewSpace(test.HostOperatorNs, "john-dev", testspace.WithCreatorLabel("john"))
			newClient, fakeClient := prepareRetargetSpace(t, space, userSignup)
			fakeClient.MockPatch = func(ctx context.Context, obj runtimeclient.Object, patch runtimeclient.Patch, opts ...runtimeclient.PatchOption) error {
				if testSignup, ok := obj.(*toolchainv1alpha1.Space); ok {
					if testSignup.Spec.TargetCluster != "" {
						return fmt.Errorf("fail target cluster")
					}
				}
				return fakeClient.Client.Patch(ctx, obj, patch, opts...)
			}
			buffy := bytes.NewBuffer(nil)
			term := ioutils.NewTerminal(buffy, buffy, ioutils.WithDefaultConfirm(true))
			ctx := clicontext.NewCommandContext(term, newClient)

			// when
			err := cmd.Retarget(ctx, space.Name, "member2")

			// then
			require.Error(t, err)
			assert.Contains(t, err.Error(), fmt.Sprintf("failed to retarget Space '%s'", space.Name))
		})

		t.Run("space without owner label", func(t *testing.T) {
			// given
			space := testspace.NewSpace(test.HostOperatorNs, "john-dev")
			newClient, _ := prepareRetargetSpace(t, space, userSignup)
			buffy := bytes.NewBuffer(nil)
			term := ioutils.NewTerminal(buffy, buffy, ioutils.WithDefaultConfirm(true))
			ctx := clicontext.NewCommandContext(term, newClient)

			// when
			err := cmd.Retarget(ctx, space.Name, "member2")

			// then
			require.Error(t, err)
			assert.Contains(t, err.Error(), "spaces without the creator label are not supported")
		})

		t.Run("usersignup not found", func(t *testing.T) {
			// given
			space := testspace.NewSpace(test.HostOperatorNs, "john-dev", testspace.WithCreatorLabel("john"))
			newClient, _ := prepareRetargetSpace(t, space)
			buffy := bytes.NewBuffer(nil)
			term := ioutils.NewTerminal(buffy, buffy, ioutils.WithDefaultConfirm(true))
			ctx := clicontext.NewCommandContext(term, newClient)

			// when
			err := cmd.Retarget(ctx, space.Name, "member2")

			// then
			require.Error(t, err)
			assert.Contains(t, err.Error(), `usersignups.toolchain.dev.openshift.com "john" not found`)
		})
	})

	t.Run("user responds no", func(t *testing.T) {
		// given
		space := testspace.NewSpace(test.HostOperatorNs, "john-dev", testspace.WithCreatorLabel("john"), testspace.WithSpecTargetCluster("member-m1.devcluster.openshift.com"))
		newClient, fakeClient := prepareRetargetSpace(t, space, userSignup)
		buffy := bytes.NewBuffer(nil)
		term := ioutils.NewTerminal(buffy, buffy, ioutils.WithDefaultConfirm(false))
		ctx := clicontext.NewCommandContext(term, newClient)

		// when
		err := cmd.Retarget(ctx, space.Name, "member2")

		// then
		require.NoError(t, err)
		testspace.AssertThatSpace(t, space.Namespace, space.Name, fakeClient).HasSpecTargetCluster("member-m1.devcluster.openshift.com")
		assert.Contains(t, buffy.String(), "!!!  DANGER ZONE  !!!")
		// assert.Contains(t, buffy.String(), fmt.Sprintf("Are you sure that you want to retarget the Space '%s' owned (created) by UserSignup '%s' to cluster 'member2'?", space.Name, userSignup.Name))
		assert.Contains(t, buffy.String(), "Space to be retargeted")
		assert.Contains(t, buffy.String(), fmt.Sprintf("Owned (created) by UserSignup '%s' with spec", userSignup.Name))
		assert.NotContains(t, buffy.String(), "Space has been patched to the 'member2' target cluster")
		assert.NotContains(t, buffy.String(), "Space has been retargeted to the 'member2' cluster")
		assert.NotContains(t, buffy.String(), "cool-token")
	})
}

func prepareRetargetSpace(t *testing.T, initObjs ...runtime.Object) (clicontext.NewClientFunc, *test.FakeClient) {
	newClient, fakeClient := NewFakeClients(t, initObjs...)
	SetFileConfig(t,
		Host(),
		Member(ClusterName("member1"), ServerName("m1.devcluster.openshift.com")),
		Member(ClusterName("member2"), ServerName("m2.devcluster.openshift.com")))

	return newClient, fakeClient
}

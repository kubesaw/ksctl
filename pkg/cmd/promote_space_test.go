package cmd_test

import (
	"bytes"
	"context"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/kubesaw/ksctl/pkg/cmd"
	clicontext "github.com/kubesaw/ksctl/pkg/context"
	"github.com/kubesaw/ksctl/pkg/ioutils"
	. "github.com/kubesaw/ksctl/pkg/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPromoteSpaceCmdWhenAnswerIsY(t *testing.T) {
	// given
	space := newSpace()
	newClient, fakeClient := NewFakeClients(t, space, newNSTemplateTier("advanced"))
	SetFileConfig(t, Host())
	buffy := bytes.NewBuffer(nil)
	term := ioutils.NewTerminal(buffy, buffy, ioutils.WithDefaultConfirm(true))
	ctx := clicontext.NewCommandContext(term, newClient)

	// when
	err := cmd.PromoteSpace(ctx, space.Name, "advanced")

	// then
	require.NoError(t, err)
	space.Spec.TierName = "advanced" // space should be changed to advanced tier
	assertSpaceSpec(t, fakeClient, space)
	assert.Contains(t, buffy.String(), "Successfully promoted Space")
	assert.NotContains(t, buffy.String(), "cool-token")
}

func TestPromoteSpaceCmdWhenAnswerIsN(t *testing.T) {
	// given
	space := newSpace()
	newClient, fakeClient := NewFakeClients(t, space, newNSTemplateTier("advanced"))
	SetFileConfig(t, Host())
	buffy := bytes.NewBuffer(nil)
	term := ioutils.NewTerminal(buffy, buffy, ioutils.WithDefaultConfirm(false))
	ctx := clicontext.NewCommandContext(term, newClient)

	// when
	err := cmd.PromoteSpace(ctx, space.Name, "advanced")

	// then
	require.NoError(t, err)
	assertSpaceSpec(t, fakeClient, space) // space should be unchanged
	// assert.Contains(t, buffy.String(), "promote the Space 'testspace' to the 'advanced' tier?")
	assert.NotContains(t, buffy.String(), "Successfully promoted Space")
	assert.NotContains(t, buffy.String(), "cool-token")
}

func TestPromoteSpaceCmdWhenSpaceNotFound(t *testing.T) {
	// given
	space := newSpace()
	newClient, fakeClient := NewFakeClients(t, space, newNSTemplateTier("advanced"))
	SetFileConfig(t, Host())
	buffy := bytes.NewBuffer(nil)
	term := ioutils.NewTerminal(buffy, buffy, ioutils.WithDefaultConfirm(true))
	ctx := clicontext.NewCommandContext(term, newClient)

	// when
	err := cmd.PromoteSpace(ctx, "another", "advanced") // attempt to promote a space that does not exist

	// then
	require.EqualError(t, err, "spaces.toolchain.dev.openshift.com \"another\" not found")
	assertSpaceSpec(t, fakeClient, space) // unrelated space should be unchanged
	// assert.NotContains(t, buffy.String(), "promote the Space 'another' to the 'advanced' tier?")
	assert.NotContains(t, buffy.String(), "Successfully promoted Space")
	assert.NotContains(t, buffy.String(), "cool-token")
}

func TestPromoteSpaceCmdWhenNSTemplateTierNotFound(t *testing.T) {
	// given
	space := newSpace()
	newClient, fakeClient := NewFakeClients(t, space)
	SetFileConfig(t, Host())
	buffy := bytes.NewBuffer(nil)
	term := ioutils.NewTerminal(buffy, buffy)
	ctx := clicontext.NewCommandContext(term, newClient)

	// when
	err := cmd.PromoteSpace(ctx, space.Name, "advanced")

	// then
	require.EqualError(t, err, "nstemplatetiers.toolchain.dev.openshift.com \"advanced\" not found")
	assertSpaceSpec(t, fakeClient, space) // space should be unchanged
	assert.NotContains(t, buffy.String(), "promote the Space 'another' to the 'advanced' tier?")
	assert.NotContains(t, buffy.String(), "Successfully promoted Space")
	assert.NotContains(t, buffy.String(), "cool-token")
}

func newNSTemplateTier(name string) *toolchainv1alpha1.NSTemplateTier {
	nsTemplateTier := &toolchainv1alpha1.NSTemplateTier{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: test.HostOperatorNs,
		},
		Spec: toolchainv1alpha1.NSTemplateTierSpec{},
	}
	return nsTemplateTier
}

func newSpace() *toolchainv1alpha1.Space {
	space := &toolchainv1alpha1.Space{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testspace",
			Namespace: test.HostOperatorNs,
			Labels: map[string]string{
				toolchainv1alpha1.SpaceCreatorLabelKey: "testcreator",
			},
		},
		Spec: toolchainv1alpha1.SpaceSpec{
			TierName: "base",
		},
	}
	return space
}

func assertSpaceSpec(t *testing.T, fakeClient *test.FakeClient, expectedSpace *toolchainv1alpha1.Space) {
	updatedSpace := &toolchainv1alpha1.Space{}
	err := fakeClient.Get(context.TODO(), test.NamespacedName(expectedSpace.Namespace, expectedSpace.Name), updatedSpace)
	require.NoError(t, err)
	assert.Equal(t, expectedSpace.Spec, updatedSpace.Spec)
}

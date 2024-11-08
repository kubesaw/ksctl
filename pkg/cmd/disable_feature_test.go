package cmd_test

import (
	"bytes"
	"fmt"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/kubesaw/ksctl/pkg/cmd"
	clicontext "github.com/kubesaw/ksctl/pkg/context"
	"github.com/kubesaw/ksctl/pkg/ioutils"
	. "github.com/kubesaw/ksctl/pkg/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDisableFeatureCmd(t *testing.T) {
	// given
	SetFileConfig(t, Host())

	var combinations = []struct {
		alreadyEnabled string
		afterDisable   map[string]string
	}{
		{
			alreadyEnabled: "feature-x",
			afterDisable:   nil,
		},
		{
			alreadyEnabled: "feature-x,feature0",
			afterDisable: map[string]string{
				toolchainv1alpha1.FeatureToggleNameAnnotationKey: "feature0",
			},
		},
		{
			alreadyEnabled: "feature1,feature2,feature-x,feature3",
			afterDisable: map[string]string{
				toolchainv1alpha1.FeatureToggleNameAnnotationKey: "feature1,feature2,feature3",
			},
		},
	}

	for _, data := range combinations {
		t.Run("with the already enabled features: "+data.alreadyEnabled, func(t *testing.T) {
			// given
			space := newSpace()
			if data.alreadyEnabled != "" {
				space.Annotations = map[string]string{
					toolchainv1alpha1.FeatureToggleNameAnnotationKey: data.alreadyEnabled,
				}
			}

			for _, answer := range []bool{true, false} {

				t.Run(fmt.Sprintf("when answer is %t", answer), func(t *testing.T) {
					// given
					newClient, fakeClient := NewFakeClients(t, space)
					buffy := bytes.NewBuffer(nil)
					term := ioutils.NewTerminal(buffy, buffy, ioutils.WithDefaultConfirm(answer))
					ctx := clicontext.NewCommandContext(term, newClient)

					// when
					err := cmd.DisableFeature(ctx, space.Name, "feature-x")

					// then
					require.NoError(t, err)

					// assert.Contains(t, buffy.String(), fmt.Sprintf("disable the feature toggle 'feature-x' for the Space 'testspace'? The enabled feature toggles are '%s'.", data.alreadyEnabled))
					assert.NotContains(t, buffy.String(), "cool-token")
					expectedSpace := newSpace()

					if answer {
						expectedSpace.Annotations = data.afterDisable
						assert.Contains(t, buffy.String(), "Successfully disabled the 'feature-x' feature for the 'testspace' Space")
					} else {
						expectedSpace.Annotations = space.Annotations
						assert.NotContains(t, buffy.String(), "Successfully disabled the 'feature-x' feature for the 'testspace' Space")
					}
					assertSpaceAnnotations(t, fakeClient, expectedSpace)

				})
			}
		})
	}
}

func TestDisableFeatureCmdWhenFeatureIsNotEnabled(t *testing.T) {
	// given
	SetFileConfig(t, Host())

	for _, alreadyEnabled := range []string{"", "feature0", "feature1,feature2,feature3"} {
		t.Run("with the already enabled features: "+alreadyEnabled, func(t *testing.T) {
			// given
			space := newSpace()
			if alreadyEnabled != "" {
				space.Annotations = map[string]string{
					toolchainv1alpha1.FeatureToggleNameAnnotationKey: alreadyEnabled,
				}
			}
			// given
			newClient, fakeClient := NewFakeClients(t, space)
			buffy := bytes.NewBuffer(nil)
			term := ioutils.NewTerminal(buffy, buffy)
			ctx := clicontext.NewCommandContext(term, newClient)

			// when
			err := cmd.DisableFeature(ctx, space.Name, "feature-x")

			// then
			require.NoError(t, err)
			assertSpaceAnnotations(t, fakeClient, space) // no change

			assert.Contains(t, buffy.String(), "Nothing to do: the 'feature-x' feature is not enabled in the 'testspace' Space")
			assert.NotContains(t, buffy.String(), "disable the feature toggle 'feature-x' for the Space 'testspace'?")
			assert.NotContains(t, buffy.String(), "Successfully disabled feature toggle for the Space")
			assert.NotContains(t, buffy.String(), "cool-token")

		})
	}
}

func TestDisableFeatureCmdWhenSpaceNotFound(t *testing.T) {
	// given
	space := newSpace()
	newClient, fakeClient := NewFakeClients(t, space)
	SetFileConfig(t, Host())
	buffy := bytes.NewBuffer(nil)
	term := ioutils.NewTerminal(buffy, buffy)
	ctx := clicontext.NewCommandContext(term, newClient)

	// when
	err := cmd.DisableFeature(ctx, "another", "feature-x")

	// then
	require.EqualError(t, err, "spaces.toolchain.dev.openshift.com \"another\" not found")
	assertSpaceAnnotations(t, fakeClient, space) // unrelated space should be unchanged
	assert.NotContains(t, buffy.String(), "disable the feature toggle 'feature-x' for the Space 'testspace'?")
	assert.NotContains(t, buffy.String(), "Successfully disabled feature toggle for the Space")
	assert.NotContains(t, buffy.String(), "cool-token")
}

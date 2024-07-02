package cmd_test

import (
	"fmt"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/kubesaw/ksctl/pkg/cmd"
	clicontext "github.com/kubesaw/ksctl/pkg/context"
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

			for _, answer := range []string{"Y", "n"} {

				t.Run("when answer is "+answer, func(t *testing.T) {
					// given
					newClient, fakeClient := NewFakeClients(t, space)
					term := NewFakeTerminalWithResponse(answer)
					ctx := clicontext.NewCommandContext(term, newClient)

					// when
					err := cmd.DisableFeature(ctx, space.Name, "feature-x")

					// then
					require.NoError(t, err)

					output := term.Output()
					assert.Contains(t, output, fmt.Sprintf("disable the feature toggle 'feature-x' for the Space 'testspace'? The enabled feature toggles are '%s'.", data.alreadyEnabled))
					assert.NotContains(t, output, "cool-token")
					expectedSpace := newSpace()

					if answer == "Y" {
						expectedSpace.Annotations = data.afterDisable
						assert.Contains(t, output, "Successfully disabled feature toggle for the Space")

					} else {
						expectedSpace.Annotations = space.Annotations
						assert.NotContains(t, output, "Successfully disabled feature toggle for the Space")
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
			term := NewFakeTerminalWithResponse("Y")
			ctx := clicontext.NewCommandContext(term, newClient)

			// when
			err := cmd.DisableFeature(ctx, space.Name, "feature-x")

			// then
			require.NoError(t, err)
			assertSpaceAnnotations(t, fakeClient, space) // no change

			output := term.Output()
			assert.Contains(t, output, "The Space doesn't have the feature toggle enabled. There is nothing to do.")
			assert.NotContains(t, output, "disable the feature toggle 'feature-x' for the Space 'testspace'?")
			assert.NotContains(t, output, "Successfully disabled feature toggle for the Space")
			assert.NotContains(t, output, "cool-token")

		})
	}
}

func TestDisableFeatureCmdWhenSpaceNotFound(t *testing.T) {
	// given
	space := newSpace()
	newClient, fakeClient := NewFakeClients(t, space)
	SetFileConfig(t, Host())
	term := NewFakeTerminalWithResponse("Y")
	ctx := clicontext.NewCommandContext(term, newClient)

	// when
	err := cmd.DisableFeature(ctx, "another", "feature-x")

	// then
	require.EqualError(t, err, "spaces.toolchain.dev.openshift.com \"another\" not found")
	assertSpaceAnnotations(t, fakeClient, space) // unrelated space should be unchanged
	output := term.Output()
	assert.NotContains(t, output, "disable the feature toggle 'feature-x' for the Space 'testspace'?")
	assert.NotContains(t, output, "Successfully disabled feature toggle for the Space")
	assert.NotContains(t, output, "cool-token")
}

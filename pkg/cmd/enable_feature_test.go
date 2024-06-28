package cmd_test

import (
	"context"
	"fmt"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/kubesaw/ksctl/pkg/cmd"
	clicontext "github.com/kubesaw/ksctl/pkg/context"
	. "github.com/kubesaw/ksctl/pkg/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnableFeatureCmd(t *testing.T) {
	// given
	SetFileConfig(t, Host())

	var combinations = []struct {
		alreadyEnabled string
		afterEnable    map[string]string
	}{
		{
			alreadyEnabled: "",
			afterEnable: map[string]string{
				toolchainv1alpha1.FeatureToggleNameAnnotationKey: "feature-x",
			},
		},
		{
			alreadyEnabled: "feature0",
			afterEnable: map[string]string{
				toolchainv1alpha1.FeatureToggleNameAnnotationKey: "feature0,feature-x",
			},
		},
		{
			alreadyEnabled: "feature1,feature2,feature3",
			afterEnable: map[string]string{
				toolchainv1alpha1.FeatureToggleNameAnnotationKey: "feature1,feature2,feature3,feature-x",
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
					err := cmd.EnableFeature(ctx, space.Name, "feature-x")

					// then
					require.NoError(t, err)

					output := term.Output()
					assert.Contains(t, output, fmt.Sprintf("enable the feature toggle 'feature-x' for the Space 'testspace'? The already enabled feature toggles are '%s'.", data.alreadyEnabled))
					assert.NotContains(t, output, "cool-token")
					expectedSpace := newSpace()

					if answer == "Y" {
						expectedSpace.Annotations = data.afterEnable
						assert.Contains(t, output, "Successfully enabled feature toggle for the Space")

					} else {
						expectedSpace.Annotations = space.Annotations
						assert.NotContains(t, output, "Successfully enabled feature toggle for the Space")
					}
					assertSpaceAnnotations(t, fakeClient, expectedSpace)

				})
			}
		})
	}
}

func TestEnableFeatureCmdWhenFeatureIsAlreadyEnabled(t *testing.T) {
	// given
	SetFileConfig(t, Host())

	for _, alreadyEnabled := range []string{"feature-x", "feature-x,feature0", "feature1,feature2,feature-x,feature3"} {
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
			err := cmd.EnableFeature(ctx, space.Name, "feature-x")

			// then
			require.NoError(t, err)
			assertSpaceAnnotations(t, fakeClient, space) // no change

			output := term.Output()
			assert.Contains(t, output, "The space has the feature toggle already enabled. There is nothing to do.")
			assert.NotContains(t, output, "enable the feature toggle 'feature-x' for the Space 'testspace'?")
			assert.NotContains(t, output, "Successfully enabled feature toggle for the Space")
			assert.NotContains(t, output, "cool-token")

		})
	}
}

func TestEnableFeatureCmdWhenSpaceNotFound(t *testing.T) {
	// given
	space := newSpace()
	newClient, fakeClient := NewFakeClients(t, space)
	SetFileConfig(t, Host())
	term := NewFakeTerminalWithResponse("Y")
	ctx := clicontext.NewCommandContext(term, newClient)

	// when
	err := cmd.EnableFeature(ctx, "another", "feature-x")

	// then
	require.EqualError(t, err, "spaces.toolchain.dev.openshift.com \"another\" not found")
	assertSpaceAnnotations(t, fakeClient, space) // unrelated space should be unchanged
	output := term.Output()
	assert.NotContains(t, output, "enable the feature toggle 'feature-x' for the Space 'testspace'?")
	assert.NotContains(t, output, "Successfully enabled feature toggle for the Space")
	assert.NotContains(t, output, "cool-token")
}

func assertSpaceAnnotations(t *testing.T, fakeClient *test.FakeClient, expectedSpace *toolchainv1alpha1.Space) {
	updatedSpace := &toolchainv1alpha1.Space{}
	err := fakeClient.Get(context.TODO(), test.NamespacedName(expectedSpace.Namespace, expectedSpace.Name), updatedSpace)
	require.NoError(t, err)
	assert.Equal(t, expectedSpace.Annotations, updatedSpace.Annotations)
}

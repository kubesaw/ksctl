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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnableFeatureCmd(t *testing.T) {
	// given
	SetFileConfig(t, Host())
	config := configWithFeatures([]toolchainv1alpha1.FeatureToggle{
		{
			Name: "feature-x",
		},
	})

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

			for _, answer := range []bool{true, false} {

				t.Run(fmt.Sprintf("when answer is %t", answer), func(t *testing.T) {
					// given
					newClient, fakeClient := NewFakeClients(t, space, config)
					buffy := bytes.NewBuffer(nil)
					term := ioutils.NewTerminal(buffy, buffy, ioutils.WithDefaultConfirm(answer))
					ctx := clicontext.NewCommandContext(term, newClient)

					// when
					err := cmd.EnableFeature(ctx, space.Name, "feature-x")

					// then
					require.NoError(t, err)

					// assert.Contains(t, buffy.String(), fmt.Sprintf("enable the feature toggle 'feature-x' for the Space 'testspace'? The already enabled feature toggles are '%s'.", data.alreadyEnabled))
					assert.NotContains(t, buffy.String(), "cool-token")
					expectedSpace := newSpace()

					if answer {
						expectedSpace.Annotations = data.afterEnable
						assert.Contains(t, buffy.String(), "Successfully enabled the 'feature-x' feature for the 'testspace' Space")

					} else {
						expectedSpace.Annotations = space.Annotations
						assert.NotContains(t, buffy.String(), "Successfully enabled the 'feature-x' feature for the 'testspace' Space")
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
	config := configWithFeatures([]toolchainv1alpha1.FeatureToggle{
		{
			Name: "feature-x",
		},
	})

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
			newClient, fakeClient := NewFakeClients(t, space, config)
			buffy := bytes.NewBuffer(nil)
			term := ioutils.NewTerminal(buffy, buffy, ioutils.WithDefaultConfirm(true))
			ctx := clicontext.NewCommandContext(term, newClient)

			// when
			err := cmd.EnableFeature(ctx, space.Name, "feature-x")

			// then
			require.NoError(t, err)
			assertSpaceAnnotations(t, fakeClient, space) // no change

			// output := term.Output()
			assert.Contains(t, buffy.String(), "The space has the feature toggle already enabled. There is nothing to do.")
			// assert.NotContains(t, buffy.String(), "enable the feature toggle 'feature-x' for the Space 'testspace'?")
			assert.NotContains(t, buffy.String(), "Successfully enabled the 'feature-x' feature for the 'testspace' Space")
			assert.NotContains(t, buffy.String(), "cool-token")

		})
	}
}

func TestEnableFeatureCmdWhenFeatureIsNotSupported(t *testing.T) {
	// given
	SetFileConfig(t, Host())

	var combinations = []struct {
		nameList          string
		supportedFeatures []toolchainv1alpha1.FeatureToggle
	}{
		{
			nameList:          "",
			supportedFeatures: nil,
		},
		{
			nameList: "feature-0",
			supportedFeatures: []toolchainv1alpha1.FeatureToggle{
				{
					Name: "feature-0",
				},
			},
		},
		{
			nameList: "feature1\nfeature2\nfeature3",
			supportedFeatures: []toolchainv1alpha1.FeatureToggle{
				{
					Name: "feature1",
				},
				{
					Name: "feature2",
				},
				{
					Name: "feature3",
				},
			},
		},
	}

	for _, data := range combinations {
		t.Run("with the supported features: "+data.nameList, func(t *testing.T) {
			// given
			space := newSpace()
			config := configWithFeatures(data.supportedFeatures)
			// given
			newClient, fakeClient := NewFakeClients(t, space, config)
			buffy := bytes.NewBuffer(nil)
			term := ioutils.NewTerminal(buffy, buffy, ioutils.WithDefaultConfirm(true))
			ctx := clicontext.NewCommandContext(term, newClient)

			// when
			err := cmd.EnableFeature(ctx, space.Name, "feature-x")

			// then
			require.Error(t, err)
			// output := term.Output()
			if data.supportedFeatures == nil {
				require.EqualError(t, err, "the feature toggle is not supported - the list of supported toggles is empty")
			} else {
				require.EqualError(t, err, "the feature toggle is not supported")
				assert.Contains(t, buffy.String(), "The feature toggle 'feature-x' is not listed as a supported feature toggle in ToolchainConfig CR.")
				assert.Contains(t, buffy.String(), "The supported feature toggles are:")
				assert.Contains(t, buffy.String(), data.nameList)
			}

			assert.NotContains(t, buffy.String(), "Successfully enabled the 'feature-x' feature for the 'testspace' Space")
			assert.NotContains(t, buffy.String(), "cool-token")
			assertSpaceAnnotations(t, fakeClient, space) // no change
		})
	}
}

func TestEnableFeatureCmdWhenSpaceNotFound(t *testing.T) {
	// given
	config := configWithFeatures([]toolchainv1alpha1.FeatureToggle{
		{
			Name: "feature-x",
		},
	})
	space := newSpace()
	newClient, fakeClient := NewFakeClients(t, space, config)
	SetFileConfig(t, Host())
	buffy := bytes.NewBuffer(nil)
	term := ioutils.NewTerminal(buffy, buffy, ioutils.WithDefaultConfirm(true))
	ctx := clicontext.NewCommandContext(term, newClient)

	// when
	err := cmd.EnableFeature(ctx, "another", "feature-x")

	// then
	require.EqualError(t, err, "spaces.toolchain.dev.openshift.com \"another\" not found")
	assertSpaceAnnotations(t, fakeClient, space) // unrelated space should be unchanged
	// output := term.Output()
	// assert.NotContains(t, buffy.String(), "enable the feature toggle 'feature-x' for the Space 'testspace'?")
	assert.NotContains(t, buffy.String(), "Successfully enabled the 'feature-x' feature for the 'testspace' Space")
	assert.NotContains(t, buffy.String(), "cool-token")
}

func TestEnableFeatureCmdWhenConfigNotFound(t *testing.T) {
	// given
	space := newSpace()
	newClient, fakeClient := NewFakeClients(t, space)
	SetFileConfig(t, Host())
	buffy := bytes.NewBuffer(nil)
	term := ioutils.NewTerminal(buffy, buffy, ioutils.WithDefaultConfirm(true))
	ctx := clicontext.NewCommandContext(term, newClient)

	// when
	err := cmd.EnableFeature(ctx, space.Name, "feature-x")

	// then
	require.EqualError(t, err, "unable to get ToolchainConfig: toolchainconfigs.toolchain.dev.openshift.com \"config\" not found")
	assertSpaceAnnotations(t, fakeClient, space) // no change
}

func assertSpaceAnnotations(t *testing.T, fakeClient *test.FakeClient, expectedSpace *toolchainv1alpha1.Space) {
	updatedSpace := &toolchainv1alpha1.Space{}
	err := fakeClient.Get(context.TODO(), test.NamespacedName(expectedSpace.Namespace, expectedSpace.Name), updatedSpace)
	require.NoError(t, err)
	assert.Equal(t, expectedSpace.Annotations, updatedSpace.Annotations)
}

func configWithFeatures(toggles []toolchainv1alpha1.FeatureToggle) *toolchainv1alpha1.ToolchainConfig {
	toolchainConfig := &toolchainv1alpha1.ToolchainConfig{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: test.HostOperatorNs,
			Name:      "config",
		},
	}
	toolchainConfig.Spec.Host.Tiers.FeatureToggles = toggles
	return toolchainConfig
}

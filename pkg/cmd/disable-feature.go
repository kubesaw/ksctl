package cmd

import (
	"strings"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/kubesaw/ksctl/pkg/client"
	clicontext "github.com/kubesaw/ksctl/pkg/context"
	"github.com/kubesaw/ksctl/pkg/ioutils"
	"k8s.io/utils/strings/slices"

	"github.com/spf13/cobra"
)

func NewDisableFeatureCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "disable-feature <space-name> <feature-name>",
		Short: "Disable a feature for the given Space",
		Long: `Disable a feature toggle for the given Space. There are two expected 
parameters - the first one is the Space name and the second is the name of the feature toggle that should be disabled for the Space.`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			term := ioutils.NewTerminal(cmd.InOrStdin, cmd.OutOrStdout)
			ctx := clicontext.NewCommandContext(term, client.DefaultNewClient)
			return DisableFeature(ctx, args[0], args[1])
		},
	}
}

func DisableFeature(ctx *clicontext.CommandContext, spaceName, featureToggleName string) error {
	return client.PatchSpace(ctx, spaceName, func(space *toolchainv1alpha1.Space) (bool, error) {
		currentFeatures := strings.TrimSpace(space.Annotations[toolchainv1alpha1.FeatureToggleNameAnnotationKey])
		var enabledFeatures []string
		if currentFeatures != "" {
			enabledFeatures = strings.Split(currentFeatures, ",")
		}
		if err := ctx.PrintObject(space, "The current Space"); err != nil {
			return false, err
		}

		if !slices.Contains(enabledFeatures, featureToggleName) {
			ctx.Println("")
			ctx.Println("The Space doesn't have the feature toggle enabled. There is nothing to do.")
			ctx.Println("")
			return false, nil
		}

		confirmation := ctx.AskForConfirmation(ioutils.WithMessagef(
			"disable the feature toggle '%s' for the Space '%s'? The enabled feature toggles are '%s'.",
			featureToggleName, spaceName, currentFeatures))

		if confirmation {
			index := slices.Index(enabledFeatures, featureToggleName)
			enabledFeatures = append(enabledFeatures[:index], enabledFeatures[index+1:]...)
			if len(enabledFeatures) == 0 {
				delete(space.Annotations, toolchainv1alpha1.FeatureToggleNameAnnotationKey)
			} else {
				space.Annotations[toolchainv1alpha1.FeatureToggleNameAnnotationKey] = strings.Join(enabledFeatures, ",")
			}
			return true, nil
		}
		return false, nil
	}, "Successfully disabled feature toggle for the Space")
}

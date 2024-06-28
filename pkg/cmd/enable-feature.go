package cmd

import (
	"fmt"
	"strings"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/kubesaw/ksctl/pkg/client"
	"github.com/kubesaw/ksctl/pkg/configuration"
	clicontext "github.com/kubesaw/ksctl/pkg/context"
	"github.com/kubesaw/ksctl/pkg/ioutils"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/strings/slices"

	"github.com/spf13/cobra"
)

func NewEnableFeatureCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "enable-feature <space-name> <feature-name>",
		Short: "Enable a feature for the given Space",
		Long: `Enable a feature toggle for the given Space. There are two expected 
parameters - the first one is the Space name and the second is the name of the feature toggle that should be enabled for the Space.`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			term := ioutils.NewTerminal(cmd.InOrStdin, cmd.OutOrStdout)
			ctx := clicontext.NewCommandContext(term, client.DefaultNewClient)
			return EnableFeature(ctx, args[0], args[1])
		},
	}
}

func EnableFeature(ctx *clicontext.CommandContext, spaceName, featureToggleName string) error {
	return client.PatchSpace(ctx, spaceName, func(space *toolchainv1alpha1.Space) (bool, error) {
		cfg, err := configuration.LoadClusterConfig(ctx, configuration.HostName)
		if err != nil {
			return false, err
		}
		cl, err := ctx.NewClient(cfg.Token, cfg.ServerAPI)
		if err != nil {
			return false, err
		}

		// get ToolchainConfig to check if the feature toggle is supported or not
		config := &toolchainv1alpha1.ToolchainConfig{}
		namespacedName := types.NamespacedName{Namespace: cfg.OperatorNamespace, Name: "config"}
		if err := cl.Get(ctx, namespacedName, config); err != nil {
			return false, fmt.Errorf("unable to get ToolchainConfig: %w", err)
		}
		// if no feature toggle is supported then return an error
		if len(config.Spec.Host.Tiers.FeatureToggles) == 0 {
			return false, fmt.Errorf("the feature toggle is not supported - the list of supported toggles is empty")
		}

		supportedFeatureToggles := make([]string, len(config.Spec.Host.Tiers.FeatureToggles))
		for i, fToggle := range config.Spec.Host.Tiers.FeatureToggles {
			supportedFeatureToggles[i] = fToggle.Name
		}

		// if the requested feature is not in the list of supported toggles, then print the list of supported ones and return an error
		if !slices.Contains(supportedFeatureToggles, featureToggleName) {
			ctx.Printlnf("The feature toggle '%s' is not listed as a supported feature toggle in ToolchainConfig CR.", featureToggleName)
			fToggleNamesList := "\n"
			for _, fToggleName := range supportedFeatureToggles {
				fToggleNamesList += fmt.Sprintf("%s\n", fToggleName)
			}
			ctx.PrintContextSeparatorWithBodyf(fToggleNamesList, "The supported feature toggles are:")
			return false, fmt.Errorf("the feature toggle is not supported")
		}

		// get already enabled features for the space
		currentFeatures := strings.TrimSpace(space.Annotations[toolchainv1alpha1.FeatureToggleNameAnnotationKey])
		var enabledFeatures []string
		if currentFeatures != "" {
			enabledFeatures = strings.Split(currentFeatures, ",")
		}
		if err := ctx.PrintObject(space, "The current Space"); err != nil {
			return false, err
		}

		// check if it's already enabled or not
		if slices.Contains(enabledFeatures, featureToggleName) {
			ctx.Println("")
			ctx.Println("The space has the feature toggle already enabled. There is nothing to do.")
			ctx.Println("")
			return false, nil
		}

		confirmation := ctx.AskForConfirmation(ioutils.WithMessagef(
			"enable the feature toggle '%s' for the Space '%s'? The already enabled feature toggles are '%s'.",
			featureToggleName, spaceName, currentFeatures))

		if confirmation {
			enabledFeatures = append(enabledFeatures, featureToggleName)
			if space.Annotations == nil {
				space.Annotations = map[string]string{}
			}
			space.Annotations[toolchainv1alpha1.FeatureToggleNameAnnotationKey] = strings.Join(enabledFeatures, ",")
			return true, nil
		}
		return false, nil
	}, "Successfully enabled feature toggle for the Space")
}

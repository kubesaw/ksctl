package cmd

import (
	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/kubesaw/ksctl/pkg/client"
	"github.com/kubesaw/ksctl/pkg/configuration"
	clicontext "github.com/kubesaw/ksctl/pkg/context"
	"github.com/kubesaw/ksctl/pkg/ioutils"

	"github.com/spf13/cobra"
)

func NewPromoteSpaceCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "promote-space <space-name> <target-tier>",
		Short: "Promote a Space to the given tier",
		Long: `Promote a Space to the given tier. There are two expected 
parameters - first one is Space name and second is the name of the target NSTemplateTier that the space should be promoted to`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			term := ioutils.NewTerminal(cmd.InOrStdin(), cmd.OutOrStdout(), ioutils.WithVerbose(configuration.Verbose))
			ctx := clicontext.NewCommandContext(term, client.DefaultNewClient)
			return PromoteSpace(ctx, args[0], args[1])
		},
	}
}

func PromoteSpace(ctx *clicontext.CommandContext, spaceName, targetTier string) error {
	return client.PatchSpace(ctx, spaceName, func(space *toolchainv1alpha1.Space) (bool, error) {
		cfg, err := configuration.LoadClusterConfig(ctx.Logger, configuration.HostName)
		if err != nil {
			return false, err
		}
		cl, err := ctx.NewClient(cfg.Token, cfg.ServerAPI)
		if err != nil {
			return false, err
		}

		// verify the NSTemplateTier exists
		if _, err := client.GetNSTemplateTier(cfg, cl, targetTier); err != nil {
			return false, err
		}

		if err := ctx.PrintObject("Space to be promoted:", space); err != nil {
			return false, err
		}

		if confirm, err := ctx.Confirm("Promote the '%s' Space to the '%s' tier?", spaceName, targetTier); err != nil || !confirm {
			return false, err
		}
		// set target tier
		space.Spec.TierName = targetTier
		return true, nil
	}, "Successfully promoted Space")
}

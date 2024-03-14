package cmd

import (
	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/kubesaw/ksctl/pkg/client"
	"github.com/kubesaw/ksctl/pkg/configuration"
	clicontext "github.com/kubesaw/ksctl/pkg/context"
	"github.com/kubesaw/ksctl/pkg/ioutils"

	"github.com/spf13/cobra"
)

func NewPromoteUserCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "promote-user <masteruserrecord-name> <target-tier>",
		Short: "Promote a user for the given MasterUserRecord resource to the given user tier",
		Long: `Promote a user for the given MasterUserRecord to the given user tier. There are two expected 
parameters - first one is MasterUserRecord name and second is the name of the target tier that the user should be promoted to`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			term := ioutils.NewTerminal(cmd.InOrStdin, cmd.OutOrStdout)
			ctx := clicontext.NewCommandContext(term, client.DefaultNewClient, client.DefaultNewRESTClient)
			return PromoteUser(ctx, args[0], args[1])
		},
	}
}

func PromoteUser(ctx *clicontext.CommandContext, murName, targetTier string) error {
	return client.PatchMasterUserRecord(ctx, murName, func(mur *toolchainv1alpha1.MasterUserRecord) (bool, error) {

		cfg, err := configuration.LoadClusterConfig(ctx, configuration.HostName)
		if err != nil {
			return false, err
		}
		cl, err := ctx.NewClient(cfg.Token, cfg.ServerAPI)
		if err != nil {
			return false, err
		}

		// verify user tier exists
		if _, err := client.GetUserTier(cfg, cl, targetTier); err != nil {
			return false, err
		}

		if err := ctx.PrintObject(mur, "MasterUserRecord to be promoted"); err != nil {
			return false, err
		}

		confirmation := ctx.AskForConfirmation(ioutils.WithMessagef(
			"promote the MasterUserRecord '%s' to the '%s' user tier?",
			murName, targetTier))

		if confirmation {
			// set target tier
			mur.Spec.TierName = targetTier
			return true, nil
		}
		return false, nil
	}, "Successfully promoted MasterUserRecord")
}

package cmd

import (
	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/kubesaw/ksctl/pkg/client"
	"github.com/kubesaw/ksctl/pkg/configuration"
	clicontext "github.com/kubesaw/ksctl/pkg/context"
	"github.com/kubesaw/ksctl/pkg/ioutils"
	"github.com/spf13/cobra"
)

func NewDisableUserCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "disable-user <mur-name>",
		Short: "Disable the given MasterUserRecord resource",
		Long: `Disable the given MasterUserRecord resource. Expects 
only one parameter which is the name of the MasterUserRecord to be disabled`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			term := ioutils.NewTerminal(cmd.InOrStdin(), cmd.OutOrStdout(), ioutils.WithVerbose(configuration.Verbose))
			ctx := clicontext.NewCommandContext(term, client.DefaultNewClient)
			return DisableUser(ctx, args...)
		},
	}
}

func DisableUser(ctx *clicontext.CommandContext, args ...string) error {
	return client.PatchMasterUserRecord(ctx, args[0], func(masterUserRecord *toolchainv1alpha1.MasterUserRecord) (bool, error) {
		if err := ctx.PrintObject("MasterUserRecord to be disabled:", masterUserRecord); err != nil {
			return false, err
		}
		ctx.Warn("!!!  DANGER ZONE  !!!")
		ctx.Warn("Disabling the MasterUserRecord will delete the User and Identity resources so the user cannot login anymore")
		if confirm, err := ctx.Confirm("Disable the MasterUserRecord above?"); err != nil || !confirm {
			return false, err
		}
		masterUserRecord.Spec.Disabled = true
		return true, nil
	}, "MasterUserRecord has been disabled")
}

package cmd

import (
	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/states"
	"github.com/kubesaw/ksctl/pkg/client"
	clicontext "github.com/kubesaw/ksctl/pkg/context"
	"github.com/kubesaw/ksctl/pkg/ioutils"

	"github.com/spf13/cobra"
)

func NewDeactivateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "deactivate <usersignup-name>",
		Short: "Deactivate the given UserSignup resource",
		Long: `Deactivate the given UserSignup resource. There is expected 
only one parameter which is the name of the UserSignup to be deactivated`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			term := ioutils.NewTerminal(cmd.InOrStdin, cmd.OutOrStdout)
			ctx := clicontext.NewCommandContext(term, client.DefaultNewClient)
			return Deactivate(ctx, args...)
		},
	}
}

func Deactivate(ctx *clicontext.CommandContext, args ...string) error {
	return client.PatchUserSignup(ctx, args[0], func(userSignup *toolchainv1alpha1.UserSignup) (bool, error) {
		if err := ctx.PrintObject(userSignup, "UserSignup to be deactivated"); err != nil {
			return false, err
		}
		confirmation := ctx.AskForConfirmation(ioutils.WithDangerZoneMessagef(
			"deletion of all user's namespaces and all related data", "deactivate the UserSignup above?"))
		if confirmation {
			states.SetDeactivated(userSignup, true)
			return true, nil
		}
		return false, nil
	}, "UserSignup has been deactivated")
}

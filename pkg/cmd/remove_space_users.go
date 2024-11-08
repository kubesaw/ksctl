package cmd

import (
	"context"
	"fmt"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/kubesaw/ksctl/pkg/client"
	"github.com/kubesaw/ksctl/pkg/cmd/flags"
	"github.com/kubesaw/ksctl/pkg/configuration"
	clicontext "github.com/kubesaw/ksctl/pkg/context"
	"github.com/kubesaw/ksctl/pkg/ioutils"

	"github.com/spf13/cobra"
)

func NewRemoveSpaceUsersCmd() *cobra.Command {
	var spaceName string
	var users []string
	command := &cobra.Command{
		Use:   "remove-space-users --space=<space> --users=<\"masteruserrecord1 masteruserrecord2...\">",
		Short: "Delete a SpaceBindings between the given Space and the given MasterUserRecords",
		Long: `Delete SpaceBindings between the given Space and the given MasterUserRecords. The first parameter is the name of the Space followed by
one or more users specified by their MasterUserRecord name.`,
		Args: cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			term := ioutils.NewTerminal(cmd.InOrStdin(), cmd.OutOrStdout(), ioutils.WithVerbose(configuration.Verbose))
			ctx := clicontext.NewCommandContext(term, client.DefaultNewClient)
			return RemoveSpaceUsers(ctx, spaceName, users)
		},
	}
	command.Flags().StringVarP(&spaceName, "space", "s", "", "the name of the space to remove users from")
	flags.MustMarkRequired(command, "space")
	command.Flags().StringArrayVarP(&users, "users", "u", []string{}, "the masteruserrecord names of the users to remove from the space")
	flags.MustMarkRequired(command, "users")

	return command
}

func RemoveSpaceUsers(ctx *clicontext.CommandContext, spaceName string, usersToRemove []string) error {
	cfg, err := configuration.LoadClusterConfig(ctx.Logger, configuration.HostName) // uses the same token as add-space-users
	if err != nil {
		return err
	}
	cl, err := ctx.NewClient(cfg.Token, cfg.ServerAPI)
	if err != nil {
		return err
	}

	// get Space
	ctx.Info("Checking space...")
	space, err := client.GetSpace(cl, cfg.OperatorNamespace, spaceName)
	if err != nil {
		return err
	}

	// get SpaceBindings to delete
	spaceBindingsToDelete := []*toolchainv1alpha1.SpaceBinding{}
	for _, murName := range usersToRemove {
		sbs, err := client.ListSpaceBindings(cl, cfg.OperatorNamespace, client.ForSpace(spaceName), client.ForMasterUserRecord(murName))
		if err != nil {
			return err
		}
		if len(sbs) == 0 {
			return fmt.Errorf("no SpaceBinding found for Space '%s' and MasterUserRecord '%s'", spaceName, murName)
		}
		for i := range sbs {
			spaceBindingsToDelete = append(spaceBindingsToDelete, &sbs[i])
		}
	}

	// confirmation before SpaceBinding deletion
	if err := ctx.PrintObject("Space:", space); err != nil {
		return err
	}
	if confirm, err := ctx.Confirm("Remove users from the Space above?"); err != nil || !confirm {
		return err
	}
	ctx.Info("Deleting SpaceBinding(s)...")
	// delete SpaceBindings
	for _, sb := range spaceBindingsToDelete {
		if err := cl.Delete(context.TODO(), sb); err != nil {
			return err
		}
	}

	ctx.Info("All SpaceBinding(s) successfully deleted")
	return nil
}

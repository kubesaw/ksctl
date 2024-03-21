package cmd

import (
	"context"
	"fmt"
	"strings"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/spacebinding"
	"github.com/kubesaw/ksctl/pkg/client"
	"github.com/kubesaw/ksctl/pkg/cmd/flags"
	"github.com/kubesaw/ksctl/pkg/configuration"
	clicontext "github.com/kubesaw/ksctl/pkg/context"
	"github.com/kubesaw/ksctl/pkg/ioutils"

	"github.com/spf13/cobra"
)

func NewAddSpaceUsersCmd() *cobra.Command {
	var spaceName string
	var role string
	var users []string
	command := &cobra.Command{
		Use:   "add-space-users --space=<space> --role=<role> --users=<masteruserrecord1,masteruserrecord2...>",
		Short: "Create SpaceBinding(s) between the given Space and the given MasterUserRecord(s)",
		Long: `Create SpaceBinding(s) between the given Space and the given MasterUserRecord(s). The first parameter is the name of the Space followed by the role to be assigned to the user and
one or more users specified by their MasterUserRecord name. One SpaceBinding will be created for each user.`,
		Args: cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			term := ioutils.NewTerminal(cmd.InOrStdin, cmd.OutOrStdout)
			ctx := clicontext.NewCommandContext(term, client.DefaultNewClient)

			return AddSpaceUsers(ctx, spaceName, role, users)
		},
	}
	command.Flags().StringVarP(&spaceName, "space", "s", "", "the name of the space to add users to")
	flags.MustMarkRequired(command, "space")
	command.Flags().StringVarP(&role, "role", "r", "", "the name of the role to assign to the users")
	flags.MustMarkRequired(command, "role")
	command.Flags().StringSliceVarP(&users, "users", "u", []string{}, "the masteruserrecord names of the users to add to the space delimited by comma")
	flags.MustMarkRequired(command, "users")

	return command
}

func AddSpaceUsers(ctx *clicontext.CommandContext, spaceName, role string, usersToAdd []string) error {
	cfg, err := configuration.LoadClusterConfig(ctx, configuration.HostName)
	if err != nil {
		return err
	}
	cl, err := ctx.NewClient(cfg.Token, cfg.ServerAPI)
	if err != nil {
		return err
	}

	// get Space
	ctx.Println("Checking space...")
	space, err := client.GetSpace(cl, cfg.OperatorNamespace, spaceName)
	if err != nil {
		return err
	}

	nsTemplTierName := space.Spec.TierName
	nsTemplTier, err := client.GetNSTemplateTier(cfg, cl, nsTemplTierName)
	if err != nil {
		return err
	}

	// check if role is within the allowed spaceroles from the NSTemplateTier
	isRoleValid := false
	validRolesMsg := strings.Builder{}
	validRolesMsg.WriteString("the following are valid roles:\n")

	for actual := range nsTemplTier.Spec.SpaceRoles {
		validRolesMsg.WriteString(fmt.Sprintf("%s\n", actual))
		if role == actual {
			isRoleValid = true
		}
	}
	if !isRoleValid {
		return fmt.Errorf("invalid role '%s' for space '%s' - %s", role, spaceName, validRolesMsg.String())
	}

	// get MasterUserRecords
	ctx.Println("Checking users...")
	spaceBindingsToCreate := []*toolchainv1alpha1.SpaceBinding{}
	for _, murName := range usersToAdd {
		mur, err := client.GetMasterUserRecord(cl, cfg.OperatorNamespace, murName)
		if err != nil {
			return err
		}
		spaceBindingsToCreate = append(spaceBindingsToCreate, spacebinding.NewSpaceBinding(mur, space, space.Labels[toolchainv1alpha1.SpaceCreatorLabelKey], spacebinding.WithRole(role)))
	}

	// confirmation before SpaceBinding creation
	if err := ctx.PrintObject(space, "Targeted Space"); err != nil {
		return err
	}
	confirmation := ctx.AskForConfirmation(ioutils.WithMessagef(
		"add users to the above Space?"))
	if !confirmation {
		return nil
	}

	ctx.Println("Creating SpaceBinding(s)...")
	// create SpaceBindings
	for _, sb := range spaceBindingsToCreate {
		if err := cl.Create(context.TODO(), sb); err != nil {
			return err
		}
	}

	ctx.Printlnf("\nSpaceBinding(s) successfully created")
	return nil
}

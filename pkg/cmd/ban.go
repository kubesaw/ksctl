package cmd

import (
	"context"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/banneduser"
	"github.com/kubesaw/ksctl/pkg/client"
	"github.com/kubesaw/ksctl/pkg/configuration"
	clicontext "github.com/kubesaw/ksctl/pkg/context"
	"github.com/kubesaw/ksctl/pkg/ioutils"

	"github.com/spf13/cobra"
)

func NewBanCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ban <usersignup-name>",
		Short: "Ban a user for the given UserSignup resource",
		Long: `Ban the given UserSignup resource. There is expected 
only one parameter which is the name of the UserSignup to be used for banning`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			term := ioutils.NewTerminal(cmd.InOrStdin, cmd.OutOrStdout)
			ctx := clicontext.NewCommandContext(term, client.DefaultNewClient)
			return Ban(ctx, args...)
		},
	}
}

const BannedByLabel = toolchainv1alpha1.LabelKeyPrefix + "banned-by"

func Ban(ctx *clicontext.CommandContext, args ...string) error {
	return CreateBannedUser(ctx, args[0], func(userSignup *toolchainv1alpha1.UserSignup, bannedUser *toolchainv1alpha1.BannedUser) (bool, error) {
		if _, exists := bannedUser.Labels[toolchainv1alpha1.BannedUserPhoneNumberHashLabelKey]; !exists {
			ctx.Printlnf("\nINFO: The UserSignup doesn't have the label '%s' set, so the resulting BannedUser resource won't have this label either.\n",
				toolchainv1alpha1.BannedUserPhoneNumberHashLabelKey)
		}

		if err := ctx.PrintObject(bannedUser, "BannedUser resource to be created"); err != nil {
			return false, err
		}

		confirmation := ctx.AskForConfirmation(ioutils.WithDangerZoneMessagef(
			"deletion of all user's namespaces and all related data.\nIn addition, the user won't be able to login any more.",
			"ban the user with the UserSignup by creating BannedUser resource that are both above?"))
		return confirmation, nil
	})
}

func CreateBannedUser(ctx *clicontext.CommandContext, userSignupName string, confirm func(*toolchainv1alpha1.UserSignup, *toolchainv1alpha1.BannedUser) (bool, error)) error {
	cfg, err := configuration.LoadClusterConfig(ctx, configuration.HostName)
	if err != nil {
		return err
	}
	cl, err := ctx.NewClient(cfg.Token, cfg.ServerAPI)
	if err != nil {
		return err
	}

	userSignup, err := client.GetUserSignup(cl, cfg.OperatorNamespace, userSignupName)
	if err != nil {
		return err
	}

	ksctlConfig, err := configuration.Load(ctx)
	if err != nil {
		return err
	}

	bannedUser, err := banneduser.NewBannedUser(userSignup, ksctlConfig.Name)
	if err != nil {
		return err
	}

	alreadyBannedUser, err := banneduser.GetBannedUser(ctx, bannedUser.Labels[toolchainv1alpha1.BannedUserEmailHashLabelKey], cl, cfg.OperatorNamespace)
	if err != nil {
		return err
	}

	if err := ctx.PrintObject(userSignup, "UserSignup to be banned"); err != nil {
		return err
	}

	if alreadyBannedUser != nil {
		ctx.Println("The user was already banned - there is a BannedUser resource with the same labels already present")
		return ctx.PrintObject(alreadyBannedUser, "BannedUser resource")
	}

	if shouldCreate, err := confirm(userSignup, bannedUser); !shouldCreate || err != nil {
		return err
	}

	if err := cl.Create(context.TODO(), bannedUser); err != nil {
		return err
	}

	ctx.Printlnf("\nUserSignup has been banned by creating BannedUser resource with name " + bannedUser.Name)
	return nil
}

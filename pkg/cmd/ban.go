package cmd

import (
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
		Use:   "ban <usersignup-name> <ban-reason>",
		Short: "Ban a user for the given UserSignup resource and reason of the ban",
		Long: `Ban the given UserSignup resource. There is expected 
only two parameters which the first one is the name of the UserSignup to be used for banning 
and the second one the reason of the ban`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			term := ioutils.NewTerminal(cmd.InOrStdin(), cmd.OutOrStdout(), ioutils.WithVerbose(configuration.Verbose))
			ctx := clicontext.NewCommandContext(term, client.DefaultNewClient)
			return Ban(ctx, args...)
		},
	}
}

func Ban(ctx *clicontext.CommandContext, args ...string) error {
	return CreateBannedUser(ctx, args[0], args[1], func(userSignup *toolchainv1alpha1.UserSignup, bannedUser *toolchainv1alpha1.BannedUser) (bool, error) {
		if _, exists := bannedUser.Labels[toolchainv1alpha1.BannedUserPhoneNumberHashLabelKey]; !exists {
			ctx.Infof("The UserSignup doesn't have the label '%s' set, so the resulting BannedUser resource won't have this label either.\n",
				toolchainv1alpha1.BannedUserPhoneNumberHashLabelKey)
		}

		if err := ctx.PrintObject("BannedUser resource to be created:", bannedUser); err != nil {
			return false, err
		}

		ctx.Warn("!!!  DANGER ZONE  !!!")
		ctx.Warn("Deleting all the user's namespaces and all their resources")
		ctx.Warn("In addition, the user won't be able to login anymore")
		return ctx.Confirm("Ban the user with the UserSignup by creating a BannedUser resource?")
	})
}

func CreateBannedUser(ctx *clicontext.CommandContext, userSignupName, banReason string, confirm func(*toolchainv1alpha1.UserSignup, *toolchainv1alpha1.BannedUser) (bool, error)) error {
	cfg, err := configuration.LoadClusterConfig(ctx.Logger, configuration.HostName)
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

	ksctlConfig, err := configuration.Load(ctx.Logger)
	if err != nil {
		return err
	}

	bannedUser, err := banneduser.NewBannedUser(userSignup, ksctlConfig.Name, banReason)
	if err != nil {
		return err
	}

	alreadyBannedUser, err := banneduser.GetBannedUser(ctx, bannedUser.Labels[toolchainv1alpha1.BannedUserEmailHashLabelKey], cl, cfg.OperatorNamespace)
	if err != nil {
		return err
	}

	if err := ctx.PrintObject("UserSignup to be banned:", userSignup); err != nil {
		return err
	}

	if alreadyBannedUser != nil {
		ctx.Info("The user was already banned - there is a BannedUser resource with the same labels already present")
		return ctx.PrintObject("BannedUser resource", alreadyBannedUser)
	}

	if shouldCreate, err := confirm(userSignup, bannedUser); !shouldCreate || err != nil {
		return err
	}

	if err := cl.Create(ctx.Context, bannedUser); err != nil {
		return err
	}

	ctx.Infof("UserSignup has been banned by creating BannedUser resource with name '%s'", bannedUser.Name)
	return nil
}

package cmd

import (
	"context"
	"fmt"
	"time"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/states"
	"github.com/kubesaw/ksctl/pkg/client"
	"github.com/kubesaw/ksctl/pkg/configuration"
	clicontext "github.com/kubesaw/ksctl/pkg/context"
	"github.com/kubesaw/ksctl/pkg/ioutils"

	"github.com/spf13/cobra"
)

func NewVerifyCmd() *cobra.Command {
	var usersignupName string
	var emailAddress string
	command := &cobra.Command{
		Use:   "verify <--email someone@example.com or --name someone>",
		Short: "Mark the given UserSignup resource as now verified",
		Long: `Mark the given UserSignup resource as verified at this given point in time. 
There is expected only one parameter which is the name of the UserSignup to be verified`,
		Args: cobra.ExactArgs(0),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if usersignupName != "" && emailAddress != "" {
				return fmt.Errorf("you cannot specify both 'name' and `email` flags")
			}
			if usersignupName == "" && emailAddress == "" {
				return fmt.Errorf("you must specify one of 'name' and `email` flags")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			term := ioutils.NewTerminal(cmd.InOrStdin, cmd.OutOrStdout)
			ctx := clicontext.NewCommandContext(term, client.DefaultNewClient)
			switch {
			case usersignupName != "":
				return Verify(ctx, ByName(usersignupName))
			default:
				return Verify(ctx, ByEmailAddress(emailAddress))
			}
		},
	}
	command.Flags().StringVar(&usersignupName, "name", "", "the name of the UserSignup resource")
	command.Flags().StringVar(&emailAddress, "email", "", "the email address of the user")
	return command
}

func Verify(ctx *clicontext.CommandContext, lookupUserSignup LookupUserSignup) error {
	cfg, err := configuration.LoadClusterConfig(ctx, configuration.HostName)
	if err != nil {
		return err
	}
	cl, err := ctx.NewClient(cfg.Token, cfg.ServerAPI)
	if err != nil {
		return err
	}
	userSignup, err := lookupUserSignup(cfg, cl)
	if err != nil {
		return err
	}

	if err := ctx.PrintObject(userSignup, "UserSignup to be verified"); err != nil {
		return err
	}
	if !ctx.AskForConfirmation(ioutils.WithMessagef("verify the UserSignup above?")) {
		return nil
	}

	userSignup.Annotations[toolchainv1alpha1.UserSignupVerifiedTimestampAnnotationKey] = time.Now().Format(time.RFC3339)
	states.SetVerificationRequired(userSignup, false)
	states.SetRejected(userSignup, false)

	if err := cl.Update(context.TODO(), userSignup); err != nil {
		return err
	}
	ctx.Printlnf("UserSignup has been verified")
	return nil
}

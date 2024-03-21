package cmd

import (
	"context"

	"github.com/kubesaw/ksctl/pkg/client"
	"github.com/kubesaw/ksctl/pkg/configuration"
	clicontext "github.com/kubesaw/ksctl/pkg/context"
	"github.com/kubesaw/ksctl/pkg/ioutils"

	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func NewGdprDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "gdpr-delete <usersignup-name>",
		Short: "Delete the given UserSignup resource",
		Long: `Delete the given UserSignup resource. There is expected 
only one parameter which is the name of the UserSignup to be deleted`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			term := ioutils.NewTerminal(cmd.InOrStdin, cmd.OutOrStdout)
			ctx := clicontext.NewCommandContext(term, client.DefaultNewClient)
			return Delete(ctx, args...)
		},
	}
}

func Delete(ctx *clicontext.CommandContext, args ...string) error {
	cfg, err := configuration.LoadClusterConfig(ctx, configuration.HostName)
	if err != nil {
		return err
	}
	cl, err := ctx.NewClient(cfg.Token, cfg.ServerAPI)
	if err != nil {
		return err
	}
	userSignup, err := client.GetUserSignup(cl, cfg.SandboxNamespace, args[0])
	if err != nil {
		return err
	}
	if err := ctx.PrintObject(userSignup, "UserSignup to be deleted"); err != nil {
		return err
	}
	confirmation := ctx.AskForConfirmation(ioutils.WithDangerZoneMessagef(
		"deletion of all user's namespaces and all related data.\n"+
			"This command should be executed based on GDPR request.", "delete the UserSignup above?"))
	if !confirmation {
		return nil
	}
	propagationPolicy := metav1.DeletePropagationForeground
	opts := runtimeclient.DeleteOption(&runtimeclient.DeleteOptions{
		PropagationPolicy: &propagationPolicy,
	})
	if err := cl.Delete(context.TODO(), userSignup, opts); err != nil {
		return err
	}
	ctx.Printlnf("\nThe deletion of the UserSignup has been triggered")
	return nil
}

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
			term := ioutils.NewTerminal(cmd.InOrStdin(), cmd.OutOrStdout(), ioutils.WithVerbose(configuration.Verbose))
			ctx := clicontext.NewCommandContext(term, client.DefaultNewClient)
			return Delete(ctx, args...)
		},
	}
}

func Delete(ctx *clicontext.CommandContext, args ...string) error {
	cfg, err := configuration.LoadClusterConfig(ctx.Logger, configuration.HostName)
	if err != nil {
		return err
	}
	cl, err := ctx.NewClient(cfg.Token, cfg.ServerAPI)
	if err != nil {
		return err
	}
	userSignup, err := client.GetUserSignup(cl, cfg.OperatorNamespace, args[0])
	if err != nil {
		return err
	}
	if err := ctx.PrintObject("UserSignup to be deleted:", userSignup); err != nil {
		return err
	}
	ctx.Warn("!!!  DANGER ZONE  !!!")
	ctx.Warn("Deleting all the user's namespaces and all their resources")
	ctx.Warn("This command should be executed after a GDPR request")
	if confirm, err := ctx.Confirm("Delete the UserSignup above?"); err != nil || !confirm {
		return err
	}
	propagationPolicy := metav1.DeletePropagationForeground
	opts := runtimeclient.DeleteOption(&runtimeclient.DeleteOptions{
		PropagationPolicy: &propagationPolicy,
	})
	if err := cl.Delete(context.TODO(), userSignup, opts); err != nil {
		return err
	}
	ctx.Info("The deletion of the UserSignup has been triggered")
	return nil
}

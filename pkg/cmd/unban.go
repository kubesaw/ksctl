package cmd

import (
	"fmt"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/hash"
	"github.com/kubesaw/ksctl/pkg/client"
	"github.com/kubesaw/ksctl/pkg/configuration"
	clicontext "github.com/kubesaw/ksctl/pkg/context"
	"github.com/kubesaw/ksctl/pkg/ioutils"
	"github.com/spf13/cobra"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func NewUnbanCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "unban <email>",
		Short: "Unban the user so that they can use the platform again",
		Long:  "Unban the user that previously registered with the provided email so that they can start using the platform again.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			term := ioutils.NewTerminal(cmd.InOrStdin, cmd.OutOrStdout)
			ctx := clicontext.NewCommandContext(term, client.DefaultNewClient)
			return Unban(ctx, args[0])
		},
	}
}

func Unban(ctx *clicontext.CommandContext, email string) error {
	cl, cfg, err := getClient(ctx)
	if err != nil {
		return err
	}

	emailHash := hash.EncodeString(email)
	list := &toolchainv1alpha1.BannedUserList{}

	if err = cl.List(ctx.Context, list,
		runtimeclient.InNamespace(cfg.OperatorNamespace),
		runtimeclient.MatchingLabels{toolchainv1alpha1.BannedUserEmailHashLabelKey: emailHash}); err != nil {
		return err
	}

	if len(list.Items) == 0 {
		ctx.Println("No BannedUser objects found with given email.")
		return nil
	}

	if len(list.Items) > 1 {
		ctx.Println("More than 1 BannedUser found for given email. Found:")
		for _, bu := range list.Items {
			_ = ctx.PrintObject(&bu, "")
		}
		return fmt.Errorf("expected 0 or 1 BannedUser objects to correspond to the email '%s' but %d found", email, len(list.Items))
	}

	bu := &list.Items[0]
	if bu.Spec.Email != email {
		_ = ctx.PrintObject(bu, "Inconsistent BannedUser encountered - the email doesn't correspond to the email-hash")
		return fmt.Errorf("inconsistent BannedUser, the email '%s' doesn't correspond to the email-hash label value '%s'", bu.Spec.Email, emailHash)
	}

	err = cl.Delete(ctx.Context, bu)
	if err != nil {
		return err
	}

	ctx.Println("User successfully unbanned")
	return nil
}

func getClient(ctx *clicontext.CommandContext) (runtimeclient.Client, *configuration.ClusterConfig, error) {
	cfg, err := configuration.LoadClusterConfig(ctx, configuration.HostName)
	if err != nil {
		return nil, nil, err
	}
	cl, err := ctx.NewClient(cfg.Token, cfg.ServerAPI)
	if err != nil {
		return nil, nil, err
	}
	return cl, &cfg, nil
}

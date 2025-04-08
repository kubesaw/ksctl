package cmd

import (

	"github.com/codeready-toolchain/toolchain-common/pkg/banneduser"
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

  bu, err := banneduser.GetBannedUser(ctx, emailHash, cl, cfg.OperatorNamespace)
  if err != nil {
    return err
  }
  if bu == nil {
    ctx.Println("No banned user with given email found.")
    return nil
  }

  if err := ctx.PrintObject(bu, "BannedUser resource to be deleted"); err != nil {
		return err
	}
	if !ctx.AskForConfirmation(ioutils.WithMessagef("delete the BannedUser resource above and thus unban all UserSignups with the given email?")) {
		return nil
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

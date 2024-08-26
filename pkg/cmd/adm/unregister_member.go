package adm

import (
	"context"
	"fmt"
	"os"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/kubesaw/ksctl/pkg/client"
	"github.com/kubesaw/ksctl/pkg/configuration"
	clicontext "github.com/kubesaw/ksctl/pkg/context"
	"github.com/kubesaw/ksctl/pkg/ioutils"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	kubectlrollout "k8s.io/kubectl/pkg/cmd/rollout"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

func NewUnregisterMemberCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "unregister-member <member-name>",
		Short: "Deletes member from host",
		Long:  `Deletes the member cluster from the host cluster. It doesn't touch the member cluster itself. Make sure there is no users left in the member cluster before unregistering it.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			term := ioutils.NewTerminal(cmd.InOrStdin, cmd.OutOrStdout)
			ctx := clicontext.NewCommandContext(term, client.DefaultNewClient)
			return UnregisterMemberCluster(ctx, args[0])
		},
	}
}

func UnregisterMemberCluster(ctx *clicontext.CommandContext, clusterName string) error {
	hostClusterConfig, err := configuration.LoadClusterConfig(ctx, configuration.HostName)
	if err != nil {
		return err
	}
	hostClusterClient, err := ctx.NewClient(hostClusterConfig.Token, hostClusterConfig.ServerAPI)
	if err != nil {
		return err
	}

	clusterDef, err := configuration.LoadClusterAccessDefinition(ctx, clusterName)
	if err != nil {
		return err
	}
	clusterResourceName := fmt.Sprintf("%s-%s", clusterDef.ClusterType, clusterDef.ServerName)

	toolchainCluster := &toolchainv1alpha1.ToolchainCluster{}
	if err := hostClusterClient.Get(context.TODO(), types.NamespacedName{Namespace: hostClusterConfig.OperatorNamespace, Name: clusterResourceName}, toolchainCluster); err != nil {
		return err
	}
	if err := ctx.PrintObject(toolchainCluster, "Toolchain Member cluster"); err != nil {
		return err
	}
	confirmation := ctx.AskForConfirmation(ioutils.WithDangerZoneMessagef("unregistering member cluster form host cluster. Make sure there is no users left in the member cluster before unregistering it.",
		"Delete Member cluster stated above from the Host cluster?"))
	if !confirmation {
		return nil
	}

	if err := hostClusterClient.Delete(context.TODO(), toolchainCluster); err != nil {
		return err
	}
	ctx.Printlnf("\nThe deletion of the Toolchain member cluster from the Host cluster has been triggered")

	return restartHostOperator(hostClusterConfig)
}

func restartHostOperator(hostConfig configuration.ClusterConfig) error {
	kubeConfigFlags := genericclioptions.NewConfigFlags(true).WithDeprecatedPasswordFlag()
	hFactory := cmdutil.NewFactory(cmdutil.NewMatchVersionFlags(kubeConfigFlags))
	ioStreams := genericclioptions.IOStreams{
		In:     nil, // Not to forward the Standard Input
		Out:    os.Stdout,
		ErrOut: os.Stderr,
	}

	hArgs := []string{"", "deployments",
		"--namespace", hostConfig.OperatorNamespace,
		"--server", hostConfig.ServerAPI,
		"--token", hostConfig.Token}

	o := kubectlrollout.NewRolloutRestartOptions(ioStreams)

	if err := o.Complete(hFactory, nil, hArgs); err != nil {
		panic(err)
	}
	o.LabelSelector = "olm.owner.namespace=toolchain-host-operator"
	if err := o.Validate(); err != nil {
		panic(err)
	}
	return o.RunRestart()
}

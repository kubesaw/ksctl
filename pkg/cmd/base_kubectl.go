package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/kubesaw/ksctl/pkg/client"
	"github.com/kubesaw/ksctl/pkg/cmd/flags"
	"github.com/kubesaw/ksctl/pkg/configuration"
	"github.com/kubesaw/ksctl/pkg/ioutils"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

type newCmd func(cmdutil.Factory, genericclioptions.IOStreams) *cobra.Command

// setupKubectlCmd takes care of setting up the flags and PreRunE func on the given Kubectl command
func setupKubectlCmd(newCmd newCmd) *cobra.Command {
	kubeConfigFlags := genericclioptions.NewConfigFlags(true).WithDeprecatedPasswordFlag()
	factory := cmdutil.NewFactory(cmdutil.NewMatchVersionFlags(kubeConfigFlags))
	ioStreams := genericclioptions.IOStreams{
		In:     os.Stdin,
		Out:    os.Stdout,
		ErrOut: os.Stderr,
	}
	cmd := newCmd(factory, ioStreams)
	cmd.Example = strings.ReplaceAll(cmd.Example, "kubectl ", "ksctl ")

	// hide unused/redefined flags
	kubeConfigFlags.ClusterName = nil     // `cluster` flag is redefined for our own purpose
	kubeConfigFlags.AuthInfoName = nil    // unused here, so we can hide it
	kubeConfigFlags.Context = nil         // unused here, so we can hide it
	kubeConfigFlags.AddFlags(cmd.Flags()) // add default flags to the command (so we have `-n`, etc.)

	cmd.Flags().StringP("target-cluster", "t", "", "Target cluster")
	// will be used to load the config (API Server URL and token)
	flags.MustMarkRequired(cmd, "target-cluster")
	// flags with values hard-coded by `PreRun` are hidden
	flags.MustMarkHidden(cmd, "server")
	flags.MustMarkHidden(cmd, "token")
	flags.MustMarkHidden(cmd, "kubeconfig")

	// set the "hard-coded" value of some specific flags before running the command,
	// by loading the config associated with the `--cluster` flag
	cmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		clusterName := cmd.Flag("target-cluster").Value.String()
		if clusterName == "" { // flag is required, but we need to manually verify its presence in the PreRun
			return fmt.Errorf("you must specify the target cluster")
		}
		term := ioutils.NewTerminal(cmd.InOrStdin, cmd.OutOrStdout)
		cfg, err := configuration.LoadClusterConfig(term, clusterName)
		if err != nil {
			return err
		}
		if !cmd.Flag("namespace").Changed { // default to kubeSaw namespace
			kubeConfigFlags.Namespace = &cfg.OperatorNamespace
		}
		kubeConfigFlags.APIServer = &cfg.ServerAPI
		kubeConfigFlags.BearerToken = &cfg.Token
		kubeconfig, err := client.EnsureKsctlConfigFile()
		if err != nil {
			return err
		}
		kubeConfigFlags.KubeConfig = &kubeconfig
		return nil
	}
	return cmd
}

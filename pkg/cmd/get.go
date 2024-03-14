package cmd

import (
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	kubectlget "k8s.io/kubectl/pkg/cmd/get"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

func NewGetCmd() *cobra.Command {
	return setupKubectlCmd(func(factory cmdutil.Factory, ioStreams genericclioptions.IOStreams) *cobra.Command {
		return kubectlget.NewCmdGet("sandbox-cli", factory, ioStreams)
	})
}

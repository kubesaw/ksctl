package cmd

import (
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	kubectldesc "k8s.io/kubectl/pkg/cmd/describe"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

func NewDescribeCmd() *cobra.Command {
	return setupKubectlCmd(func(factory cmdutil.Factory, ioStreams genericclioptions.IOStreams) *cobra.Command {
		return kubectldesc.NewCmdDescribe("ksctl", factory, ioStreams)
	})
}

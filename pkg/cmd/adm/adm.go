package adm

import (
	"github.com/kubesaw/ksctl/pkg/ioutils"

	"github.com/spf13/cobra"
)

func NewAdmCmd() *cobra.Command {
	admCommand := &cobra.Command{
		Use:   "adm",
		Short: "Administrative Commands",
		Long:  `Actions for administering a KubeSaw instance.`,
	}
	registerCommands(admCommand)

	admCommand.PersistentFlags().BoolVarP(&ioutils.AssumeYes, "assume-yes", "y", false, "Automatically answer yes for all questions.")

	return admCommand
}

func registerCommands(admCommand *cobra.Command) {
	// commands with go runtime client
	admCommand.AddCommand(NewRestartCmd())
	admCommand.AddCommand(NewUnregisterMemberCmd())
	admCommand.AddCommand(NewMustGatherNamespaceCmd())
	admCommand.AddCommand(NewInstallOperatorCmd())

	// commands running external script
	admCommand.AddCommand(NewRegisterMemberCmd())
}

package adm

import (
	"github.com/kubesaw/ksctl/pkg/ioutils"

	"github.com/spf13/cobra"
)

func NewAdmCmd() *cobra.Command {
	admCommand := &cobra.Command{
		Use:   "adm",
		Short: "Administrative Commands",
		Long:  `Actions for administering Dev Sandbox instance.`,
	}
	registerCommands(admCommand)

	admCommand.PersistentFlags().BoolVarP(&ioutils.AssumeYes, "assume-yes", "y", false, "Automatically answer yes for all questions.")

	return admCommand
}

func registerCommands(admCommand *cobra.Command) {
	// commands with go runtime client
	admCommand.AddCommand(NewRestartCmd())
	admCommand.AddCommand(NewSetupCmd())
	admCommand.AddCommand(NewGenerateCliConfigsCmd())
	admCommand.AddCommand(NewUnregisterMemberCmd())
	admCommand.AddCommand(NewMustGatherNamespaceCmd())

	// commands running external script
	admCommand.AddCommand(NewRegisterMemberCmd())
}

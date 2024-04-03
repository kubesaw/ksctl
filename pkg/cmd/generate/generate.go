package generate

import (
	"github.com/spf13/cobra"
)

func NewGenerateCmd() *cobra.Command {
	admCommand := &cobra.Command{
		Use:   "generate",
		Short: "Generate Commands",
		Long:  `Actions for generating manifests and files.`,
	}
	registerCommands(admCommand)
	return admCommand
}

func registerCommands(admCommand *cobra.Command) {
	admCommand.AddCommand(NewAdminManifestsCmd())
	admCommand.AddCommand(NewCliConfigsCmd())
}

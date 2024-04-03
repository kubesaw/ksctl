package generate

import (
	"github.com/spf13/cobra"
)

func NewGenerateCmd() *cobra.Command {
	admCommand := &cobra.Command{
		Use:   "generate",
		Short: "Generate commands",
		Long:  `Commands to generate manifests and CLI config files`,
	}
	registerCommands(admCommand)
	return admCommand
}

func registerCommands(admCommand *cobra.Command) {
	admCommand.AddCommand(NewAdminManifestsCmd())
	admCommand.AddCommand(NewCliConfigsCmd())
}

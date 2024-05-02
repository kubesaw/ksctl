package generate

import (
	"github.com/spf13/cobra"
)

func NewGenerateCmd() *cobra.Command {
	generateCommand := &cobra.Command{
		Use:   "generate",
		Short: "Generate commands",
		Long:  `Commands to generate manifests and CLI config files`,
	}
	registerCommands(generateCommand)
	return generateCommand
}

func registerCommands(generateCommand *cobra.Command) {
	generateCommand.AddCommand(NewAdminManifestsCmd())
	generateCommand.AddCommand(NewCliConfigsCmd())
	generateCommand.AddCommand(NewNSTemplateTiersCmd())
}

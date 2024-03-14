package flags

import (
	"github.com/spf13/cobra"
)

func MustMarkHidden(cmd *cobra.Command, name string) {
	if err := cmd.Flags().MarkHidden(name); err != nil {
		panic(err)
	}
}

func MustMarkRequired(cmd *cobra.Command, name string) {
	if err := cmd.MarkFlagRequired(name); err != nil {
		panic(err)
	}
}

package cmd

import (
	"github.com/spf13/cobra"
	"github.com/wbreza/azd-extensions/sdk/ext/debug"
)

func NewRootCommand() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "azd ai <group> [options]",
		Short: "A CLI for managing AI models and services",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			debug.WaitForDebugger()
		},
	}

	rootCmd.AddCommand(newModelCommand())
	rootCmd.AddCommand(newServiceCommand())

	return rootCmd
}

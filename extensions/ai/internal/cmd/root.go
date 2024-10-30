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
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	rootCmd.AddCommand(newModelCommand())
	rootCmd.AddCommand(newServiceCommand())
	rootCmd.AddCommand(newChatCommand())
	rootCmd.AddCommand(newDocumentCommand())

	rootCmd.PersistentFlags().Bool("debug", false, "Enable debug mode")

	return rootCmd
}

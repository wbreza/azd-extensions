package main

import (
	"context"
	"fmt"
	"os"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/wbreza/azd-extensions/sdk/ext/prompt"
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "azd ai <group> [options]",
		Short: "A CLI for managing AI models and services",
	}

	// Model command
	modelCmd := &cobra.Command{
		Use:   "model",
		Short: "Commands for managing models",
	}

	modelListCmd := &cobra.Command{
		Use:   "list",
		Short: "List all models",
		RunE: func(cmd *cobra.Command, args []string) error {
			subscription, err := prompt.PromptSubscription(cmd.Context())
			if err != nil {
				return err
			}

			location, err := prompt.PromptLocation(cmd.Context(), subscription)
			if err != nil {
				return err
			}

			fmt.Printf("Selected subscription: %s\n", subscription.Id)
			fmt.Printf("Selected location: %s\n", location.RegionalDisplayName)
			return nil
		},
	}

	modelDeployCmd := &cobra.Command{
		Use:   "deploy",
		Short: "Deploy a model",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("Executing model deploy command")
		},
	}

	// Add subcommands to model command
	modelCmd.AddCommand(modelListCmd)
	modelCmd.AddCommand(modelDeployCmd)

	// Service command
	serviceCmd := &cobra.Command{
		Use:   "service",
		Short: "Commands for managing services",
	}

	serviceListCmd := &cobra.Command{
		Use:   "list",
		Short: "List all services",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("Executing service list command")
		},
	}

	serviceDeployCmd := &cobra.Command{
		Use:   "deploy",
		Short: "Deploy a service",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("Executing service deploy command")
		},
	}

	// Add subcommands to service command
	serviceCmd.AddCommand(serviceListCmd)
	serviceCmd.AddCommand(serviceDeployCmd)

	// Add model and service commands to root
	rootCmd.AddCommand(modelCmd)
	rootCmd.AddCommand(serviceCmd)

	// Execute the root command
	ctx := context.Background()
	if err := rootCmd.ExecuteContext(ctx); err != nil {
		color.Red("Error: %v", err)
		os.Exit(1)
	}
}

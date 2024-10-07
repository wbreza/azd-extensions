package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "azd ai",
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
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("Executing model list command")
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
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

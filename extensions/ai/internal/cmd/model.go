package cmd

import "github.com/spf13/cobra"

func newModelCommand() *cobra.Command {
	modelCmd := &cobra.Command{
		Use:   "model",
		Short: "Commands for managing Azure AI models",
	}

	modelListCmd := &cobra.Command{
		Use:   "list",
		Short: "List all models",
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}

	type modelSelectFlags struct {
		modelName string
	}

	selectFlags := &modelSelectFlags{}

	modelSelectCmd := &cobra.Command{
		Use:   "select",
		Short: "Select a model",
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}

	modelSelectCmd.Flags().StringVarP(&selectFlags.modelName, "name", "n", "", "Model name")

	modelCmd.AddCommand(modelListCmd)
	modelCmd.AddCommand(newDeploymentCommand())

	return modelCmd
}

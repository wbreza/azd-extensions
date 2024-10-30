package cmd

import (
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/cognitiveservices/armcognitiveservices"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/wbreza/azd-extensions/extensions/ai/internal"
	"github.com/wbreza/azd-extensions/sdk/ext"
	"github.com/wbreza/azd-extensions/sdk/ext/prompt"
	"github.com/wbreza/azd-extensions/sdk/ux"
)

func newDeploymentCommand() *cobra.Command {
	deploymentCmd := &cobra.Command{
		Use:   "deployment",
		Short: "Commands for managing Azure AI model deployments",
	}

	deploymentListCmd := &cobra.Command{
		Use:   "list",
		Short: "List all deployments",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			azdContext, err := ext.CurrentContext(ctx)
			if err != nil {
				return err
			}

			aiConfig, err := internal.LoadOrPromptAiConfig(ctx, azdContext)
			if err != nil {
				return err
			}

			credential, err := azdContext.Credential()
			if err != nil {
				return err
			}

			var armClientOptions *arm.ClientOptions
			azdContext.Invoke(func(clientOptions *arm.ClientOptions) error {
				armClientOptions = clientOptions
				return nil
			})

			deployments := []*armcognitiveservices.Deployment{}

			deploymentsClient, err := armcognitiveservices.NewDeploymentsClient(aiConfig.Subscription, credential, armClientOptions)
			if err != nil {
				return err
			}

			deploymentsPager := deploymentsClient.NewListPager(aiConfig.ResourceGroup, aiConfig.Service, nil)
			for deploymentsPager.More() {
				pageResponse, err := deploymentsPager.NextPage(ctx)
				if err != nil {
					return err
				}

				deployments = append(deployments, pageResponse.Value...)
			}

			for _, deployment := range deployments {
				fmt.Printf("Name: %s\n", *deployment.Name)
				fmt.Printf("SKU: %s\n", *deployment.SKU.Name)
				fmt.Printf("Model: %s\n", *deployment.Properties.Model.Name)
				fmt.Printf("Version: %s\n", *deployment.Properties.Model.Version)
				fmt.Println()
			}

			return nil
		},
	}

	deploymentCreateCmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new model deployment",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			azdContext, err := ext.CurrentContext(ctx)
			if err != nil {
				return err
			}

			aiConfig, err := internal.LoadOrPromptAiConfig(ctx, azdContext)
			if err != nil {
				return err
			}

			modelDeployment, err := internal.PromptModelDeployment(ctx, azdContext, aiConfig, &internal.PromptModelDeploymentOptions{
				SelectorOptions: &prompt.PromptSelectOptions{
					ForceNewResource: to.Ptr(true),
				},
			})
			if err != nil {
				return err
			}

			aiConfig.Models.ChatCompletion = *modelDeployment.Name
			if err := internal.SaveAiConfig(ctx, azdContext, aiConfig); err != nil {
				return err
			}

			color.Green("Deployment '%s' created successfully", *modelDeployment.Name)

			return nil
		},
	}

	type deploymentDeleteFlags struct {
		name  string
		force bool
	}

	deleteFlags := &deploymentDeleteFlags{}

	deploymentDeleteCmd := &cobra.Command{
		Use:   "delete <deployment-name>",
		Short: "Delete a model deployment",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			azdContext, err := ext.CurrentContext(ctx)
			if err != nil {
				return err
			}

			aiConfig, err := internal.LoadOrPromptAiConfig(ctx, azdContext)
			if err != nil {
				return err
			}

			credential, err := azdContext.Credential()
			if err != nil {
				return err
			}

			var armClientOptions *arm.ClientOptions
			azdContext.Invoke(func(clientOptions *arm.ClientOptions) error {
				armClientOptions = clientOptions
				return nil
			})

			clientFactory, err := armcognitiveservices.NewClientFactory(aiConfig.Subscription, credential, armClientOptions)
			if err != nil {
				return err
			}

			deploymentsClient := clientFactory.NewDeploymentsClient()

			if deleteFlags.name == "" {
				selectedDeployment, err := internal.PromptModelDeployment(
					ctx,
					azdContext,
					aiConfig,
					&internal.PromptModelDeploymentOptions{
						SelectorOptions: &prompt.PromptSelectOptions{
							AllowNewResource: to.Ptr(false),
						},
					})
				if err != nil {
					return err
				}

				deleteFlags.name = *selectedDeployment.Name
			}

			_, err = deploymentsClient.Get(ctx, aiConfig.ResourceGroup, aiConfig.Service, deleteFlags.name, nil)
			if err != nil {
				return fmt.Errorf("deployment '%s' not found", deleteFlags.name)
			}

			confirmed := to.Ptr(false)

			if !deleteFlags.force {
				confirmPrompt := ux.NewConfirm(&ux.ConfirmConfig{
					Message:      fmt.Sprintf("Are you sure you want to delete the deployment '%s'?", deleteFlags.name),
					DefaultValue: to.Ptr(false),
				})

				confirmed, err = confirmPrompt.Ask()
				if err != nil {
					return err
				}
			}

			taskName := fmt.Sprintf("Deleting deployment %s", color.CyanString(deleteFlags.name))

			err = ux.NewTaskList(nil).
				AddTask(ux.TaskOptions{
					Title: taskName,
					Action: func() (ux.TaskState, error) {
						if !*confirmed {
							return ux.Skipped, ux.ErrCancelled
						}

						poller, err := deploymentsClient.BeginDelete(ctx, aiConfig.ResourceGroup, aiConfig.Service, deleteFlags.name, nil)
						if err != nil {
							return ux.Error, err
						}

						if _, err := poller.PollUntilDone(ctx, nil); err != nil {
							return ux.Error, err
						}

						return ux.Success, nil
					},
				}).
				Run()

			if err != nil {
				return err
			}

			color.Green("Deployment '%s' deleted successfully", deleteFlags.name)

			return nil
		},
	}

	deploymentDeleteCmd.Flags().StringVarP(&deleteFlags.name, "name", "n", "", "Name of the deployment to delete")
	deploymentDeleteCmd.Flags().BoolVarP(&deleteFlags.force, "force", "f", false, "Force deletion without confirmation")

	type deploymentSelectFlags struct {
		deploymentName string
	}

	selectFlags := &deploymentSelectFlags{}

	deploymentSelectCmd := &cobra.Command{
		Use:   "select",
		Short: "Select a model",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			azdContext, err := ext.CurrentContext(ctx)
			if err != nil {
				return err
			}

			// Load AI config
			aiConfig, err := internal.LoadOrPromptAiConfig(ctx, azdContext)
			if err != nil {
				return err
			}

			// Select model deployment
			if selectFlags.deploymentName == "" {
				selectedDeployment, err := internal.PromptModelDeployment(ctx, azdContext, aiConfig, nil)
				if err != nil {
					return err
				}

				aiConfig.Models.ChatCompletion = *selectedDeployment.Name
			} else {
				aiConfig.Models.ChatCompletion = selectFlags.deploymentName
			}

			// Update AI Config
			err = ux.NewTaskList(nil).
				AddTask(ux.TaskOptions{
					Title: "Save AI configuration",
					Action: func() (ux.TaskState, error) {
						if err := internal.SaveAiConfig(ctx, azdContext, aiConfig); err != nil {
							return ux.Error, err
						}

						return ux.Success, nil
					},
				}).
				Run()

			if err != nil {
				return err
			}

			return nil
		},
	}

	deploymentSelectCmd.Flags().StringVarP(&selectFlags.deploymentName, "name", "n", "", "Model name")

	deploymentCmd.AddCommand(deploymentListCmd)
	deploymentCmd.AddCommand(deploymentCreateCmd)
	deploymentCmd.AddCommand(deploymentDeleteCmd)
	deploymentCmd.AddCommand(deploymentSelectCmd)

	return deploymentCmd
}

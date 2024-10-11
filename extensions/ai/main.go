package main

import (
	"context"
	"os"

	"github.com/fatih/color"
	"github.com/wbreza/azd-extensions/extensions/ai/internal/cmd"
)

func main() {
	// rootCmd := &cobra.Command{
	// 	Use:   "azd ai <group> [options]",
	// 	Short: "A CLI for managing AI models and services",
	// 	PersistentPreRun: func(cmd *cobra.Command, args []string) {
	// 		debug.WaitForDebugger()
	// 	},
	// }

	// // Model command
	// modelCmd := &cobra.Command{
	// 	Use:   "model",
	// 	Short: "Commands for managing models",
	// }

	// modelListCmd := &cobra.Command{
	// 	Use:   "list",
	// 	Short: "List all models",
	// 	RunE: func(cmd *cobra.Command, args []string) error {
	// 		ctx := cmd.Context()

	// 		azdContext, err := ext.CurrentContext(ctx)
	// 		if err != nil {
	// 			return err
	// 		}

	// 		credential, err := azdContext.Credential()
	// 		if err != nil {
	// 			return err
	// 		}

	// 		var subscriptionId string
	// 		var resourceGroup string
	// 		var aiServiceName string

	// 		azdEnv, err := azdContext.Environment(ctx)
	// 		if err != nil {
	// 			subscription, err := prompt.PromptSubscription(ctx, nil)
	// 			if err != nil {
	// 				return err
	// 			}

	// 			subscriptionId = subscription.Id

	// 			aiService, err := prompt.PromptSubscriptionResource(ctx, subscription, prompt.PromptResourceOptions{
	// 				ResourceType:            to.Ptr(azure.ResourceTypeCognitiveServiceAccount),
	// 				ResourceTypeDisplayName: "Azure AI service",
	// 			})
	// 			if err != nil {
	// 				return err
	// 			}

	// 			parsedService, err := arm.ParseResourceID(aiService.Id)
	// 			if err != nil {
	// 				return err
	// 			}

	// 			resourceGroup = parsedService.ResourceGroupName
	// 			aiServiceName = parsedService.Name
	// 		} else {
	// 			if subscriptionVal, has := azdEnv.Config.GetString("ai.subscription"); has && subscriptionVal != "" {
	// 				subscriptionId = subscriptionVal
	// 			} else {
	// 				subscription, err := prompt.PromptSubscription(ctx, nil)
	// 				if err != nil {
	// 					return err
	// 				}

	// 				subscriptionId = subscription.Id
	// 			}

	// 			if aiServiceVal, has := azdEnv.Config.GetString("ai.service"); has && aiServiceVal != "" {
	// 				aiServiceName = aiServiceVal
	// 			} else {
	// 				principal, err := azdContext.Principal(ctx)
	// 				if err != nil {
	// 					return err
	// 				}

	// 				subscription := &azure.Subscription{
	// 					Id:       subscriptionId,
	// 					TenantId: principal.TenantId,
	// 				}
	// 				aiService, err := prompt.PromptSubscriptionResource(ctx, subscription, prompt.PromptResourceOptions{
	// 					ResourceType:            to.Ptr(azure.ResourceTypeCognitiveServiceAccount),
	// 					ResourceTypeDisplayName: "Azure AI service",
	// 				})
	// 				if err != nil {
	// 					return err
	// 				}

	// 				parsedService, err := arm.ParseResourceID(aiService.Id)
	// 				if err != nil {
	// 					return err
	// 				}

	// 				resourceGroup = parsedService.ResourceGroupName
	// 				aiServiceName = parsedService.Name
	// 			}

	// 			if resourceGroupVal, has := azdEnv.Config.GetString("ai.resourceGroup"); has && resourceGroupVal != "" {
	// 				resourceGroup = resourceGroupVal
	// 			}
	// 		}

	// 		if azdEnv != nil {
	// 			azdEnv.Config.Set("ai.subscription", subscriptionId)
	// 			azdEnv.Config.Set("ai.resourceGroup", resourceGroup)
	// 			azdEnv.Config.Set("ai.service", aiServiceName)

	// 			if err := azdContext.SaveEnvironment(ctx, azdEnv); err != nil {
	// 				return err
	// 			}
	// 		}

	// 		deployments := []*armcognitiveservices.Deployment{}

	// 		deploymentsClient, err := armcognitiveservices.NewDeploymentsClient(subscriptionId, credential, nil)
	// 		if err != nil {
	// 			return err
	// 		}

	// 		deploymentsPager := deploymentsClient.NewListPager(resourceGroup, aiServiceName, nil)
	// 		for deploymentsPager.More() {
	// 			pageResponse, err := deploymentsPager.NextPage(ctx)
	// 			if err != nil {
	// 				return err
	// 			}

	// 			deployments = append(deployments, pageResponse.Value...)
	// 		}

	// 		for _, deployment := range deployments {
	// 			fmt.Printf("Name: %s\n", *deployment.Name)
	// 			fmt.Printf("SKU: %s\n", *deployment.SKU.Name)
	// 			fmt.Printf("Model: %s\n", *deployment.Properties.Model.Name)
	// 			fmt.Printf("Version: %s\n", *deployment.Properties.Model.Version)
	// 			fmt.Println()
	// 		}

	// 		return nil
	// 	},
	// }

	// modelDeployCmd := &cobra.Command{
	// 	Use:   "deploy",
	// 	Short: "Deploy a model",
	// 	Run: func(cmd *cobra.Command, args []string) {
	// 		fmt.Println("Executing model deploy command")
	// 	},
	// }

	// // Add subcommands to model command
	// modelCmd.AddCommand(modelListCmd)
	// modelCmd.AddCommand(modelDeployCmd)

	// // Service command
	// serviceCmd := &cobra.Command{
	// 	Use:   "service",
	// 	Short: "Commands for managing services",
	// }

	// serviceListCmd := &cobra.Command{
	// 	Use:   "list",
	// 	Short: "List all services",
	// 	Run: func(cmd *cobra.Command, args []string) {
	// 		fmt.Println("Executing service list command")
	// 	},
	// }

	// serviceDeployCmd := &cobra.Command{
	// 	Use:   "deploy",
	// 	Short: "Deploy a service",
	// 	Run: func(cmd *cobra.Command, args []string) {
	// 		fmt.Println("Executing service deploy command")
	// 	},
	// }

	// // Add subcommands to service command
	// serviceCmd.AddCommand(serviceListCmd)
	// serviceCmd.AddCommand(serviceDeployCmd)

	// // Add model and service commands to root
	// rootCmd.AddCommand(modelCmd)
	// rootCmd.AddCommand(serviceCmd)

	// Execute the root command
	ctx := context.Background()
	rootCmd := cmd.NewRootCommand()

	if err := rootCmd.ExecuteContext(ctx); err != nil {
		color.Red("Error: %v", err)
		os.Exit(1)
	}
}

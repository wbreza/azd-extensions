package cmd

import (
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/spf13/cobra"
	"github.com/wbreza/azd-extensions/extensions/ai/internal"
	"github.com/wbreza/azd-extensions/sdk/ext"
)

type serviceSetFlags struct {
	subscription  string
	resourceGroup string
	serviceName   string
	modelName     string
}

func newServiceCommand() *cobra.Command {
	serviceCmd := &cobra.Command{
		Use:   "service",
		Short: "Commands for managing Azure AI services",
	}

	setFlags := &serviceSetFlags{}

	serviceSetCmd := &cobra.Command{
		Use:   "set",
		Short: "Set the default Azure AI service",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			azdContext, err := ext.CurrentContext(ctx)
			if err != nil {
				return err
			}

			azureContext, err := azdContext.AzureContext(ctx)
			if err != nil {
				return err
			}

			var aiConfig *internal.ExtensionConfig

			if setFlags.subscription == "" || setFlags.resourceGroup == "" || setFlags.serviceName == "" {
				selectedAccount, err := internal.PromptAIServiceAccount(ctx, azdContext, azureContext)
				if err != nil {
					return err
				}

				parsedResource, err := arm.ParseResourceID(*selectedAccount.ID)
				if err != nil {
					return err
				}

				aiConfig = &internal.ExtensionConfig{
					Subscription:  parsedResource.SubscriptionID,
					ResourceGroup: parsedResource.ResourceGroupName,
					Ai: internal.AiConfig{
						Service:  *selectedAccount.Name,
						Endpoint: *selectedAccount.Properties.Endpoint,
					},
				}
			} else {
				aiConfig = &internal.ExtensionConfig{
					Subscription:  setFlags.subscription,
					ResourceGroup: setFlags.resourceGroup,
					Ai: internal.AiConfig{
						Service: setFlags.serviceName,
					},
				}
			}

			if err := internal.SaveExtensionConfig(ctx, azdContext, aiConfig); err != nil {
				return err
			}

			return nil
		},
	}

	serviceSetCmd.Flags().StringVarP(&setFlags.subscription, "subscription", "s", "", "Azure subscription ID")
	serviceSetCmd.Flags().StringVarP(&setFlags.resourceGroup, "resource-group", "g", "", "Azure resource group")
	serviceSetCmd.Flags().StringVarP(&setFlags.serviceName, "name", "n", "", "Azure AI service name")

	serviceShowCmd := &cobra.Command{
		Use:   "show",
		Short: "Show the currently selected Azure AI service",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			azdContext, err := ext.CurrentContext(ctx)
			if err != nil {
				return err
			}

			aiConfig, err := internal.LoadExtensionConfig(ctx, azdContext)
			if err != nil {
				return err
			}

			fmt.Printf("Service: %s\n", aiConfig.Ai.Service)
			fmt.Printf("Resource Group: %s\n", aiConfig.ResourceGroup)
			fmt.Printf("Subscription ID: %s\n", aiConfig.Subscription)

			return nil
		},
	}

	serviceCmd.AddCommand(serviceSetCmd)
	serviceCmd.AddCommand(serviceShowCmd)

	return serviceCmd
}

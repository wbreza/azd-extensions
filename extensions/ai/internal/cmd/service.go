package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/wbreza/azd-extensions/extensions/ai/internal/service"
	"github.com/wbreza/azd-extensions/sdk/ext"
)

type serviceSetFlags struct {
	subscription  string
	resourceGroup string
	serviceName   string
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

			var serviceConfig *service.ServiceConfig

			if setFlags.subscription == "" || setFlags.resourceGroup == "" || setFlags.serviceName == "" {
				var err error
				serviceConfig, err = service.Prompt(ctx, azdContext)
				if err != nil {
					return err
				}
			} else {
				serviceConfig = &service.ServiceConfig{
					Subscription:  setFlags.subscription,
					ResourceGroup: setFlags.resourceGroup,
					Service:       setFlags.serviceName,
				}
			}

			if err := service.Save(ctx, azdContext, serviceConfig); err != nil {
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

			serviceConfig, err := service.Load(ctx, azdContext)
			if err != nil {
				return err
			}

			fmt.Printf("Service: %s\n", serviceConfig.Service)
			fmt.Printf("Resource Group: %s\n", serviceConfig.ResourceGroup)
			fmt.Printf("Subscription ID: %s\n", serviceConfig.Subscription)

			return nil
		},
	}

	serviceCmd.AddCommand(serviceSetCmd)
	serviceCmd.AddCommand(serviceShowCmd)

	return serviceCmd
}

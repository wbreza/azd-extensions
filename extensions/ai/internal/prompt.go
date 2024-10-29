package internal

import (
	"context"
	"errors"
	"fmt"

	"dario.cat/mergo"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/cognitiveservices/armcognitiveservices"
	"github.com/wbreza/azd-extensions/sdk/azure"
	"github.com/wbreza/azd-extensions/sdk/ext"
	"github.com/wbreza/azd-extensions/sdk/ext/prompt"
	"github.com/wbreza/azd-extensions/sdk/ux"
)

func PromptAccount(ctx context.Context, azdContext *ext.Context) (*armcognitiveservices.Account, error) {
	subscription, err := prompt.PromptSubscription(ctx, nil)
	if err != nil {
		return nil, err
	}

	credential, err := azdContext.Credential()
	if err != nil {
		return nil, err
	}

	selectedAccount, err := prompt.PromptSubscriptionResource(ctx, subscription, prompt.PromptResourceOptions{
		ResourceType:            to.Ptr(azure.ResourceTypeCognitiveServiceAccount),
		Kinds:                   []string{"OpenAI", "AIService", "CognitiveServices"},
		ResourceTypeDisplayName: "Azure AI service",
		CreateResource: func(ctx context.Context) (*azure.ResourceExtended, error) {
			resourceGroup, err := prompt.PromptResourceGroup(ctx, subscription, nil)
			if err != nil {
				return nil, err
			}

			namePrompt := ux.NewPrompt(&ux.PromptConfig{
				Message: "Enter the name for the Azure AI service account",
			})

			accountName, err := namePrompt.Ask()
			if err != nil {
				return nil, err
			}

			spinner := ux.NewSpinner(&ux.SpinnerConfig{
				Text: "Creating Azure AI service account...",
			})

			var aiService *azure.ResourceExtended

			err = spinner.Run(ctx, func(ctx context.Context) error {
				accountsClient, err := armcognitiveservices.NewAccountsClient(subscription.Id, credential, nil)
				if err != nil {
					return err
				}

				account := armcognitiveservices.Account{
					Name: &accountName,
					Identity: &armcognitiveservices.Identity{
						Type: to.Ptr(armcognitiveservices.ResourceIdentityTypeSystemAssigned),
					},
					Location: &resourceGroup.Location,
					Kind:     to.Ptr("OpenAI"),
					SKU: &armcognitiveservices.SKU{
						Name: to.Ptr("S0"),
					},
					Properties: &armcognitiveservices.AccountProperties{
						CustomSubDomainName: &accountName,
						PublicNetworkAccess: to.Ptr(armcognitiveservices.PublicNetworkAccessEnabled),
						DisableLocalAuth:    to.Ptr(false),
					},
				}

				poller, err := accountsClient.BeginCreate(ctx, resourceGroup.Name, accountName, account, nil)
				if err != nil {
					return err
				}

				accountResponse, err := poller.PollUntilDone(ctx, nil)
				if err != nil {
					return err
				}

				aiService = &azure.ResourceExtended{
					Resource: azure.Resource{
						Id:       *accountResponse.ID,
						Name:     *accountResponse.Name,
						Type:     *accountResponse.Type,
						Location: *accountResponse.Location,
					},
					Kind: *accountResponse.Kind,
				}

				return nil
			})

			if err != nil {
				return nil, err
			}

			return aiService, nil
		},
	})
	if err != nil {
		return nil, err
	}

	parsedService, err := arm.ParseResourceID(selectedAccount.Id)
	if err != nil {
		return nil, err
	}

	accountClient, err := armcognitiveservices.NewAccountsClient(subscription.Id, credential, nil)
	if err != nil {
		return nil, err
	}

	accountResponse, err := accountClient.Get(ctx, parsedService.ResourceGroupName, parsedService.Name, nil)
	if err != nil {
		return nil, err
	}

	return &accountResponse.Account, nil
}

func PromptModelDeployment(ctx context.Context, azdContext *ext.Context, aiConfig *AiConfig, options *prompt.PromptSelectOptions) (*armcognitiveservices.Deployment, error) {
	mergedOptions := &prompt.PromptSelectOptions{}

	if options == nil {
		options = &prompt.PromptSelectOptions{}
	}

	defaultOptions := &prompt.PromptSelectOptions{
		Message:            "Select an AI model deployment",
		LoadingMessage:     "Loading model deployments...",
		AllowNewResource:   to.Ptr(true),
		NewResourceMessage: "Deploy a new AI model",
		CreatingMessage:    "Deploying AI model...",
	}

	mergo.Merge(mergedOptions, options, mergo.WithoutDereference)
	mergo.Merge(mergedOptions, defaultOptions, mergo.WithoutDereference)

	credential, err := azdContext.Credential()
	if err != nil {
		return nil, err
	}

	deploymentsClient, err := armcognitiveservices.NewDeploymentsClient(aiConfig.Subscription, credential, nil)
	if err != nil {
		return nil, err
	}

	return prompt.PromptCustomResource(ctx, prompt.PromptCustomResourceOptions[armcognitiveservices.Deployment]{
		SelectorOptions: mergedOptions,
		LoadData: func(ctx context.Context) ([]*armcognitiveservices.Deployment, error) {
			deploymentList := []*armcognitiveservices.Deployment{}
			modelPager := deploymentsClient.NewListPager(aiConfig.ResourceGroup, aiConfig.Service, nil)
			for modelPager.More() {
				models, err := modelPager.NextPage(ctx)
				if err != nil {
					return nil, err
				}

				deploymentList = append(deploymentList, models.Value...)
			}

			return deploymentList, nil
		},
		DisplayResource: func(resource *armcognitiveservices.Deployment) (string, error) {
			return fmt.Sprintf("%s (Model: %s, Version: %s)", *resource.Name, *resource.Properties.Model.Name, *resource.Properties.Model.Version), nil
		},
		CreateResource: func(ctx context.Context) (*armcognitiveservices.Deployment, error) {
			selectedModel, err := PromptModel(ctx, azdContext, aiConfig)
			if err != nil {
				return nil, err
			}

			selectedSku, err := PromptModelSku(ctx, azdContext, aiConfig, selectedModel)
			if err != nil {
				return nil, err
			}
			var deploymentName string

			namePrompt := ux.NewPrompt(&ux.PromptConfig{
				Message:      "Enter the name for the deployment",
				DefaultValue: *selectedModel.Model.Name,
			})

			deploymentName, err = namePrompt.Ask()
			if err != nil {
				return nil, err
			}

			deployment := armcognitiveservices.Deployment{
				Name: &deploymentName,
				SKU: &armcognitiveservices.SKU{
					Name:     selectedSku.Name,
					Capacity: selectedSku.Capacity.Default,
				},
				Properties: &armcognitiveservices.DeploymentProperties{
					Model: &armcognitiveservices.DeploymentModel{
						Format:  selectedModel.Model.Format,
						Name:    selectedModel.Model.Name,
						Version: selectedModel.Model.Version,
					},
					RaiPolicyName:        to.Ptr("Microsoft.DefaultV2"),
					VersionUpgradeOption: to.Ptr(armcognitiveservices.DeploymentModelVersionUpgradeOptionOnceNewDefaultVersionAvailable),
				},
			}

			spinner := ux.NewSpinner(&ux.SpinnerConfig{
				Text: "Deploying AI model...",
			})

			var modelDeployment *armcognitiveservices.Deployment

			err = spinner.Run(ctx, func(ctx context.Context) error {
				existingDeployment, err := deploymentsClient.Get(ctx, aiConfig.ResourceGroup, aiConfig.Service, deploymentName, nil)
				if err == nil && *existingDeployment.Name == deploymentName {
					return errors.New("deployment with the same name already exists")
				}

				poller, err := deploymentsClient.BeginCreateOrUpdate(ctx, aiConfig.ResourceGroup, aiConfig.Service, deploymentName, deployment, nil)
				if err != nil {
					return err
				}

				deploymentResponse, err := poller.PollUntilDone(ctx, nil)
				if err != nil {
					return err
				}

				modelDeployment = &deploymentResponse.Deployment
				return nil
			})

			if err != nil {
				return nil, err
			}

			return modelDeployment, nil
		},
	})
}

func PromptModel(ctx context.Context, azdContext *ext.Context, aiConfig *AiConfig) (*armcognitiveservices.Model, error) {
	return prompt.PromptCustomResource(ctx, prompt.PromptCustomResourceOptions[armcognitiveservices.Model]{
		SelectorOptions: &prompt.PromptSelectOptions{
			Message:          "Select a model",
			LoadingMessage:   "Loading models...",
			AllowNewResource: to.Ptr(false),
		},
		LoadData: func(ctx context.Context) ([]*armcognitiveservices.Model, error) {
			credential, err := azdContext.Credential()
			if err != nil {
				return nil, err
			}

			clientFactory, err := armcognitiveservices.NewClientFactory(aiConfig.Subscription, credential, nil)
			if err != nil {
				return nil, err
			}

			accountsClient := clientFactory.NewAccountsClient()
			modelsClient := clientFactory.NewModelsClient()

			aiService, err := accountsClient.Get(ctx, aiConfig.ResourceGroup, aiConfig.Service, nil)
			if err != nil {
				return nil, err
			}

			models := []*armcognitiveservices.Model{}

			modelPager := modelsClient.NewListPager(*aiService.Location, nil)
			for modelPager.More() {
				pageResponse, err := modelPager.NextPage(ctx)
				if err != nil {
					return nil, err
				}

				for _, model := range pageResponse.Value {
					if *model.Kind == *aiService.Kind {
						models = append(models, model)
					}
				}
			}

			return models, nil
		},
		DisplayResource: func(model *armcognitiveservices.Model) (string, error) {
			return fmt.Sprintf("%s (Version: %s)", *model.Model.Name, *model.Model.Version), nil
		},
	})
}

func PromptModelSku(ctx context.Context, azdContext *ext.Context, aiConfig *AiConfig, model *armcognitiveservices.Model) (*armcognitiveservices.ModelSKU, error) {
	return prompt.PromptCustomResource(ctx, prompt.PromptCustomResourceOptions[armcognitiveservices.ModelSKU]{
		SelectorOptions: &prompt.PromptSelectOptions{
			Message:          "Select a deployment type",
			LoadingMessage:   "Loading deployment types...",
			AllowNewResource: to.Ptr(false),
		},
		LoadData: func(ctx context.Context) ([]*armcognitiveservices.ModelSKU, error) {
			return model.Model.SKUs, nil
		},
		DisplayResource: func(sku *armcognitiveservices.ModelSKU) (string, error) {
			return *sku.Name, nil
		},
	})
}

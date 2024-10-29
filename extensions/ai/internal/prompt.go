package internal

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/cognitiveservices/armcognitiveservices"
	"github.com/wbreza/azd-extensions/sdk/azure"
	"github.com/wbreza/azd-extensions/sdk/ext"
	"github.com/wbreza/azd-extensions/sdk/ext/prompt"
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

func PromptModelDeployment(ctx context.Context, azdContext *ext.Context, aiConfig *AiConfig) (*armcognitiveservices.Deployment, error) {
	return prompt.PromptCustomResource(ctx, prompt.PromptCustomResourceOptions[armcognitiveservices.Deployment]{
		SelectorOptions: &prompt.PromptSelectOptions{
			Message:        "Select a model deployment",
			LoadingMessage: "Loading model deployments...",
		},
		LoadData: func(ctx context.Context) ([]*armcognitiveservices.Deployment, error) {
			credential, err := azdContext.Credential()
			if err != nil {
				return nil, err
			}

			deploymentsClient, err := armcognitiveservices.NewDeploymentsClient(aiConfig.Subscription, credential, nil)
			if err != nil {
				return nil, err
			}

			deploymentList := []*armcognitiveservices.Deployment{}
			modelPager := deploymentsClient.NewListPager(aiConfig.ResourceGroup, aiConfig.Service, nil)
			for modelPager.More() {
				models, err := modelPager.NextPage(ctx)
				if err != nil {
					return nil, err
				}

				deploymentList = append(deploymentList, models.Value...)
			}

			if len(deploymentList) == 0 {
				return nil, ErrNoModelDeployments
			}

			return deploymentList, nil
		},
		DisplayResource: func(resource *armcognitiveservices.Deployment) (string, error) {
			return fmt.Sprintf("%s (Model: %s, Version: %s)", *resource.Name, *resource.Properties.Model.Name, *resource.Properties.Model.Version), nil
		},
	})
}

func PromptModel(ctx context.Context, azdContext *ext.Context, aiConfig *AiConfig) (*armcognitiveservices.Model, error) {
	return prompt.PromptCustomResource(ctx, prompt.PromptCustomResourceOptions[armcognitiveservices.Model]{
		SelectorOptions: &prompt.PromptSelectOptions{
			Message: "Select a model",
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
			Message:        "Select a deployment type",
			LoadingMessage: "Loading deployment types...",
		},
		LoadData: func(ctx context.Context) ([]*armcognitiveservices.ModelSKU, error) {
			return model.Model.SKUs, nil
		},
		DisplayResource: func(sku *armcognitiveservices.ModelSKU) (string, error) {
			return *sku.Name, nil
		},
	})
}

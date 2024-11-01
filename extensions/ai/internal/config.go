package internal

import (
	"context"
	"errors"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/wbreza/azd-extensions/sdk/azure"
	"github.com/wbreza/azd-extensions/sdk/core/config"
	"github.com/wbreza/azd-extensions/sdk/ext"
	"github.com/wbreza/azd-extensions/sdk/ext/prompt"
)

type AiConfig struct {
	Subscription  string        `json:"subscription"`
	ResourceGroup string        `json:"resourceGroup"`
	Service       string        `json:"service"`
	Models        ModelsConfig  `json:"models"`
	Search        SearchConfig  `json:"search"`
	Storage       StorageConfig `json:"storage"`
}

type StorageConfig struct {
	Account   string `json:"account"`
	Container string `json:"container"`
}

type ModelsConfig struct {
	ChatCompletion string `json:"chatCompletion"`
	Embeddings     string `json:"embeddings"`
	Audio          string `json:"audio"`
}

type SearchConfig struct {
	Service string `json:"service"`
	Index   string `json:"index"`
}

var (
	ErrNotFound           = errors.New("service config not found")
	ErrNoModelDeployments = errors.New("no model deployments found")
	ErrNoAiServices       = errors.New("no Azure AI services found")
)

func LoadOrPromptSubscription(ctx context.Context, azdContext *ext.Context, aiConfig *AiConfig) (*azure.Subscription, error) {
	var subscription *azure.Subscription

	if aiConfig == nil || aiConfig.Subscription == "" {
		selectedSubscription, err := prompt.PromptSubscription(ctx, nil)
		if err != nil {
			return nil, err
		}

		subscription = selectedSubscription
	} else {
		credential, err := azdContext.Credential()
		if err != nil {
			return nil, err
		}

		principal, err := azdContext.Principal(ctx)
		if err != nil {
			return nil, err
		}

		subscriptionService := azure.NewSubscriptionsService(credential, nil)
		existingSubscription, err := subscriptionService.GetSubscription(ctx, aiConfig.Subscription, principal.TenantId)
		if err != nil {
			return nil, err
		}

		subscription = existingSubscription
	}

	return subscription, nil
}

func LoadOrPromptResourceGroup(ctx context.Context, azdContext *ext.Context, aiConfig *AiConfig) (*azure.ResourceGroup, error) {
	subscription, err := LoadOrPromptSubscription(ctx, azdContext, aiConfig)
	if err != nil {
		return nil, err
	}

	var resourceGroup *azure.ResourceGroup

	if aiConfig == nil || aiConfig.ResourceGroup == "" {
		selectedResourceGroup, err := prompt.PromptResourceGroup(ctx, subscription, nil)
		if err != nil {
			return nil, err
		}

		resourceGroup = selectedResourceGroup
	} else {
		credential, err := azdContext.Credential()
		if err != nil {
			return nil, err
		}

		resourceGroupService := azure.NewResourceService(credential, nil)
		existingResourceGroup, err := resourceGroupService.GetResourceGroup(ctx, aiConfig.Subscription, aiConfig.ResourceGroup)
		if err != nil {
			return nil, err
		}

		resourceGroup = existingResourceGroup
	}

	return resourceGroup, nil
}

func LoadOrPromptAiConfig(ctx context.Context, azdContext *ext.Context) (*AiConfig, error) {
	config, err := LoadAiConfig(ctx, azdContext)
	if err != nil && errors.Is(err, ErrNotFound) {
		account, err := PromptAIServiceAccount(ctx, azdContext, nil)
		if err != nil {
			return nil, err
		}

		parsedResource, err := arm.ParseResourceID(*account.ID)
		if err != nil {
			return nil, err
		}

		config = &AiConfig{
			Subscription:  parsedResource.SubscriptionID,
			ResourceGroup: parsedResource.ResourceGroupName,
			Service:       parsedResource.Name,
		}

		if err := SaveAiConfig(ctx, azdContext, config); err != nil {
			return nil, err
		}
	}

	return config, nil
}

func LoadAiConfig(ctx context.Context, azdContext *ext.Context) (*AiConfig, error) {
	var azdConfig config.Config

	env, err := azdContext.Environment(ctx)
	if err == nil {
		azdConfig = env.Config
	} else {
		userConfig, err := azdContext.UserConfig(ctx)
		if err == nil {
			azdConfig = userConfig
		}
	}

	if azdConfig == nil {
		return nil, errors.New("azd configuration is not available")
	}

	var config AiConfig
	has, err := azdConfig.GetSection("ai.config", &config)
	if err != nil {
		return nil, err
	}

	if has {
		return &config, nil
	}

	return nil, ErrNotFound
}

func SaveAiConfig(ctx context.Context, azdContext *ext.Context, config *AiConfig) error {
	if azdContext == nil {
		return errors.New("azdContext is required")
	}

	if config == nil {
		return errors.New("config is required")
	}

	env, err := azdContext.Environment(ctx)
	if err == nil && env != nil {
		if err := env.Config.Set("ai.config", config); err != nil {
			return err
		}

		if err := azdContext.SaveEnvironment(ctx, env); err != nil {
			return err
		}

		return nil
	}

	userConfig, err := azdContext.UserConfig(ctx)
	if err == nil && userConfig != nil {
		if err := userConfig.Set("ai.config", config); err != nil {
			return err
		}

		if err := azdContext.SaveUserConfig(ctx, userConfig); err != nil {
			return err
		}

		return nil
	}

	return errors.New("unable to save service configuration")
}

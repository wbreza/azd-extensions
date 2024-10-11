package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/cognitiveservices/armcognitiveservices"
	"github.com/wbreza/azd-extensions/sdk/azure"
	"github.com/wbreza/azd-extensions/sdk/core/config"
	"github.com/wbreza/azd-extensions/sdk/ext"
	"github.com/wbreza/azd-extensions/sdk/ext/prompt"
)

type ServiceConfig struct {
	Subscription  string `json:"subscription"`
	ResourceGroup string `json:"resourceGroup"`
	Service       string `json:"service"`
}

var (
	ErrNotFound = errors.New("service config not found")
)

func LoadOrPrompt(ctx context.Context, azdContext *ext.Context) (*ServiceConfig, error) {
	config, err := Load(ctx, azdContext)
	if err != nil && errors.Is(err, ErrNotFound) {
		config, err = Prompt(ctx, azdContext)
		if err != nil {
			return nil, err
		}

		if err := Save(ctx, azdContext, config); err != nil {
			return nil, err
		}
	}

	return config, nil
}

func Load(ctx context.Context, azdContext *ext.Context) (*ServiceConfig, error) {
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

	var config ServiceConfig
	has, err := azdConfig.GetSection("ai.config", &config)
	if err != nil {
		return nil, err
	}

	if has {
		return &config, nil
	}

	return nil, ErrNotFound
}

func Save(ctx context.Context, azdContext *ext.Context, config *ServiceConfig) error {
	if azdContext == nil {
		return errors.New("azdContext is required")
	}

	if config == nil {
		return errors.New("config is required")
	}

	credential, err := azdContext.Credential()
	if err != nil {
		return err
	}

	accountClient, err := armcognitiveservices.NewAccountsClient(config.Subscription, credential, nil)
	if err != nil {
		return err
	}

	_, err = accountClient.Get(ctx, config.ResourceGroup, config.Service, nil)
	if err != nil {
		return fmt.Errorf("the specified service configuration is invalid: %w", err)
	}

	env, err := azdContext.Environment(ctx)
	if err == nil {
		if err := env.Config.Set("ai.config", config); err != nil {
			return err
		}

		if err := azdContext.SaveEnvironment(ctx, env); err != nil {
			return err
		}

		return nil
	}

	userConfig, err := azdContext.UserConfig(ctx)
	if err == nil {
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

func Prompt(ctx context.Context, azdContext *ext.Context) (*ServiceConfig, error) {
	subscription, err := prompt.PromptSubscription(ctx, nil)
	if err != nil {
		return nil, err
	}

	service, err := prompt.PromptSubscriptionResource(ctx, subscription, prompt.PromptResourceOptions{
		ResourceType:            to.Ptr(azure.ResourceTypeCognitiveServiceAccount),
		ResourceTypeDisplayName: "Azure AI service",
	})
	if err != nil {
		return nil, err
	}

	parsedService, err := arm.ParseResourceID(service.Id)
	if err != nil {
		return nil, err
	}

	config := &ServiceConfig{
		Subscription:  subscription.Id,
		ResourceGroup: parsedService.ResourceGroupName,
		Service:       parsedService.Name,
	}

	return config, nil
}

package internal

import (
	"context"
	"errors"

	"github.com/wbreza/azd-extensions/sdk/core/config"
	"github.com/wbreza/azd-extensions/sdk/ext"
)

type ExtensionConfig struct {
	Subscription  string        `json:"subscription"`
	ResourceGroup string        `json:"resourceGroup"`
	Ai            AiConfig      `json:"ai"`
	Search        SearchConfig  `json:"search"`
	Storage       StorageConfig `json:"storage"`
}

type AiConfig struct {
	Service  string       `json:"service"`
	Endpoint string       `json:"endpoint"`
	Models   ModelsConfig `json:"models"`
}

type StorageConfig struct {
	Account   string `json:"account"`
	Endpoint  string `json:"endpoint"`
	Container string `json:"container"`
}

type ModelsConfig struct {
	ChatCompletion string `json:"chatCompletion"`
	Embeddings     string `json:"embeddings"`
	Audio          string `json:"audio"`
}

type SearchConfig struct {
	Service  string `json:"service"`
	Endpoint string `json:"endpoint"`
	Index    string `json:"index"`
}

var (
	ErrNotFound           = errors.New("service config not found")
	ErrNoModelDeployments = errors.New("no model deployments found")
	ErrNoAiServices       = errors.New("no Azure AI services found")
)

func LoadExtensionConfig(ctx context.Context, azdContext *ext.Context) (*ExtensionConfig, error) {
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

	azureContext, err := azdContext.AzureContext(ctx)
	if err != nil {
		return nil, err
	}

	if azdConfig == nil {
		return nil, errors.New("azd configuration is not available")
	}

	var config ExtensionConfig
	has, err := azdConfig.GetSection("ai.config", &config)
	if err != nil {
		return nil, err
	}

	if has {
		if config.Subscription == "" {
			config.Subscription = azureContext.Scope.SubscriptionId
		}

		if config.ResourceGroup == "" {
			config.ResourceGroup = azureContext.Scope.ResourceGroup
		}

		return &config, nil
	}

	return nil, ErrNotFound
}

func SaveExtensionConfig(ctx context.Context, azdContext *ext.Context, config *ExtensionConfig) error {
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

	azureContext, err := azdContext.AzureContext(ctx)
	if err != nil {
		return err
	}

	if config.Subscription == "" && azureContext.Scope.SubscriptionId != "" {
		config.Subscription = azureContext.Scope.SubscriptionId
	}

	if config.ResourceGroup == "" && azureContext.Scope.ResourceGroup != "" {
		config.ResourceGroup = azureContext.Scope.ResourceGroup
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

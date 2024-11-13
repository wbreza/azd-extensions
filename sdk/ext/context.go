package ext

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/wbreza/azd-extensions/sdk/azure"
	"github.com/wbreza/azd-extensions/sdk/common/ioc"
	"github.com/wbreza/azd-extensions/sdk/core/azd"
	"github.com/wbreza/azd-extensions/sdk/core/config"
	"github.com/wbreza/azd-extensions/sdk/core/contracts"
	"github.com/wbreza/azd-extensions/sdk/core/environment"
	"github.com/wbreza/azd-extensions/sdk/core/project"
	"github.com/wbreza/azd-extensions/sdk/ext/account"
)

var (
	ErrProjectNotFound     = errors.New("azd project not found in current path")
	ErrEnvironmentNotFound = errors.New("azd environment not found")
	ErrUserConfigNotFound  = errors.New("azd user config not found")
	ErrPrincipalNotFound   = errors.New("azd principal not found")
	ErrNotLoggedIn         = errors.New("azd credential not available")
)

var current *Context

type Context struct {
	container    *ioc.NestedContainer
	project      *project.ProjectConfig
	environment  *environment.Environment
	userConfig   config.UserConfig
	credential   azcore.TokenCredential
	principal    *account.Principal
	deployment   *azure.ResourceDeployment
	azureContext *AzureContext
}

func CurrentContext(ctx context.Context) (*Context, error) {
	if current == nil {
		container := ioc.NewNestedContainer(nil)
		registerComponents(ctx, container)

		current = &Context{
			container: container,
		}
	}

	return current, nil
}

func (c *Context) Project(ctx context.Context) (*project.ProjectConfig, error) {
	if c.project == nil {
		var project *project.ProjectConfig
		if err := c.container.Resolve(&project); err != nil {
			return nil, fmt.Errorf("%w, Details: %w", ErrProjectNotFound, err)
		}

		c.project = project
	}

	return c.project, nil
}

func (c *Context) Environment(ctx context.Context) (*environment.Environment, error) {
	if c.environment == nil {
		var env *environment.Environment
		if err := c.container.Resolve(&env); err != nil {
			return nil, fmt.Errorf("%w, Details: %w", ErrEnvironmentNotFound, err)
		}

		c.environment = env
	}

	return c.environment, nil
}

func (c *Context) UserConfig(ctx context.Context) (config.UserConfig, error) {
	if c.userConfig == nil {
		var userConfig config.UserConfig
		if err := c.container.Resolve(&userConfig); err != nil {
			return nil, fmt.Errorf("%w, Details: %w", ErrUserConfigNotFound, err)
		}

		c.userConfig = userConfig
	}

	return c.userConfig, nil
}

func (c *Context) Credential() (azcore.TokenCredential, error) {
	if c.credential == nil {
		azdCredential, err := azidentity.NewAzureDeveloperCLICredential(nil)
		if err != nil {
			return nil, fmt.Errorf("%w, Details: %w", ErrNotLoggedIn, err)
		}

		c.credential = azdCredential
	}

	return c.credential, nil
}

func (c *Context) Principal(ctx context.Context) (*account.Principal, error) {
	if c.principal == nil {
		credential, err := c.Credential()
		if err != nil {
			return nil, fmt.Errorf("%w, Details: %w", ErrPrincipalNotFound, err)
		}

		accessToken, err := credential.GetToken(ctx, policy.TokenRequestOptions{
			Scopes: []string{"https://management.azure.com/.default"},
		})
		if err != nil {
			return nil, err
		}

		claims, err := azure.GetClaimsFromAccessToken(accessToken.Token)
		if err != nil {
			return nil, err
		}

		principal := account.Principal(claims)
		c.principal = &principal
	}

	return c.principal, nil
}

func (c *Context) Deployment(ctx context.Context) (*azure.ResourceDeployment, error) {
	if c.deployment == nil {
		err := c.container.Invoke(func(env *environment.Environment, deploymentService *azure.StackDeployments) error {
			if env == nil {
				return ErrEnvironmentNotFound
			}

			deploymentName := fmt.Sprintf("azd-stack-%s", env.Name())
			deployment, err := deploymentService.GetSubscriptionDeployment(ctx, env.GetSubscriptionId(), deploymentName)
			if err != nil {
				return err
			}

			c.deployment = deployment
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	return c.deployment, nil
}

func (c *Context) AzureContext(ctx context.Context) (*AzureContext, error) {
	if c.azureContext == nil {
		azdEnv, err := c.Environment(ctx)
		if err != nil {
			if errors.Is(err, ErrEnvironmentNotFound) {
				return NewEmptyAzureContext(), nil
			}

			return nil, err
		}

		scope := AzureScope{}
		resources := AzureResourceList{}

		if azdEnv != nil {
			subscriptionId := azdEnv.GetSubscriptionId()
			resourceGroup := azdEnv.Getenv(environment.ResourceGroupEnvVarName)
			location := azdEnv.Getenv(environment.LocationEnvVarName)

			scope.SubscriptionId = subscriptionId
			scope.ResourceGroup = resourceGroup
			scope.Location = location
		}

		deployment, err := c.Deployment(ctx)
		if err != nil {
			if errors.Is(err, azure.ErrDeploymentNotFound) {
				return NewAzureContext(scope, &resources), nil
			}

			return nil, err
		}

		if deployment != nil {
			for _, resource := range deployment.Resources {
				if err := resources.Add(*resource.ID); err != nil {
					return nil, err
				}
			}
		}

		c.azureContext = NewAzureContext(scope, &resources)
	}

	return c.azureContext, nil
}

func (c *Context) SaveEnvironment(ctx context.Context, env *environment.Environment) error {
	err := c.container.Invoke(func(envManager environment.Manager) error {
		return envManager.Save(ctx, env)
	})

	return err
}

func (c *Context) SaveUserConfig(ctx context.Context, userConfig config.UserConfig) error {
	err := c.container.Invoke(func(userConfigManager config.UserConfigManager) error {
		return userConfigManager.Save(userConfig)
	})

	return err
}

func (c *Context) Invoke(resolver any) error {
	return c.container.Invoke(resolver)
}

func registerComponents(ctx context.Context, container *ioc.NestedContainer) error {
	container.MustRegisterSingleton(func() ioc.ServiceLocator {
		return container
	})

	container.MustRegisterSingleton(azd.NewContext)
	container.MustRegisterSingleton(environment.NewManager)
	container.MustRegisterSingleton(environment.NewLocalFileDataStore)
	container.MustRegisterSingleton(config.NewFileConfigManager)
	container.MustRegisterSingleton(config.NewManager)
	container.MustRegisterSingleton(config.NewUserConfigManager)

	container.MustRegisterSingleton(azure.NewResourceService)
	container.MustRegisterSingleton(azure.NewSubscriptionsService)
	container.MustRegisterSingleton(azure.NewEntraIdService)
	container.MustRegisterSingleton(azure.NewStandardDeployments)
	container.MustRegisterSingleton(azure.NewStackDeployments)

	container.MustRegisterSingleton(func() (azcore.TokenCredential, error) {
		return current.Credential()
	})

	container.MustRegisterSingleton(func(azdContext *azd.Context) (*project.ProjectConfig, error) {
		if azdContext == nil {
			return nil, azd.ErrNoProject
		}

		return project.Load(ctx, azdContext.ProjectPath())
	})

	container.MustRegisterSingleton(func(azdContext *azd.Context, envManager environment.Manager) (*environment.Environment, error) {
		if azdContext == nil {
			return nil, azd.ErrNoProject
		}

		envName, err := azdContext.GetDefaultEnvironmentName()
		if err != nil {
			return nil, err
		}

		environment, err := envManager.Get(ctx, envName)
		if err != nil {
			return nil, err
		}

		return environment, nil
	})

	container.MustRegisterSingleton(func(userConfigManager config.UserConfigManager) (config.UserConfig, error) {
		return userConfigManager.Load()
	})

	container.MustRegisterSingleton(func(projectConfig *project.ProjectConfig, userConfigManager config.UserConfigManager) (*contracts.RemoteConfig, error) {
		var remoteStateConfig *contracts.RemoteConfig

		userConfig, err := userConfigManager.Load()
		if err != nil {
			return nil, fmt.Errorf("loading user config: %w", err)
		}

		// Lookup remote state config in the following precedence:
		// 1. Project azure.yaml
		// 2. User configuration
		if projectConfig != nil && projectConfig.State != nil && projectConfig.State.Remote != nil {
			remoteStateConfig = projectConfig.State.Remote
		} else {
			if _, err := userConfig.GetSection("state.remote", &remoteStateConfig); err != nil {
				return nil, fmt.Errorf("getting remote state config: %w", err)
			}
		}

		return remoteStateConfig, nil
	})

	ioc.RegisterInstance[policy.Transporter](container, http.DefaultClient)
	ioc.RegisterInstance(container, azure.AzurePublic())

	container.MustRegisterSingleton(func(transport policy.Transporter, cloud *azure.Cloud) *arm.ClientOptions {
		return &arm.ClientOptions{
			ClientOptions: azcore.ClientOptions{
				Cloud: cloud.Configuration,
				Logging: policy.LogOptions{
					AllowedHeaders: []string{azure.MsCorrelationIdHeader},
					IncludeBody:    false,
				},
				PerCallPolicies: []policy.Policy{
					azure.NewMsCorrelationPolicy(),
					azure.NewUserAgentPolicy("azd"),
				},
				Transport: transport,
			},
		}
	})

	container.MustRegisterSingleton(func(transport policy.Transporter, cloud *azure.Cloud) *azcore.ClientOptions {
		return &azcore.ClientOptions{
			Cloud: cloud.Configuration,
			Logging: policy.LogOptions{
				AllowedHeaders: []string{azure.MsCorrelationIdHeader},
				IncludeBody:    false,
			},
			PerCallPolicies: []policy.Policy{
				azure.NewMsCorrelationPolicy(),
				azure.NewUserAgentPolicy("azd"),
			},
			Transport: transport,
		}
	})

	return nil
}

type AzureScope struct {
	TenantId       string
	SubscriptionId string
	Location       string
	ResourceGroup  string
}

type AzureResourceList struct {
	resources []*arm.ResourceID
}

func (arl *AzureResourceList) Add(resourceId string) error {
	if _, has := arl.FindById(resourceId); has {
		return nil
	}

	parsedResource, err := arm.ParseResourceID(resourceId)
	if err != nil {
		return err
	}

	arl.resources = append(arl.resources, parsedResource)
	log.Printf("Added resource: %s", resourceId)

	return nil
}

func (arl *AzureResourceList) Find(predicate func(resourceId *arm.ResourceID) bool) (*arm.ResourceID, bool) {
	for _, resource := range arl.resources {
		if predicate(resource) {
			return resource, true
		}
	}

	return nil, false
}

func (arl *AzureResourceList) FindByType(resourceType azure.ResourceType) (*arm.ResourceID, bool) {
	return arl.Find(func(resourceId *arm.ResourceID) bool {
		return strings.EqualFold(resourceId.ResourceType.String(), string(resourceType))
	})
}

func (arl *AzureResourceList) FindById(resourceId string) (*arm.ResourceID, bool) {
	return arl.Find(func(resource *arm.ResourceID) bool {
		return strings.EqualFold(resource.String(), resourceId)
	})
}

func (arl *AzureResourceList) FindByTypeAndName(resourceType azure.ResourceType, resourceName string) (*arm.ResourceID, bool) {
	return arl.Find(func(resource *arm.ResourceID) bool {
		return strings.EqualFold(resource.ResourceType.String(), string(resourceType)) && strings.EqualFold(resource.Name, resourceName)
	})
}

type AzureContext struct {
	Scope     AzureScope
	Resources *AzureResourceList
}

func NewEmptyAzureContext() *AzureContext {
	return &AzureContext{
		Scope:     AzureScope{},
		Resources: &AzureResourceList{},
	}
}

func NewAzureContext(scope AzureScope, resourceList *AzureResourceList) *AzureContext {
	return &AzureContext{
		Scope:     scope,
		Resources: resourceList,
	}
}

func (pc *AzureContext) EnsureSubscription(ctx context.Context) error {
	if pc.Scope.SubscriptionId == "" {
		subscription, err := PromptSubscription(context.Background(), nil)
		if err != nil {
			return err
		}

		pc.Scope.TenantId = subscription.TenantId
		pc.Scope.SubscriptionId = subscription.Id
	}

	return nil
}

func (pc *AzureContext) EnsureResourceGroup(ctx context.Context) error {
	if pc.Scope.ResourceGroup == "" {
		resourceGroup, err := PromptResourceGroup(ctx, pc, nil)
		if err != nil {
			return err
		}

		pc.Scope.ResourceGroup = resourceGroup.Name
	}

	return nil
}

func (pc *AzureContext) EnsureLocation(ctx context.Context) error {
	if pc.Scope.Location == "" {
		location, err := PromptLocation(ctx, pc, nil)
		if err != nil {
			return err
		}

		pc.Scope.Location = location.Name
	}

	return nil
}

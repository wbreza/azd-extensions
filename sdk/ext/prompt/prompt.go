package prompt

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"slices"

	"dario.cat/mergo"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	"github.com/wbreza/azd-extensions/sdk/azure"
	"github.com/wbreza/azd-extensions/sdk/ext"
	"github.com/wbreza/azd-extensions/sdk/ux"
)

var (
	ErrNoResourcesFound   = fmt.Errorf("no resources found")
	ErrNoResourceSelected = fmt.Errorf("no resource selected")
)

// PromptResourceOptions contains options for prompting the user to select a resource.
type PromptResourceOptions struct {
	// ResourceType is the type of resource to select.
	ResourceType *azure.ResourceType
	// Kinds is a list of resource kinds to filter by.
	Kinds []string
	// ResourceTypeDisplayName is the display name of the resource type.
	ResourceTypeDisplayName string
	// SelectorOptions contains options for the resource selector.
	SelectorOptions *PromptSelectOptions
}

// PromptCustomResourceOptions contains options for prompting the user to select a custom resource.
type PromptCustomResourceOptions[T any] struct {
	// SelectorOptions contains options for the resource selector.
	SelectorOptions *PromptSelectOptions
	// LoadData is a function that loads the resource data.
	LoadData func(ctx context.Context) ([]*T, error)
	// DisplayResource is a function that displays the resource.
	DisplayResource func(resource *T) (string, error)
	// SortResource is a function that sorts the resources.
	SortResource func(a *T, b *T) int
	// Selected is a function that determines if a resource is selected
	Selected func(resource *T) bool
	// CreateResource is a function that creates a new resource.
	CreateResource func(ctx context.Context) (*T, error)
}

// PromptResourceGroupOptions contains options for prompting the user to select a resource group.
type PromptResourceGroupOptions struct {
	// SelectorOptions contains options for the resource group selector.
	SelectorOptions *PromptSelectOptions
}

// PromptSelectOptions contains options for prompting the user to select a resource.
type PromptSelectOptions struct {
	// AllowNewResource specifies whether to allow the user to create a new resource.
	AllowNewResource bool
	// NewResourceMessage is the message to display to the user when creating a new resource.
	NewResourceMessage string
	// CreatingMessage is the message to display to the user when creating a new resource.
	CreatingMessage string
	// Message is the message to display to the user.
	Message string
	// HelpMessage is the help message to display to the user.
	HelpMessage string
	// LoadingMessage is the loading message to display to the user.
	LoadingMessage string
	// DisplayNumbers specifies whether to display numbers next to the choices.
	DisplayNumbers *bool
	// DisplayCount is the number of choices to display at a time.
	DisplayCount int
}

type ResourceSelection[T any] struct {
	Resource *T
	Exists   bool
}

// PromptSubscription prompts the user to select an Azure subscription.
func PromptSubscription(ctx context.Context, selectorOptions *PromptSelectOptions) (*azure.Subscription, error) {
	if selectorOptions == nil {
		selectorOptions = &PromptSelectOptions{}
	}

	mergo.Merge(selectorOptions, &PromptSelectOptions{
		Message:          "Select subscription",
		LoadingMessage:   "Loading subscriptions...",
		HelpMessage:      "Choose an Azure subscription for your project.",
		DisplayNumbers:   ux.Ptr(true),
		DisplayCount:     10,
		AllowNewResource: false,
	})

	azdContext, err := ext.CurrentContext(ctx)
	if err != nil {
		return nil, err
	}

	userConfig, err := azdContext.UserConfig(ctx)
	if err != nil {
		log.Println("User config not found")
	}

	var defaultSubscriptionId = ""
	if userConfig != nil {
		subscriptionId, has := userConfig.GetString("defaults.subscription")
		if has {
			defaultSubscriptionId = subscriptionId
		}
	}

	return PromptCustomResource(ctx, PromptCustomResourceOptions[azure.Subscription]{
		SelectorOptions: selectorOptions,
		LoadData: func(ctx context.Context) ([]*azure.Subscription, error) {
			principal, err := azdContext.Principal(ctx)
			if err != nil {
				return nil, err
			}

			credential, err := azdContext.Credential()
			if err != nil {
				return nil, err
			}

			subscriptionService := azure.NewSubscriptionsService(credential, nil)
			subscriptionList, err := subscriptionService.ListSubscriptions(ctx, principal.TenantId)
			if err != nil {
				return nil, err
			}

			subscriptions := make([]*azure.Subscription, len(subscriptionList))
			for i, subscription := range subscriptionList {
				subscriptions[i] = &azure.Subscription{
					Id:                 *subscription.SubscriptionID,
					Name:               *subscription.DisplayName,
					TenantId:           *subscription.TenantID,
					UserAccessTenantId: principal.TenantId,
				}
			}

			return subscriptions, nil
		},
		DisplayResource: func(subscription *azure.Subscription) (string, error) {
			return fmt.Sprintf("%s (%s)", subscription.Name, subscription.Id), nil
		},
		Selected: func(subscription *azure.Subscription) bool {
			return strings.EqualFold(subscription.Id, defaultSubscriptionId)
		},
	})
}

// PromptLocation prompts the user to select an Azure location.
func PromptLocation(ctx context.Context, subscription *azure.Subscription, selectorOptions *PromptSelectOptions) (*azure.Location, error) {
	if selectorOptions == nil {
		selectorOptions = &PromptSelectOptions{}
	}

	mergo.Merge(selectorOptions, &PromptSelectOptions{
		Message:          "Select location",
		LoadingMessage:   "Loading locations...",
		HelpMessage:      "Choose an Azure location for your project.",
		DisplayNumbers:   ux.Ptr(true),
		DisplayCount:     10,
		AllowNewResource: false,
	})

	azdContext, err := ext.CurrentContext(ctx)
	if err != nil {
		return nil, err
	}

	credential, err := azdContext.Credential()
	if err != nil {
		return nil, err
	}

	userConfig, err := azdContext.UserConfig(ctx)
	if errors.Is(err, ext.ErrUserConfigNotFound) {
		log.Println("User config not found")
	}

	var defaultLocation = "eastus2"
	if userConfig != nil {
		location, has := userConfig.GetString("defaults.location")
		if has {
			defaultLocation = location
		}
	}

	return PromptCustomResource(ctx, PromptCustomResourceOptions[azure.Location]{
		SelectorOptions: selectorOptions,
		LoadData: func(ctx context.Context) ([]*azure.Location, error) {
			subscriptionService := azure.NewSubscriptionsService(credential, nil)
			locationList, err := subscriptionService.ListSubscriptionLocations(ctx, subscription.Id, subscription.TenantId)
			if err != nil {
				return nil, err
			}

			locations := make([]*azure.Location, len(locationList))
			for i, location := range locationList {
				locations[i] = &azure.Location{
					Name:                location.Name,
					DisplayName:         location.DisplayName,
					RegionalDisplayName: location.RegionalDisplayName,
				}
			}

			return locations, nil
		},
		DisplayResource: func(location *azure.Location) (string, error) {
			return fmt.Sprintf("%s (%s)", location.RegionalDisplayName, location.Name), nil
		},
		Selected: func(resource *azure.Location) bool {
			return resource.Name == defaultLocation
		},
	})
}

// PromptResourceGroup prompts the user to select an Azure resource group.
func PromptResourceGroup(ctx context.Context, subscription *azure.Subscription, options *PromptResourceGroupOptions) (*azure.ResourceGroup, error) {
	if options == nil {
		options = &PromptResourceGroupOptions{}
	}

	if options.SelectorOptions == nil {
		options.SelectorOptions = &PromptSelectOptions{}
	}

	mergo.Merge(options.SelectorOptions, &PromptSelectOptions{
		Message:            "Select resource group",
		LoadingMessage:     "Loading resource groups...",
		HelpMessage:        "Choose an Azure resource group for your project.",
		DisplayNumbers:     ux.Ptr(true),
		DisplayCount:       10,
		AllowNewResource:   true,
		NewResourceMessage: "Create new resource group",
		CreatingMessage:    "Creating new resource group...",
	})

	azdContext, err := ext.CurrentContext(ctx)
	if err != nil {
		return nil, err
	}

	credential, err := azdContext.Credential()
	if err != nil {
		return nil, err
	}

	resourceService := azure.NewResourceService(credential, nil)

	return PromptCustomResource(ctx, PromptCustomResourceOptions[azure.ResourceGroup]{
		SelectorOptions: options.SelectorOptions,
		LoadData: func(ctx context.Context) ([]*azure.ResourceGroup, error) {
			resourceGroupList, err := resourceService.ListResourceGroup(ctx, subscription.Id, nil)
			if err != nil {
				return nil, err
			}

			resourceGroups := make([]*azure.ResourceGroup, len(resourceGroupList))
			for i, resourceGroup := range resourceGroupList {
				resourceGroups[i] = &azure.ResourceGroup{
					Id:       resourceGroup.Id,
					Name:     resourceGroup.Name,
					Location: resourceGroup.Location,
				}
			}

			return resourceGroups, nil
		},
		DisplayResource: func(resourceGroup *azure.ResourceGroup) (string, error) {
			return fmt.Sprintf("%s (Location: %s)", resourceGroup.Name, resourceGroup.Location), nil
		},
		CreateResource: func(ctx context.Context) (*azure.ResourceGroup, error) {
			namePrompt := ux.NewPrompt(&ux.PromptConfig{
				Message: "Enter the name for the resource group",
			})

			resourceGroupName, err := namePrompt.Ask()
			if err != nil {
				return nil, err
			}

			location, err := PromptLocation(ctx, subscription, nil)
			if err != nil {
				return nil, err
			}

			spinner := ux.NewSpinner(&ux.SpinnerConfig{
				Text: "Creating resource group...",
			})

			var resourceGroup *azure.ResourceGroup

			err = spinner.Run(ctx, func(ctx context.Context) error {
				if err := resourceService.CreateOrUpdateResourceGroup(ctx, subscription.Id, resourceGroupName, location.Name, nil); err != nil {
					return err
				}

				newResourceGroup, err := resourceService.GetResourceGroup(ctx, subscription.Id, resourceGroupName)
				if err != nil {
					return err
				}

				resourceGroup = newResourceGroup

				return nil
			})

			if err != nil {
				return nil, err
			}

			return resourceGroup, nil
		},
	})
}

// PromptSubscriptionResource prompts the user to select an Azure subscription resource.
func PromptSubscriptionResource(ctx context.Context, subscription *azure.Subscription, options PromptResourceOptions) (*azure.ResourceExtended, error) {
	if options.SelectorOptions == nil {
		resourceName := options.ResourceTypeDisplayName

		if resourceName == "" && options.ResourceType != nil {
			resourceName = string(*options.ResourceType)
		}

		if resourceName == "" {
			resourceName = "resource"
		}

		options.SelectorOptions = &PromptSelectOptions{
			Message:            fmt.Sprintf("Select %s", resourceName),
			LoadingMessage:     fmt.Sprintf("Loading %s resources...", resourceName),
			HelpMessage:        fmt.Sprintf("Choose an Azure %s for your project.", resourceName),
			DisplayNumbers:     ux.Ptr(true),
			DisplayCount:       10,
			AllowNewResource:   true,
			NewResourceMessage: fmt.Sprintf("Create new %s", resourceName),
			CreatingMessage:    fmt.Sprintf("Creating new %s...", resourceName),
		}
	}

	return PromptCustomResource(ctx, PromptCustomResourceOptions[azure.ResourceExtended]{
		SelectorOptions: options.SelectorOptions,
		LoadData: func(ctx context.Context) ([]*azure.ResourceExtended, error) {
			var resourceListOptions *armresources.ClientListOptions
			if options.ResourceType != nil {
				resourceListOptions = &armresources.ClientListOptions{
					Filter: to.Ptr(fmt.Sprintf("resourceType eq '%s'", string(*options.ResourceType))),
				}
			}

			azdContext, err := ext.CurrentContext(ctx)
			if err != nil {
				return nil, err
			}

			credential, err := azdContext.Credential()
			if err != nil {
				return nil, err
			}

			resourceService := azure.NewResourceService(credential, nil)
			resourceList, err := resourceService.ListSubscriptionResources(ctx, subscription.Id, resourceListOptions)
			if err != nil {
				return nil, err
			}

			filteredResources := []*azure.ResourceExtended{}
			hasKindFilter := len(options.Kinds) > 0

			for _, resource := range resourceList {
				if !hasKindFilter || slices.Contains(options.Kinds, resource.Kind) {
					filteredResources = append(filteredResources, resource)
				}
			}

			if len(filteredResources) == 0 {
				if options.ResourceType == nil {
					return nil, ErrNoResourcesFound
				}

				return nil, fmt.Errorf("no resources found with type '%v'", *options.ResourceType)
			}

			return filteredResources, nil
		},
		DisplayResource: func(resource *azure.ResourceExtended) (string, error) {
			parsedResource, err := arm.ParseResourceID(resource.Id)
			if err != nil {
				return "", fmt.Errorf("parsing resource id: %w", err)
			}

			return fmt.Sprintf("%s (%s)", parsedResource.Name, parsedResource.ResourceGroupName), nil
		},
	})
}

// PromptResourceGroupResource prompts the user to select an Azure resource group resource.
func PromptResourceGroupResource(ctx context.Context, resourceGroup *azure.ResourceGroup, options PromptResourceOptions) (*azure.ResourceExtended, error) {
	if options.SelectorOptions == nil {
		resourceName := options.ResourceTypeDisplayName

		if resourceName == "" && options.ResourceType != nil {
			resourceName = string(*options.ResourceType)
		}

		if resourceName == "" {
			resourceName = "resource"
		}

		options.SelectorOptions = &PromptSelectOptions{
			Message:            fmt.Sprintf("Select %s", resourceName),
			LoadingMessage:     fmt.Sprintf("Loading %s resources...", resourceName),
			HelpMessage:        fmt.Sprintf("Choose an Azure %s for your project.", resourceName),
			DisplayNumbers:     ux.Ptr(true),
			DisplayCount:       10,
			AllowNewResource:   true,
			NewResourceMessage: fmt.Sprintf("Create new %s", resourceName),
			CreatingMessage:    fmt.Sprintf("Creating new %s...", resourceName),
		}
	}

	azdContext, err := ext.CurrentContext(ctx)
	if err != nil {
		return nil, err
	}

	credential, err := azdContext.Credential()
	if err != nil {
		return nil, err
	}

	parsedResourceGroup, err := arm.ParseResourceID(resourceGroup.Id)
	if err != nil {
		return nil, fmt.Errorf("parsing resource group id: %w", err)
	}

	resource, err := PromptCustomResource(ctx, PromptCustomResourceOptions[azure.ResourceExtended]{
		SelectorOptions: options.SelectorOptions,
		LoadData: func(ctx context.Context) ([]*azure.ResourceExtended, error) {
			var resourceListOptions *azure.ListResourceGroupResourcesOptions
			if options.ResourceType != nil {
				resourceListOptions = &azure.ListResourceGroupResourcesOptions{
					Filter: to.Ptr(fmt.Sprintf("resourceType eq '%s'", *options.ResourceType)),
				}
			}

			resourceService := azure.NewResourceService(credential, nil)
			resourceList, err := resourceService.ListResourceGroupResources(ctx, parsedResourceGroup.SubscriptionID, resourceGroup.Name, resourceListOptions)
			if err != nil {
				return nil, err
			}

			filteredResources := []*azure.ResourceExtended{}
			hasKindFilter := len(options.Kinds) > 0

			for _, resource := range resourceList {
				if !hasKindFilter || slices.Contains(options.Kinds, resource.Kind) {
					filteredResources = append(filteredResources, resource)
				}
			}

			if len(filteredResources) == 0 {
				if options.ResourceType == nil {
					return nil, ErrNoResourcesFound
				}

				return nil, fmt.Errorf("no resources found with type '%v'", *options.ResourceType)
			}

			return filteredResources, nil
		},
		DisplayResource: func(resource *azure.ResourceExtended) (string, error) {
			return resource.Name, nil
		},
	})

	if err != nil {
		return nil, err
	}

	return resource, nil
}

// PromptCustomResource prompts the user to select a custom resource from a list of resources.
func PromptCustomResource[T any](ctx context.Context, options PromptCustomResourceOptions[T]) (*T, error) {
	loadingSpinner := ux.NewSpinner(&ux.SpinnerConfig{
		Text: options.SelectorOptions.LoadingMessage,
	})

	var resources []*T

	err := loadingSpinner.Run(ctx, func(ctx context.Context) error {
		resourceList, err := options.LoadData(ctx)
		if err != nil {
			return err
		}

		resources = resourceList
		return nil
	})
	if err != nil {
		return nil, err
	}

	if len(resources) == 0 {
		return nil, ErrNoResourcesFound
	}

	if options.SortResource != nil {
		slices.SortFunc(resources, options.SortResource)
	}

	var defaultIndex *int
	if options.Selected != nil {
		for i, resource := range resources {
			if options.Selected(resource) {
				defaultIndex = &i
				break
			}
		}
	}

	hasCustomDisplay := options.DisplayResource != nil

	var choices []string

	if options.SelectorOptions.AllowNewResource {
		choices = make([]string, len(resources)+1)
		choices[0] = options.SelectorOptions.NewResourceMessage
	} else {
		choices = make([]string, len(resources))
	}

	for i, resource := range resources {
		var displayValue string

		if hasCustomDisplay {
			customDisplayValue, err := options.DisplayResource(resource)
			if err != nil {
				return nil, err
			}

			displayValue = customDisplayValue
		} else {
			displayValue = fmt.Sprintf("%v", resource)
		}

		if options.SelectorOptions.AllowNewResource {
			choices[i+1] = displayValue
		} else {
			choices[i] = displayValue
		}
	}

	resourceSelector := ux.NewSelect(&ux.SelectConfig{
		Message:        options.SelectorOptions.Message,
		HelpMessage:    options.SelectorOptions.HelpMessage,
		DisplayCount:   options.SelectorOptions.DisplayCount,
		DisplayNumbers: options.SelectorOptions.DisplayNumbers,
		Allowed:        choices,
		DefaultIndex:   defaultIndex,
	})

	selectedIndex, err := resourceSelector.Ask()
	if err != nil {
		return nil, err
	}

	if selectedIndex == nil {
		return nil, ErrNoResourceSelected
	}

	// Create new resource
	if *selectedIndex == 0 && options.SelectorOptions.AllowNewResource {
		if options.CreateResource == nil {
			return nil, fmt.Errorf("no create resource function provided")
		}

		return options.CreateResource(ctx)
	}

	return resources[*selectedIndex], nil
}

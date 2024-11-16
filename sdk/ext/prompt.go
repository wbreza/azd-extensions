package ext

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
	"github.com/fatih/color"
	"github.com/wbreza/azd-extensions/sdk/azure"
	"github.com/wbreza/azd-extensions/sdk/ux"
)

var (
	ErrNoResourcesFound   = fmt.Errorf("no resources found")
	ErrNoResourceSelected = fmt.Errorf("no resource selected")
)

// ResourceOptions contains options for prompting the user to select a resource.
type ResourceOptions struct {
	// ResourceType is the type of resource to select.
	ResourceType *azure.ResourceType
	// Kinds is a list of resource kinds to filter by.
	Kinds []string
	// ResourceTypeDisplayName is the display name of the resource type.
	ResourceTypeDisplayName string
	// SelectorOptions contains options for the resource selector.
	SelectorOptions *SelectOptions
	// CreateResource is a function that creates a new resource.
	CreateResource func(ctx context.Context) (*azure.ResourceExtended, error)
	// Selected is a function that determines if a resource is selected
	Selected func(resource *azure.ResourceExtended) bool
}

// CustomResourceOptions contains options for prompting the user to select a custom resource.
type CustomResourceOptions[T any] struct {
	// SelectorOptions contains options for the resource selector.
	SelectorOptions *SelectOptions
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

// ResourceGroupOptions contains options for prompting the user to select a resource group.
type ResourceGroupOptions struct {
	// SelectorOptions contains options for the resource group selector.
	SelectorOptions *SelectOptions
}

// SelectOptions contains options for prompting the user to select a resource.
type SelectOptions struct {
	// ForceNewResource specifies whether to force the user to create a new resource.
	ForceNewResource *bool
	// AllowNewResource specifies whether to allow the user to create a new resource.
	AllowNewResource *bool
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
func PromptSubscription(ctx context.Context, selectorOptions *SelectOptions) (*azure.Subscription, error) {
	mergedOptions := &SelectOptions{}
	if selectorOptions == nil {
		selectorOptions = &SelectOptions{}
	}

	defaultOptions := &SelectOptions{
		Message:          "Select subscription",
		LoadingMessage:   "Loading subscriptions...",
		HelpMessage:      "Choose an Azure subscription for your project.",
		AllowNewResource: ux.Ptr(false),
	}

	mergo.Merge(mergedOptions, selectorOptions, mergo.WithoutDereference)
	mergo.Merge(mergedOptions, defaultOptions, mergo.WithoutDereference)

	azdContext, err := CurrentContext(ctx)
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

	return PromptCustomResource(ctx, CustomResourceOptions[azure.Subscription]{
		SelectorOptions: mergedOptions,
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
			return fmt.Sprintf("%s %s", subscription.Name, color.HiBlackString("(%s)", subscription.Id)), nil
		},
		Selected: func(subscription *azure.Subscription) bool {
			return strings.EqualFold(subscription.Id, defaultSubscriptionId)
		},
	})
}

// PromptLocation prompts the user to select an Azure location.
func PromptLocation(ctx context.Context, azureContext *AzureContext, selectorOptions *SelectOptions) (*azure.Location, error) {
	if azureContext == nil {
		azureContext = NewEmptyAzureContext()
	}

	if err := azureContext.EnsureSubscription(ctx); err != nil {
		return nil, err
	}

	mergedOptions := &SelectOptions{}

	if selectorOptions == nil {
		selectorOptions = &SelectOptions{}
	}

	defaultOptions := &SelectOptions{
		Message:          "Select location",
		LoadingMessage:   "Loading locations...",
		HelpMessage:      "Choose an Azure location for your project.",
		AllowNewResource: ux.Ptr(false),
	}

	mergo.Merge(mergedOptions, selectorOptions, mergo.WithoutDereference)
	mergo.Merge(mergedOptions, defaultOptions, mergo.WithoutDereference)

	azdContext, err := CurrentContext(ctx)
	if err != nil {
		return nil, err
	}

	credential, err := azdContext.Credential()
	if err != nil {
		return nil, err
	}

	userConfig, err := azdContext.UserConfig(ctx)
	if errors.Is(err, ErrUserConfigNotFound) {
		log.Println("User config not found")
	}

	var defaultLocation = "eastus2"
	if userConfig != nil {
		location, has := userConfig.GetString("defaults.location")
		if has {
			defaultLocation = location
		}
	}

	return PromptCustomResource(ctx, CustomResourceOptions[azure.Location]{
		SelectorOptions: mergedOptions,
		LoadData: func(ctx context.Context) ([]*azure.Location, error) {
			subscriptionService := azure.NewSubscriptionsService(credential, nil)
			locationList, err := subscriptionService.ListSubscriptionLocations(ctx, azureContext.Scope.SubscriptionId, azureContext.Scope.TenantId)
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
			return fmt.Sprintf("%s %s", location.RegionalDisplayName, color.HiBlackString("(%s)", location.Name)), nil
		},
		Selected: func(resource *azure.Location) bool {
			return resource.Name == defaultLocation
		},
	})
}

// PromptResourceGroup prompts the user to select an Azure resource group.
func PromptResourceGroup(ctx context.Context, azureContext *AzureContext, options *ResourceGroupOptions) (*azure.ResourceGroup, error) {
	if azureContext == nil {
		azureContext = NewEmptyAzureContext()
	}

	if err := azureContext.EnsureSubscription(ctx); err != nil {
		return nil, err
	}

	mergedSelectorOptions := &SelectOptions{}

	if options == nil {
		options = &ResourceGroupOptions{}
	}

	if options.SelectorOptions == nil {
		options.SelectorOptions = &SelectOptions{}
	}

	defaultSelectorOptions := &SelectOptions{
		Message:            "Select resource group",
		LoadingMessage:     "Loading resource groups...",
		HelpMessage:        "Choose an Azure resource group for your project.",
		AllowNewResource:   ux.Ptr(true),
		NewResourceMessage: "Create new resource group",
		CreatingMessage:    "Creating new resource group...",
	}

	mergo.Merge(mergedSelectorOptions, options.SelectorOptions, mergo.WithoutDereference)
	mergo.Merge(mergedSelectorOptions, defaultSelectorOptions, mergo.WithoutDereference)

	azdContext, err := CurrentContext(ctx)
	if err != nil {
		return nil, err
	}

	credential, err := azdContext.Credential()
	if err != nil {
		return nil, err
	}

	resourceService := azure.NewResourceService(credential, nil)

	return PromptCustomResource(ctx, CustomResourceOptions[azure.ResourceGroup]{
		SelectorOptions: mergedSelectorOptions,
		LoadData: func(ctx context.Context) ([]*azure.ResourceGroup, error) {
			resourceGroupList, err := resourceService.ListResourceGroup(ctx, azureContext.Scope.SubscriptionId, nil)
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
			return fmt.Sprintf("%s %s", resourceGroup.Name, color.HiBlackString("(Location: %s)", resourceGroup.Location)), nil
		},
		CreateResource: func(ctx context.Context) (*azure.ResourceGroup, error) {
			namePrompt := ux.NewPrompt(&ux.PromptOptions{
				Message: "Enter the name for the resource group",
			})

			resourceGroupName, err := namePrompt.Ask()
			if err != nil {
				return nil, err
			}

			if err := azureContext.EnsureLocation(ctx); err != nil {
				return nil, err
			}

			var resourceGroup *azure.ResourceGroup

			taskName := fmt.Sprintf("Creating resource group %s", color.CyanString(resourceGroupName))

			err = ux.NewTaskList(nil).
				AddTask(ux.TaskOptions{
					Title: taskName,
					Action: func(setProgress ux.SetProgressFunc) (ux.TaskState, error) {
						newResourceGroup, err := resourceService.CreateOrUpdateResourceGroup(ctx, azureContext.Scope.SubscriptionId, resourceGroupName, azureContext.Scope.Location, nil)
						if err != nil {
							return ux.Error, err
						}

						resourceGroup = newResourceGroup
						return ux.Success, nil
					},
				}).
				Run()

			if err != nil {
				return nil, err
			}

			return resourceGroup, nil
		},
	})
}

// PromptSubscriptionResource prompts the user to select an Azure subscription resource.
func PromptSubscriptionResource(ctx context.Context, azureContext *AzureContext, options ResourceOptions) (*azure.ResourceExtended, error) {
	if azureContext == nil {
		azureContext = NewEmptyAzureContext()
	}

	if err := azureContext.EnsureSubscription(ctx); err != nil {
		return nil, err
	}

	mergedSelectorOptions := &SelectOptions{}

	if options.SelectorOptions == nil {
		options.SelectorOptions = &SelectOptions{}
	}

	var existingResource *arm.ResourceID
	if options.ResourceType != nil {
		match, has := azureContext.Resources.FindByTypeAndKind(ctx, *options.ResourceType, options.Kinds)
		if has {
			existingResource = match
		}
	}

	if options.Selected == nil {
		options.Selected = func(resource *azure.ResourceExtended) bool {
			if existingResource == nil {
				return false
			}

			if strings.EqualFold(resource.Id, existingResource.String()) {
				return true
			}

			return false
		}
	}

	resourceName := options.ResourceTypeDisplayName

	if resourceName == "" && options.ResourceType != nil {
		resourceName = string(*options.ResourceType)
	}

	if resourceName == "" {
		resourceName = "resource"
	}

	defaultSelectorOptions := &SelectOptions{
		Message:            fmt.Sprintf("Select %s", resourceName),
		LoadingMessage:     fmt.Sprintf("Loading %s resources...", resourceName),
		HelpMessage:        fmt.Sprintf("Choose an Azure %s for your project.", resourceName),
		AllowNewResource:   ux.Ptr(true),
		NewResourceMessage: fmt.Sprintf("Create new %s", resourceName),
		CreatingMessage:    fmt.Sprintf("Creating new %s...", resourceName),
	}

	mergo.Merge(mergedSelectorOptions, options.SelectorOptions, mergo.WithoutDereference)
	mergo.Merge(mergedSelectorOptions, defaultSelectorOptions, mergo.WithoutDereference)

	resource, err := PromptCustomResource(ctx, CustomResourceOptions[azure.ResourceExtended]{
		SelectorOptions: mergedSelectorOptions,
		LoadData: func(ctx context.Context) ([]*azure.ResourceExtended, error) {
			var resourceListOptions *armresources.ClientListOptions
			if options.ResourceType != nil {
				resourceListOptions = &armresources.ClientListOptions{
					Filter: to.Ptr(fmt.Sprintf("resourceType eq '%s'", string(*options.ResourceType))),
				}
			}

			azdContext, err := CurrentContext(ctx)
			if err != nil {
				return nil, err
			}

			credential, err := azdContext.Credential()
			if err != nil {
				return nil, err
			}

			resourceService := azure.NewResourceService(credential, nil)
			resourceList, err := resourceService.ListSubscriptionResources(ctx, azureContext.Scope.SubscriptionId, resourceListOptions)
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

			return fmt.Sprintf("%s %s", parsedResource.Name, color.HiBlackString("(%s)", parsedResource.ResourceGroupName)), nil
		},
		Selected:       options.Selected,
		CreateResource: options.CreateResource,
	})

	if err != nil {
		return nil, err
	}

	if err := azureContext.Resources.Add(resource.Id); err != nil {
		return nil, err
	}

	return resource, nil
}

// PromptResourceGroupResource prompts the user to select an Azure resource group resource.
func PromptResourceGroupResource(ctx context.Context, azureContext *AzureContext, options ResourceOptions) (*azure.ResourceExtended, error) {
	if azureContext == nil {
		azureContext = NewEmptyAzureContext()
	}

	if err := azureContext.EnsureResourceGroup(ctx); err != nil {
		return nil, err
	}

	mergedSelectorOptions := &SelectOptions{}

	if options.SelectorOptions == nil {
		options.SelectorOptions = &SelectOptions{}
	}

	var existingResource *arm.ResourceID
	if options.ResourceType != nil {
		match, has := azureContext.Resources.FindByTypeAndKind(ctx, *options.ResourceType, options.Kinds)
		if has {
			existingResource = match
		}
	}

	if options.Selected == nil {
		options.Selected = func(resource *azure.ResourceExtended) bool {
			if existingResource == nil {
				return false
			}

			return strings.EqualFold(resource.Id, existingResource.String())
		}
	}

	resourceName := options.ResourceTypeDisplayName

	if resourceName == "" && options.ResourceType != nil {
		resourceName = string(*options.ResourceType)
	}

	if resourceName == "" {
		resourceName = "resource"
	}

	defaultSelectorOptions := &SelectOptions{
		Message:            fmt.Sprintf("Select %s", resourceName),
		LoadingMessage:     fmt.Sprintf("Loading %s resources...", resourceName),
		HelpMessage:        fmt.Sprintf("Choose an Azure %s for your project.", resourceName),
		AllowNewResource:   ux.Ptr(true),
		NewResourceMessage: fmt.Sprintf("Create new %s", resourceName),
		CreatingMessage:    fmt.Sprintf("Creating new %s...", resourceName),
	}

	mergo.Merge(mergedSelectorOptions, options.SelectorOptions, mergo.WithoutDereference)
	mergo.Merge(mergedSelectorOptions, defaultSelectorOptions, mergo.WithoutDereference)

	azdContext, err := CurrentContext(ctx)
	if err != nil {
		return nil, err
	}

	credential, err := azdContext.Credential()
	if err != nil {
		return nil, err
	}

	resource, err := PromptCustomResource(ctx, CustomResourceOptions[azure.ResourceExtended]{
		Selected:        options.Selected,
		SelectorOptions: mergedSelectorOptions,
		LoadData: func(ctx context.Context) ([]*azure.ResourceExtended, error) {
			var resourceListOptions *azure.ListResourceGroupResourcesOptions
			if options.ResourceType != nil {
				resourceListOptions = &azure.ListResourceGroupResourcesOptions{
					Filter: to.Ptr(fmt.Sprintf("resourceType eq '%s'", *options.ResourceType)),
				}
			}

			resourceService := azure.NewResourceService(credential, nil)
			resourceList, err := resourceService.ListResourceGroupResources(ctx, azureContext.Scope.SubscriptionId, azureContext.Scope.ResourceGroup, resourceListOptions)
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
		CreateResource: options.CreateResource,
	})

	if err != nil {
		return nil, err
	}

	if err := azureContext.Resources.Add(resource.Id); err != nil {
		return nil, err
	}

	return resource, nil
}

// PromptCustomResource prompts the user to select a custom resource from a list of resources.
func PromptCustomResource[T any](ctx context.Context, options CustomResourceOptions[T]) (*T, error) {
	mergedSelectorOptions := &SelectOptions{}

	if options.SelectorOptions == nil {
		options.SelectorOptions = &SelectOptions{}
	}

	defaultSelectorOptions := &SelectOptions{
		Message:            "Select resource",
		LoadingMessage:     "Loading resources...",
		HelpMessage:        "Choose a resource for your project.",
		AllowNewResource:   ux.Ptr(true),
		ForceNewResource:   ux.Ptr(false),
		NewResourceMessage: "Create new resource",
		CreatingMessage:    "Creating new resource...",
		DisplayNumbers:     ux.Ptr(true),
		DisplayCount:       10,
	}

	mergo.Merge(mergedSelectorOptions, options.SelectorOptions, mergo.WithoutDereference)
	mergo.Merge(mergedSelectorOptions, defaultSelectorOptions, mergo.WithoutDereference)

	allowNewResource := mergedSelectorOptions.AllowNewResource != nil && *mergedSelectorOptions.AllowNewResource
	forceNewResource := mergedSelectorOptions.ForceNewResource != nil && *mergedSelectorOptions.ForceNewResource

	var resources []*T
	var selectedIndex *int

	if forceNewResource {
		allowNewResource = true
		selectedIndex = ux.Ptr(0)
	} else {
		loadingSpinner := ux.NewSpinner(&ux.SpinnerOptions{
			Text: options.SelectorOptions.LoadingMessage,
		})

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

		if !allowNewResource && len(resources) == 0 {
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

		if allowNewResource {
			choices = make([]string, len(resources)+1)
			choices[0] = mergedSelectorOptions.NewResourceMessage

			if defaultIndex != nil {
				*defaultIndex++
			}
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

			if allowNewResource {
				choices[i+1] = displayValue
			} else {
				choices[i] = displayValue
			}
		}

		resourceSelector := ux.NewSelect(&ux.SelectOptions{
			Message:        mergedSelectorOptions.Message,
			HelpMessage:    mergedSelectorOptions.HelpMessage,
			DisplayCount:   mergedSelectorOptions.DisplayCount,
			DisplayNumbers: mergedSelectorOptions.DisplayNumbers,
			Allowed:        choices,
			SelectedIndex:  defaultIndex,
		})

		userSelectedIndex, err := resourceSelector.Ask()
		if err != nil {
			return nil, err
		}

		if userSelectedIndex == nil {
			return nil, ErrNoResourceSelected
		}

		selectedIndex = userSelectedIndex
	}

	var selectedResource *T

	// Create new resource
	if allowNewResource && *selectedIndex == 0 {
		if options.CreateResource == nil {
			return nil, fmt.Errorf("no create resource function provided")
		}

		createdResource, err := options.CreateResource(ctx)
		if err != nil {
			return nil, err
		}

		selectedResource = createdResource
	} else {
		// If a new resource is allowed, decrement the selected index
		if allowNewResource {
			*selectedIndex--
		}

		selectedResource = resources[*selectedIndex]
	}

	log.Printf("Selected resource: %v", *selectedResource)

	return selectedResource, nil
}

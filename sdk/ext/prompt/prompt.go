package prompt

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armsubscriptions"
	"github.com/wbreza/azd-extensions/sdk/azure"
	"github.com/wbreza/azd-extensions/sdk/ext/account"
	"github.com/wbreza/azd-extensions/sdk/ux"
)

func PromptSubscription(ctx context.Context) (*azure.Subscription, error) {
	principal, err := account.CurrentPrincipal(ctx)
	if err != nil {
		return nil, err
	}

	credential, err := account.Credential()
	if err != nil {
		return nil, err
	}

	subscriptionService := azure.NewSubscriptionsService(credential, nil)

	loadingSpinner := ux.NewSpinner(&ux.SpinnerConfig{
		Text:        "Loading subscriptions...",
		ClearOnStop: true,
	})

	var subscriptions []*armsubscriptions.Subscription

	err = loadingSpinner.Run(ctx, func(ctx context.Context) error {
		subscriptionList, err := subscriptionService.ListSubscriptions(ctx, principal.TenantId)
		if err != nil {
			return err
		}

		subscriptions = subscriptionList
		return nil
	})
	if err != nil {
		return nil, err
	}

	var defaultIndex *int
	var defaultSubscriptionId = "faa080af-c1d8-40ad-9cce-e1a450ca5b57"
	for i, subscription := range subscriptions {
		if *subscription.SubscriptionID == defaultSubscriptionId {
			defaultIndex = &i
			break
		}
	}

	choices := make([]string, len(subscriptions))
	for i, subscription := range subscriptions {
		choices[i] = fmt.Sprintf("%s (%s)", *subscription.DisplayName, *subscription.SubscriptionID)
	}

	subscriptionSelector := ux.NewSelect(&ux.SelectConfig{
		Message:        "Select subscription",
		HelpMessage:    "Choose an Azure subscription for your project.",
		DisplayCount:   10,
		DisplayNumbers: ux.Ptr(true),
		Allowed:        choices,
		DefaultIndex:   defaultIndex,
	})

	selectedIndex, err := subscriptionSelector.Ask()
	if err != nil {
		return nil, err
	}

	if selectedIndex == nil {
		return nil, nil
	}

	selectedSubscription := subscriptions[*selectedIndex]

	return &azure.Subscription{
		Id:                 *selectedSubscription.SubscriptionID,
		Name:               *selectedSubscription.DisplayName,
		TenantId:           *selectedSubscription.TenantID,
		UserAccessTenantId: principal.TenantId,
	}, nil
}

func PromptLocation(ctx context.Context, subscription *azure.Subscription) (*azure.Location, error) {
	credential, err := azidentity.NewAzureDeveloperCLICredential(nil)
	if err != nil {
		return nil, err
	}

	loadingSpinner := ux.NewSpinner(&ux.SpinnerConfig{
		Text:        "Loading locations...",
		ClearOnStop: true,
	})

	var locations []azure.Location

	err = loadingSpinner.Run(ctx, func(ctx context.Context) error {
		subscriptionService := azure.NewSubscriptionsService(credential, nil)
		locationList, err := subscriptionService.ListSubscriptionLocations(ctx, subscription.Id, subscription.TenantId)
		if err != nil {
			return err
		}

		locations = locationList

		return nil
	})

	if err != nil {
		return nil, err
	}

	var defaultIndex *int
	var defaultLocation = "eastus2"
	for i, location := range locations {
		if location.Name == defaultLocation {
			defaultIndex = &i
			break
		}
	}

	choices := make([]string, len(locations))
	for i, location := range locations {
		choices[i] = fmt.Sprintf("%s (%s)", location.RegionalDisplayName, location.Name)
	}

	locationSelector := ux.NewSelect(&ux.SelectConfig{
		Message:        "Select location",
		HelpMessage:    "Choose an Azure location for your project.",
		DisplayCount:   10,
		DisplayNumbers: ux.Ptr(true),
		Allowed:        choices,
		DefaultIndex:   defaultIndex,
	})

	selectedIndex, err := locationSelector.Ask()
	if err != nil {
		return nil, err
	}

	if selectedIndex == nil {
		return nil, nil
	}

	selectedLocation := locations[*selectedIndex]

	return &azure.Location{
		Name:                selectedLocation.Name,
		DisplayName:         selectedLocation.DisplayName,
		RegionalDisplayName: selectedLocation.RegionalDisplayName,
	}, nil
}

func PromptResourceGroup() (*azure.ResourceGroup, error) {
	return nil, nil
}

func PromptResource() (*azure.Resource, error) {
	return nil, nil
}

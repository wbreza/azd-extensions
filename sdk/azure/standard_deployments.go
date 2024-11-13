package azure

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	"github.com/wbreza/azd-extensions/sdk/common/convert"
)

// cArmDeploymentNameLengthMax is the maximum length of the name of a deployment in ARM.
const (
	cArmDeploymentNameLengthMax = 64
	cPortalUrlFragment          = "#view/HubsExtension/DeploymentDetailsBlade/~/overview/id"
	cOutputsUrlFragment         = "#view/HubsExtension/DeploymentDetailsBlade/~/outputs/id"
)

type StandardDeployments struct {
	credential       azcore.TokenCredential
	armClientOptions *arm.ClientOptions
	resourceService  *ResourceService
	cloud            *Cloud
}

func NewStandardDeployments(
	credential azcore.TokenCredential,
	armClientOptions *arm.ClientOptions,
	resourceService *ResourceService,
	cloud *Cloud,
) *StandardDeployments {
	return &StandardDeployments{
		credential:       credential,
		armClientOptions: armClientOptions,
		resourceService:  resourceService,
		cloud:            cloud,
	}
}

func (ds *StandardDeployments) ListSubscriptionDeployments(
	ctx context.Context,
	subscriptionId string,
) ([]*ResourceDeployment, error) {
	deploymentClient, err := ds.createDeploymentsClient(ctx, subscriptionId)
	if err != nil {
		return nil, fmt.Errorf("creating deployments client: %w", err)
	}

	results := []*ResourceDeployment{}

	pager := deploymentClient.NewListAtSubscriptionScopePager(nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}

		for _, deployment := range page.Value {
			results = append(results, ds.convertFromArmDeployment(deployment))
		}
	}

	return results, nil
}

func (ds *StandardDeployments) GetSubscriptionDeployment(
	ctx context.Context,
	subscriptionId string,
	deploymentName string,
) (*ResourceDeployment, error) {
	deploymentClient, err := ds.createDeploymentsClient(ctx, subscriptionId)
	if err != nil {
		return nil, fmt.Errorf("creating deployments client: %w", err)
	}

	deployment, err := deploymentClient.GetAtSubscriptionScope(ctx, deploymentName, nil)
	if err != nil {
		var errDetails *azcore.ResponseError
		if errors.As(err, &errDetails) && errDetails.StatusCode == 404 {
			return nil, ErrDeploymentNotFound
		}
		return nil, fmt.Errorf("getting deployment from subscription: %w", err)
	}

	return ds.convertFromArmDeployment(&deployment.DeploymentExtended), nil
}

func (ds *StandardDeployments) ListResourceGroupDeployments(
	ctx context.Context,
	subscriptionId string,
	resourceGroupName string,
) ([]*ResourceDeployment, error) {
	deploymentClient, err := ds.createDeploymentsClient(ctx, subscriptionId)
	if err != nil {
		return nil, fmt.Errorf("creating deployments client: %w", err)
	}

	results := []*ResourceDeployment{}

	pager := deploymentClient.NewListByResourceGroupPager(resourceGroupName, nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}

		for _, deployment := range page.Value {
			results = append(results, ds.convertFromArmDeployment(deployment))
		}
	}

	return results, nil
}

func (ds *StandardDeployments) GetResourceGroupDeployment(
	ctx context.Context,
	subscriptionId string,
	resourceGroupName string,
	deploymentName string,
) (*ResourceDeployment, error) {
	deploymentClient, err := ds.createDeploymentsClient(ctx, subscriptionId)
	if err != nil {
		return nil, fmt.Errorf("creating deployments client: %w", err)
	}

	deployment, err := deploymentClient.Get(ctx, resourceGroupName, deploymentName, nil)
	if err != nil {
		var errDetails *azcore.ResponseError
		if errors.As(err, &errDetails) && errDetails.StatusCode == 404 {
			return nil, ErrDeploymentNotFound
		}
		return nil, fmt.Errorf("getting deployment from resource group: %w", err)
	}

	return ds.convertFromArmDeployment(&deployment.DeploymentExtended), nil
}

func (ds *StandardDeployments) createDeploymentsClient(
	ctx context.Context,
	subscriptionId string,
) (*armresources.DeploymentsClient, error) {
	client, err := armresources.NewDeploymentsClient(subscriptionId, ds.credential, ds.armClientOptions)
	if err != nil {
		return nil, fmt.Errorf("creating deployments client: %w", err)
	}

	return client, nil
}

func (ds *StandardDeployments) ListSubscriptionDeploymentOperations(
	ctx context.Context,
	subscriptionId string,
	deploymentName string,
) ([]*armresources.DeploymentOperation, error) {
	result := []*armresources.DeploymentOperation{}
	deploymentOperationsClient, err := ds.createDeploymentsOperationsClient(ctx, subscriptionId)
	if err != nil {
		return nil, fmt.Errorf("creating deployments client: %w", err)
	}

	// Get all without any filter
	getDeploymentsPager := deploymentOperationsClient.NewListAtSubscriptionScopePager(deploymentName, nil)

	for getDeploymentsPager.More() {
		page, err := getDeploymentsPager.NextPage(ctx)
		var errDetails *azcore.ResponseError
		if errors.As(err, &errDetails) && errDetails.StatusCode == 404 {
			return nil, ErrDeploymentNotFound
		}
		if err != nil {
			return nil, fmt.Errorf("failed getting list of deployment operations from subscription: %w", err)
		}
		result = append(result, page.Value...)
	}

	return result, nil
}

func (ds *StandardDeployments) ListResourceGroupDeploymentOperations(
	ctx context.Context,
	subscriptionId string,
	resourceGroupName string,
	deploymentName string,
) ([]*armresources.DeploymentOperation, error) {
	result := []*armresources.DeploymentOperation{}
	deploymentOperationsClient, err := ds.createDeploymentsOperationsClient(ctx, subscriptionId)
	if err != nil {
		return nil, fmt.Errorf("creating deployments client: %w", err)
	}

	// Get all without any filter
	getDeploymentsPager := deploymentOperationsClient.NewListPager(resourceGroupName, deploymentName, nil)

	for getDeploymentsPager.More() {
		page, err := getDeploymentsPager.NextPage(ctx)
		var errDetails *azcore.ResponseError
		if errors.As(err, &errDetails) && errDetails.StatusCode == 404 {
			return nil, ErrDeploymentNotFound
		}
		if err != nil {
			return nil, fmt.Errorf("failed getting list of deployment operations from resource group: %w", err)
		}
		result = append(result, page.Value...)
	}

	return result, nil
}

func (ds *StandardDeployments) ListSubscriptionDeploymentResources(
	ctx context.Context,
	subscriptionId string,
	deploymentName string,
) ([]*armresources.ResourceReference, error) {
	subscriptionDeployment, err := ds.GetSubscriptionDeployment(ctx, subscriptionId, deploymentName)
	if err != nil {
		return nil, fmt.Errorf("getting subscription deployment: %w", err)
	}

	// Get the environment name from the deployment tags
	envName, has := subscriptionDeployment.Tags["azd-env-name"]
	if !has || envName == nil {
		return nil, fmt.Errorf("environment name not found in deployment tags")
	}

	// Get all resource groups tagged with the azd-env-name tag
	resourceGroups, err := ds.resourceService.ListResourceGroup(ctx, subscriptionId, &ListResourceGroupOptions{
		TagFilter: &Filter{Key: "azd-env-name", Value: *envName},
	})

	if err != nil {
		return nil, fmt.Errorf("listing resource groups: %w", err)
	}

	allResources := []*armresources.ResourceReference{}

	// Find all the resources from all the resource groups
	for _, resourceGroup := range resourceGroups {

		resources, err := ds.resourceService.ListResourceGroupResources(ctx, subscriptionId, resourceGroup.Name, nil)
		if err != nil {
			return nil, fmt.Errorf("listing resource group resources: %w", err)
		}

		resourceGroupId := ResourceGroupRID(subscriptionId, resourceGroup.Name)
		allResources = append(allResources, &armresources.ResourceReference{
			ID: &resourceGroupId,
		})

		for _, resource := range resources {
			allResources = append(allResources, &armresources.ResourceReference{
				ID: to.Ptr(resource.Id),
			})
		}
	}

	return allResources, nil
}

func (ds *StandardDeployments) ListResourceGroupDeploymentResources(
	ctx context.Context,
	subscriptionId string,
	resourceGroupName string,
	deploymentName string,
) ([]*armresources.ResourceReference, error) {
	resources, err := ds.resourceService.ListResourceGroupResources(ctx, subscriptionId, resourceGroupName, nil)
	if err != nil {
		return nil, fmt.Errorf("listing resource group resources: %w", err)
	}

	resourceGroupId := ResourceGroupRID(subscriptionId, resourceGroupName)

	allResources := []*armresources.ResourceReference{}
	allResources = append(allResources, &armresources.ResourceReference{
		ID: &resourceGroupId,
	})

	for _, resource := range resources {
		allResources = append(allResources, &armresources.ResourceReference{
			ID: to.Ptr(resource.Id),
		})
	}

	return allResources, nil
}

func (ds *StandardDeployments) createDeploymentsOperationsClient(
	ctx context.Context,
	subscriptionId string,
) (*armresources.DeploymentOperationsClient, error) {
	client, err := armresources.NewDeploymentOperationsClient(subscriptionId, ds.credential, ds.armClientOptions)
	if err != nil {
		return nil, fmt.Errorf("creating deployments client: %w", err)
	}

	return client, nil
}

// Converts from an ARM Extended Deployment to Azd Generic deployment
func (ds *StandardDeployments) convertFromArmDeployment(deployment *armresources.DeploymentExtended) *ResourceDeployment {
	return &ResourceDeployment{
		Id:                *deployment.ID,
		Location:          convert.ToValueWithDefault(deployment.Location, ""),
		DeploymentId:      *deployment.ID,
		Name:              *deployment.Name,
		Type:              *deployment.Type,
		Tags:              deployment.Tags,
		ProvisioningState: convertFromStandardProvisioningState(*deployment.Properties.ProvisioningState),
		Timestamp:         *deployment.Properties.Timestamp,
		TemplateHash:      deployment.Properties.TemplateHash,
		Outputs:           deployment.Properties.Outputs,
		Resources:         deployment.Properties.OutputResources,
		Dependencies:      deployment.Properties.Dependencies,

		PortalUrl: fmt.Sprintf("%s/%s/%s",
			ds.cloud.PortalUrlBase,
			cPortalUrlFragment,
			url.PathEscape(*deployment.ID),
		),

		OutputsUrl: fmt.Sprintf("%s/%s/%s",
			ds.cloud.PortalUrlBase,
			cOutputsUrlFragment,
			url.PathEscape(*deployment.ID),
		),

		DeploymentUrl: fmt.Sprintf("%s/%s/%s",
			ds.cloud.PortalUrlBase,
			cPortalUrlFragment,
			url.PathEscape(*deployment.ID),
		),
	}
}

func convertFromStandardProvisioningState(state armresources.ProvisioningState) DeploymentProvisioningState {
	switch state {
	case armresources.ProvisioningStateAccepted:
		return DeploymentProvisioningStateAccepted
	case armresources.ProvisioningStateCanceled:
		return DeploymentProvisioningStateCanceled
	case armresources.ProvisioningStateCreating:
		return DeploymentProvisioningStateCreating
	case armresources.ProvisioningStateDeleted:
		return DeploymentProvisioningStateDeleted
	case armresources.ProvisioningStateDeleting:
		return DeploymentProvisioningStateDeleting
	case armresources.ProvisioningStateFailed:
		return DeploymentProvisioningStateFailed
	case armresources.ProvisioningStateNotSpecified:
		return DeploymentProvisioningStateNotSpecified
	case armresources.ProvisioningStateReady:
		return DeploymentProvisioningStateReady
	case armresources.ProvisioningStateRunning:
		return DeploymentProvisioningStateRunning
	case armresources.ProvisioningStateSucceeded:
		return DeploymentProvisioningStateSucceeded
	case armresources.ProvisioningStateUpdating:
		return DeploymentProvisioningStateUpdating
	}

	return DeploymentProvisioningState("")
}

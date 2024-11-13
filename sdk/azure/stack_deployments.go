package azure

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armdeploymentstacks"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	"github.com/sethvargo/go-retry"
	"github.com/wbreza/azd-extensions/sdk/common"
	"github.com/wbreza/azd-extensions/sdk/common/convert"
	"github.com/wbreza/azd-extensions/sdk/core/config"
)

const (
	deploymentStacksConfigKey      = "DeploymentStacks"
	stacksPortalUrlFragment        = "#@microsoft.onmicrosoft.com/resource"
	bypassOutOfSyncErrorEnvVarName = "DEPLOYMENT_STACKS_BYPASS_STACK_OUT_OF_SYNC_ERROR"
)

var defaultDeploymentStackOptions = &deploymentStackOptions{
	BypassStackOutOfSyncError: to.Ptr(false),
	ActionOnUnmanage: &armdeploymentstacks.ActionOnUnmanage{
		ManagementGroups: to.Ptr(armdeploymentstacks.DeploymentStacksDeleteDetachEnumDelete),
		ResourceGroups:   to.Ptr(armdeploymentstacks.DeploymentStacksDeleteDetachEnumDelete),
		Resources:        to.Ptr(armdeploymentstacks.DeploymentStacksDeleteDetachEnumDelete),
	},
	DenySettings: &armdeploymentstacks.DenySettings{
		Mode: to.Ptr(armdeploymentstacks.DenySettingsModeNone),
	},
}

type StackDeployments struct {
	credential          azcore.TokenCredential
	armClientOptions    *arm.ClientOptions
	standardDeployments *StandardDeployments
	cloud               *Cloud
}

type deploymentStackOptions struct {
	BypassStackOutOfSyncError *bool                                 `yaml:"bypassStackOutOfSyncError,omitempty"`
	ActionOnUnmanage          *armdeploymentstacks.ActionOnUnmanage `yaml:"actionOnUnmanage,omitempty"`
	DenySettings              *armdeploymentstacks.DenySettings     `yaml:"denySettings,omitempty"`
}

func NewStackDeployments(
	credential azcore.TokenCredential,
	armClientOptions *arm.ClientOptions,
	standardDeployments *StandardDeployments,
	cloud *Cloud,
) *StackDeployments {
	return &StackDeployments{
		credential:          credential,
		armClientOptions:    armClientOptions,
		standardDeployments: standardDeployments,
		cloud:               cloud,
	}
}

func (d *StackDeployments) ListSubscriptionDeployments(
	ctx context.Context,
	subscriptionId string,
) ([]*ResourceDeployment, error) {
	client, err := d.createClient(ctx, subscriptionId)
	if err != nil {
		return nil, err
	}

	results := []*ResourceDeployment{}

	pager := client.NewListAtSubscriptionPager(nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}

		for _, deployment := range page.Value {
			results = append(results, d.convertFromStackDeployment(deployment))
		}
	}

	return results, nil
}

func (d *StackDeployments) GetSubscriptionDeployment(
	ctx context.Context,
	subscriptionId string,
	deploymentName string,
) (*ResourceDeployment, error) {
	client, err := d.createClient(ctx, subscriptionId)
	if err != nil {
		return nil, err
	}

	var deploymentStack *armdeploymentstacks.DeploymentStack

	err = retry.Do(
		ctx,
		retry.WithMaxDuration(10*time.Minute, retry.NewConstant(5*time.Second)),
		func(ctx context.Context) error {
			response, err := client.GetAtSubscription(ctx, deploymentName, nil)
			if err != nil {
				return fmt.Errorf(
					"%w: '%s' in subscription '%s', Error: %w",
					ErrDeploymentNotFound,
					subscriptionId,
					deploymentName,
					err,
				)
			}

			if response.DeploymentStack.Properties.DeploymentID == nil {
				return retry.RetryableError(errors.New("deployment stack is missing ARM deployment id"))
			}

			deploymentStack = &response.DeploymentStack

			return nil
		})

	if err != nil {
		// If a deployment stack is not found with the given name, fallback to check for standard deployments
		if errors.Is(err, ErrDeploymentNotFound) {
			return d.standardDeployments.GetSubscriptionDeployment(ctx, subscriptionId, deploymentName)
		}

		return nil, err
	}

	return d.convertFromStackDeployment(deploymentStack), nil
}

func (d *StackDeployments) ListResourceGroupDeployments(
	ctx context.Context,
	subscriptionId string,
	resourceGroupName string,
) ([]*ResourceDeployment, error) {
	client, err := d.createClient(ctx, subscriptionId)
	if err != nil {
		return nil, err
	}

	results := []*ResourceDeployment{}

	pager := client.NewListAtResourceGroupPager(resourceGroupName, nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}

		for _, deployment := range page.Value {
			results = append(results, d.convertFromStackDeployment(deployment))
		}
	}

	return results, nil
}

func (d *StackDeployments) GetResourceGroupDeployment(
	ctx context.Context,
	subscriptionId string,
	resourceGroupName string,
	deploymentName string,
) (*ResourceDeployment, error) {
	client, err := d.createClient(ctx, subscriptionId)
	if err != nil {
		return nil, err
	}

	var deploymentStack *armdeploymentstacks.DeploymentStack

	err = retry.Do(
		ctx,
		retry.WithMaxDuration(10*time.Minute, retry.NewConstant(5*time.Second)),
		func(ctx context.Context) error {
			response, err := client.GetAtResourceGroup(ctx, resourceGroupName, deploymentName, nil)
			if err != nil {
				return fmt.Errorf(
					"%w: '%s' in resource group '%s', Error: %w",
					ErrDeploymentNotFound,
					resourceGroupName,
					deploymentName,
					err,
				)
			}

			if response.DeploymentStack.Properties.DeploymentID == nil {
				return retry.RetryableError(errors.New("deployment stack is missing ARM deployment id"))
			}

			deploymentStack = &response.DeploymentStack

			return nil
		})

	if err != nil {
		// If a deployment stack is not found with the given name, fallback to check for standard deployments
		if errors.Is(err, ErrDeploymentNotFound) {
			return d.standardDeployments.GetResourceGroupDeployment(ctx, subscriptionId, resourceGroupName, deploymentName)
		}

		return nil, err
	}

	return d.convertFromStackDeployment(deploymentStack), nil
}

func (d *StackDeployments) ListSubscriptionDeploymentOperations(
	ctx context.Context,
	subscriptionId string,
	deploymentName string,
) ([]*armresources.DeploymentOperation, error) {
	deployment, err := d.GetSubscriptionDeployment(ctx, subscriptionId, deploymentName)
	if err != nil && !errors.Is(err, ErrDeploymentNotFound) {
		return nil, err
	}

	if deployment != nil && deployment.DeploymentId != "" {
		deploymentName = filepath.Base(deployment.DeploymentId)
	}

	return d.standardDeployments.ListSubscriptionDeploymentOperations(ctx, subscriptionId, deploymentName)
}

func (d *StackDeployments) ListResourceGroupDeploymentOperations(
	ctx context.Context,
	subscriptionId string,
	resourceGroupName string,
	deploymentName string,
) ([]*armresources.DeploymentOperation, error) {
	// The requested deployment name may be an inner deployment which will not be found in the deployment stacks.
	// If this is the case continue on checking if there is a stack deployment
	// If a deployment stack is found then use the deployment id of the stack
	deployment, err := d.GetResourceGroupDeployment(ctx, subscriptionId, resourceGroupName, deploymentName)
	if err != nil && !errors.Is(err, ErrDeploymentNotFound) {
		return nil, err
	}

	if deployment != nil && deployment.DeploymentId != "" {
		deploymentName = filepath.Base(deployment.DeploymentId)
	}

	return d.standardDeployments.ListResourceGroupDeploymentOperations(
		ctx,
		subscriptionId,
		resourceGroupName,
		deploymentName,
	)
}

// parseDeploymentStackOptions parses the deployment stack options from the given options map.
// If the options map is nil, the default deployment stack options are returned.
// Default deployment stack options are:
// - BypassStackOutOfSyncError: false
// - ActionOnUnmanage: Delete for all
// - DenySettings: nil
func parseDeploymentStackOptions(options map[string]any) (*deploymentStackOptions, error) {
	bypassStackOutOfSyncErrorVal, hasBypassStackOutOfSyncError := os.LookupEnv(bypassOutOfSyncErrorEnvVarName)

	if options == nil && !hasBypassStackOutOfSyncError {
		return defaultDeploymentStackOptions, nil
	}

	optionsConfig := config.NewConfig(options)

	var deploymentStackOptions *deploymentStackOptions
	hasDeploymentStacksConfig, err := optionsConfig.GetSection(deploymentStacksConfigKey, &deploymentStackOptions)
	if err != nil {
		suggestion := &common.ErrorWithSuggestion{
			Err:        fmt.Errorf("failed parsing deployment stack options: %w", err),
			Suggestion: "Review the 'infra.deploymentStacks' configuration section in the 'azure.yaml' file.",
		}

		return nil, suggestion
	}

	if !hasBypassStackOutOfSyncError && (!hasDeploymentStacksConfig || deploymentStackOptions == nil) {
		return defaultDeploymentStackOptions, nil
	}

	if deploymentStackOptions == nil {
		deploymentStackOptions = defaultDeploymentStackOptions
	}

	// The BypassStackOutOfSyncError will NOT be exposed in the `azure.yaml` for configuration
	// since this option will typically only be used on a per call basis.
	// The value will be read from the environment variable `DEPLOYMENT_STACKS_BYPASS_STACK_OUT_OF_SYNC_ERROR`
	// If the value is a truthy value, the value will be set to true, otherwise it will be set to false (default)
	if hasBypassStackOutOfSyncError {
		byPassOutOfSyncError, err := strconv.ParseBool(bypassStackOutOfSyncErrorVal)
		if err != nil {
			log.Printf(
				"Failed to parse environment variable '%s' value '%s' as a boolean. Defaulting to false.",
				bypassOutOfSyncErrorEnvVarName,
				bypassStackOutOfSyncErrorVal,
			)
		} else {
			deploymentStackOptions.BypassStackOutOfSyncError = &byPassOutOfSyncError
		}
	}

	if deploymentStackOptions.BypassStackOutOfSyncError == nil {
		deploymentStackOptions.BypassStackOutOfSyncError = defaultDeploymentStackOptions.BypassStackOutOfSyncError
	}

	if deploymentStackOptions.ActionOnUnmanage == nil {
		deploymentStackOptions.ActionOnUnmanage = defaultDeploymentStackOptions.ActionOnUnmanage
	}

	if deploymentStackOptions.DenySettings == nil {
		deploymentStackOptions.DenySettings = defaultDeploymentStackOptions.DenySettings
	}

	return deploymentStackOptions, nil
}

func (d *StackDeployments) createClient(ctx context.Context, subscriptionId string) (*armdeploymentstacks.Client, error) {
	return armdeploymentstacks.NewClient(subscriptionId, d.credential, d.armClientOptions)
}

// Converts from an ARM Extended Deployment to Azd Generic deployment
func (d *StackDeployments) convertFromStackDeployment(deployment *armdeploymentstacks.DeploymentStack) *ResourceDeployment {
	resources := []*armresources.ResourceReference{}
	for _, resource := range deployment.Properties.Resources {
		resources = append(resources, &armresources.ResourceReference{ID: resource.ID})
	}

	deploymentId := convert.ToValueWithDefault(deployment.Properties.DeploymentID, "")

	return &ResourceDeployment{
		Id:                *deployment.ID,
		Location:          convert.ToValueWithDefault(deployment.Location, ""),
		DeploymentId:      deploymentId,
		Name:              *deployment.Name,
		Type:              *deployment.Type,
		Tags:              deployment.Tags,
		ProvisioningState: convertFromStacksProvisioningState(*deployment.Properties.ProvisioningState),
		Timestamp:         *deployment.SystemData.LastModifiedAt,
		Outputs:           deployment.Properties.Outputs,
		Resources:         resources,
		Dependencies:      []*armresources.Dependency{},

		PortalUrl: fmt.Sprintf("%s/%s/%s",
			d.cloud.PortalUrlBase,
			stacksPortalUrlFragment,
			*deployment.ID,
		),

		OutputsUrl: fmt.Sprintf("%s/%s/%s/outputs",
			d.cloud.PortalUrlBase,
			stacksPortalUrlFragment,
			*deployment.ID,
		),

		DeploymentUrl: fmt.Sprintf("%s/%s/%s",
			d.cloud.PortalUrlBase,
			cPortalUrlFragment,
			url.PathEscape(deploymentId),
		),
	}
}

func convertFromStacksProvisioningState(
	state armdeploymentstacks.DeploymentStackProvisioningState,
) DeploymentProvisioningState {
	switch state {
	case armdeploymentstacks.DeploymentStackProvisioningStateCanceled:
		return DeploymentProvisioningStateCanceled
	case armdeploymentstacks.DeploymentStackProvisioningStateCanceling:
		return DeploymentProvisioningStateCanceling
	case armdeploymentstacks.DeploymentStackProvisioningStateCreating:
		return DeploymentProvisioningStateCreating
	case armdeploymentstacks.DeploymentStackProvisioningStateDeleting:
		return DeploymentProvisioningStateDeleting
	case armdeploymentstacks.DeploymentStackProvisioningStateDeletingResources:
		return DeploymentProvisioningStateDeletingResources
	case armdeploymentstacks.DeploymentStackProvisioningStateDeploying:
		return DeploymentProvisioningStateDeploying
	case armdeploymentstacks.DeploymentStackProvisioningStateFailed:
		return DeploymentProvisioningStateFailed
	case armdeploymentstacks.DeploymentStackProvisioningStateSucceeded:
		return DeploymentProvisioningStateSucceeded
	case armdeploymentstacks.DeploymentStackProvisioningStateUpdatingDenyAssignments:
		return DeploymentProvisioningStateUpdatingDenyAssignments
	case armdeploymentstacks.DeploymentStackProvisioningStateValidating:
		return DeploymentProvisioningStateValidating
	case armdeploymentstacks.DeploymentStackProvisioningStateWaiting:
		return DeploymentProvisioningStateWaiting
	}

	return DeploymentProvisioningState("")
}

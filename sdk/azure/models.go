package azure

import (
	"errors"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
)

type ResourceType string

const (
	ResourceTypeApim                      ResourceType = "Microsoft.ApiManagement/service"
	ResourceTypeAppConfig                 ResourceType = "Microsoft.AppConfiguration/configurationStores"
	ResourceTypeAppInsightComponent       ResourceType = "Microsoft.Insights/components"
	ResourceTypeCacheForRedis             ResourceType = "Microsoft.Cache/redis"
	ResourceTypeCDNProfile                ResourceType = "Microsoft.Cdn/profiles"
	ResourceTypeCosmosDb                  ResourceType = "Microsoft.DocumentDB/databaseAccounts"
	ResourceTypeContainerApp              ResourceType = "Microsoft.App/containerApps"
	ResourceTypeSpringApp                 ResourceType = "Microsoft.AppPlatform/Spring"
	ResourceTypeContainerAppEnvironment   ResourceType = "Microsoft.App/managedEnvironments"
	ResourceTypeDeployment                ResourceType = "Microsoft.Resources/deployments"
	ResourceTypeKeyVault                  ResourceType = "Microsoft.KeyVault/vaults"
	ResourceTypeManagedHSM                ResourceType = "Microsoft.KeyVault/managedHSMs"
	ResourceTypeLoadTest                  ResourceType = "Microsoft.LoadTestService/loadTests"
	ResourceTypeLogAnalyticsWorkspace     ResourceType = "Microsoft.OperationalInsights/workspaces"
	ResourceTypePortalDashboard           ResourceType = "Microsoft.Portal/dashboards"
	ResourceTypePostgreSqlServer          ResourceType = "Microsoft.DBforPostgreSQL/flexibleServers"
	ResourceTypeMySqlServer               ResourceType = "Microsoft.DBforMySQL/flexibleServers"
	ResourceTypeResourceGroup             ResourceType = "Microsoft.Resources/resourceGroups"
	ResourceTypeStorageAccount            ResourceType = "Microsoft.Storage/storageAccounts"
	ResourceTypeStaticWebSite             ResourceType = "Microsoft.Web/staticSites"
	ResourceTypeServiceBusNamespace       ResourceType = "Microsoft.ServiceBus/namespaces"
	ResourceTypeServicePlan               ResourceType = "Microsoft.Web/serverfarms"
	ResourceTypeSqlServer                 ResourceType = "Microsoft.Sql/servers"
	ResourceTypeVirtualNetwork            ResourceType = "Microsoft.Network/virtualNetworks"
	ResourceTypeWebSite                   ResourceType = "Microsoft.Web/sites"
	ResourceTypeContainerRegistry         ResourceType = "Microsoft.ContainerRegistry/registries"
	ResourceTypeManagedCluster            ResourceType = "Microsoft.ContainerService/managedClusters"
	ResourceTypeAgentPool                 ResourceType = "Microsoft.ContainerService/managedClusters/agentPools"
	ResourceTypeCognitiveServiceAccount   ResourceType = "Microsoft.CognitiveServices/accounts"
	ResourceTypeSearchService             ResourceType = "Microsoft.Search/searchServices"
	ResourceTypeVideoIndexer              ResourceType = "Microsoft.VideoIndexer/accounts"
	ResourceTypePrivateEndpoint           ResourceType = "Microsoft.Network/privateEndpoints"
	ResourceTypeDevCenter                 ResourceType = "Microsoft.DevCenter/devcenters"
	ResourceTypeDevCenterProject          ResourceType = "Microsoft.DevCenter/projects"
	ResourceTypeMachineLearningWorkspace  ResourceType = "Microsoft.MachineLearningServices/workspaces"
	ResourceTypeMachineLearningConnection ResourceType = "Microsoft.MachineLearningServices/workspaces/connections"

	//nolint:lll
	ResourceTypeMachineLearningEndpoint           ResourceType = "Microsoft.MachineLearningServices/workspaces/onlineEndpoints"
	ResourceTypeCognitiveServiceAccountDeployment ResourceType = "Microsoft.CognitiveServices/accounts/deployments"
)

type Subscription struct {
	Id       string `json:"id"`
	Name     string `json:"name"`
	TenantId string `json:"tenantId"`
	// The tenant under which the user has access to the subscription.
	UserAccessTenantId string `json:"userAccessTenantId"`
	IsDefault          bool   `json:"isDefault,omitempty"`
}

type Location struct {
	// The name of the location (e.g. "westus2")
	Name string `json:"name"`
	// The human friendly name of the location (e.g. "West US 2")
	DisplayName string `json:"displayName"`
	// The human friendly name of the location, prefixed with a
	// region name (e.g "(US) West US 2")
	RegionalDisplayName string `json:"regionalDisplayName"`
}

type ResourceGroup struct {
	Id       string `json:"id"`
	Name     string `json:"name"`
	Location string `json:"location"`
}

type Resource struct {
	Id       string `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Location string `json:"location"`
}

type ResourceExtended struct {
	Resource
	Kind string `json:"kind"`
}

var (
	ErrDeploymentNotFound = errors.New("deployment not found")
)

type ResourceDeployment struct {
	// The Azure resource id of the deployment operation
	Id string

	// The location of the deployment
	Location string

	// The Azure resource id of the actual deployment object
	DeploymentId string

	// The deployment name
	Name string

	// The deployment type
	Type string

	// The tags associated with the deployment
	Tags map[string]*string

	// The outputs from the deployment
	Outputs any

	// The hash produced for the template.
	TemplateHash *string

	// The timestamp of the template deployment.
	Timestamp time.Time

	// The resources created from the deployment
	Resources []*armresources.ResourceReference

	// The dependencies of the deployment
	Dependencies []*armresources.Dependency

	// The status of the deployment
	ProvisioningState DeploymentProvisioningState

	PortalUrl string

	OutputsUrl string

	DeploymentUrl string
}

type DeploymentProvisioningState string

const (
	DeploymentProvisioningStateAccepted                DeploymentProvisioningState = "Accepted"
	DeploymentProvisioningStateCanceled                DeploymentProvisioningState = "Canceled"
	DeploymentProvisioningStateCanceling               DeploymentProvisioningState = "Canceling"
	DeploymentProvisioningStateCreating                DeploymentProvisioningState = "Creating"
	DeploymentProvisioningStateDeleted                 DeploymentProvisioningState = "Deleted"
	DeploymentProvisioningStateDeleting                DeploymentProvisioningState = "Deleting"
	DeploymentProvisioningStateDeletingResources       DeploymentProvisioningState = "DeletingResources"
	DeploymentProvisioningStateDeploying               DeploymentProvisioningState = "Deploying"
	DeploymentProvisioningStateFailed                  DeploymentProvisioningState = "Failed"
	DeploymentProvisioningStateNotSpecified            DeploymentProvisioningState = "NotSpecified"
	DeploymentProvisioningStateReady                   DeploymentProvisioningState = "Ready"
	DeploymentProvisioningStateRunning                 DeploymentProvisioningState = "Running"
	DeploymentProvisioningStateSucceeded               DeploymentProvisioningState = "Succeeded"
	DeploymentProvisioningStateUpdatingDenyAssignments DeploymentProvisioningState = "UpdatingDenyAssignments"
	DeploymentProvisioningStateValidating              DeploymentProvisioningState = "Validating"
	DeploymentProvisioningStateWaiting                 DeploymentProvisioningState = "Waiting"
	DeploymentProvisioningStateUpdating                DeploymentProvisioningState = "Updating"
)

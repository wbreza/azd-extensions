package azure

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/authorization/armauthorization/v3"
	"github.com/google/uuid"
)

type RoleName string

const (
	RoleDefinitionStorageBlobDataContributor RoleName = "Storage Blob Data Contributor"
	RoleCognitiveServicesOpenAIContributor   RoleName = "Cognitive Services OpenAI Contributor"
	RoleSearchIndexDataContributor           RoleName = "Search Index Data Contributor"
	RoleSearchServiceContributor             RoleName = "Search Service Contributor"
)

type EntraIdService struct {
	credential       azcore.TokenCredential
	armClientOptions *arm.ClientOptions
}

func NewEntraIdService(
	credential azcore.TokenCredential,
	armClientOptions *arm.ClientOptions,
) *EntraIdService {
	return &EntraIdService{
		credential:       credential,
		armClientOptions: armClientOptions,
	}
}

func (eis *EntraIdService) EnsureRoleAssignment(ctx context.Context, subscriptionId string, scope string, principalId string, roleNames ...RoleName) error {
	authClient, err := armauthorization.NewClientFactory(subscriptionId, eis.credential, eis.armClientOptions)
	if err != nil {
		return err
	}

	rbacClient := authClient.NewRoleAssignmentsClient()

	for _, roleName := range roleNames {
		roleDefinitionId, err := eis.getRoleDefinitionId(ctx, subscriptionId, roleName)
		if err != nil {
			return err
		}

		_, err = rbacClient.Create(ctx, scope, uuid.New().String(), armauthorization.RoleAssignmentCreateParameters{
			Properties: &armauthorization.RoleAssignmentProperties{
				PrincipalID:      &principalId,
				RoleDefinitionID: &roleDefinitionId,
			},
		}, nil)

		if err != nil {
			var responseError *azcore.ResponseError

			// If the response is a 409 conflict then the role has already been assigned.
			if errors.As(err, &responseError) && responseError.StatusCode == http.StatusConflict {
				return nil
			}

			return err
		}
	}

	return nil
}

func (eis *EntraIdService) getRoleDefinitionId(ctx context.Context, subscriptionId string, roleName RoleName) (string, error) {
	authClient, err := armauthorization.NewClientFactory(subscriptionId, eis.credential, eis.armClientOptions)
	if err != nil {
		return "", err
	}

	rolesClient := authClient.NewRoleDefinitionsClient()
	scope := fmt.Sprintf("/subscriptions/%s", subscriptionId)

	pager := rolesClient.NewListPager(scope, &armauthorization.RoleDefinitionsClientListOptions{
		Filter: to.Ptr(fmt.Sprintf("roleName eq '%s'", roleName)),
	})

	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return "", err
		}

		for _, roleDefinition := range page.Value {
			if strings.EqualFold(*roleDefinition.Properties.RoleName, string(roleName)) {
				return *roleDefinition.ID, nil
			}
		}
	}

	return "", fmt.Errorf("role definition not found for role name: %s", roleName)
}

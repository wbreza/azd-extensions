package azure

import (
	"context"
	"errors"
	"net/http"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/authorization/armauthorization/v3"
	"github.com/google/uuid"
)

const (
	RoleDefinitionStorageBlobDataContributor string = "ba92f5b4-2d11-453d-a403-e96b0029c9fe"
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

func (eis *EntraIdService) EnsureRoleAssignment(ctx context.Context, subscriptionId string, scope string, principalId string, roleDefinitionId string) error {
	authClient, err := armauthorization.NewClientFactory(subscriptionId, eis.credential, eis.armClientOptions)
	if err != nil {
		return err
	}

	rbacClient := authClient.NewRoleAssignmentsClient()

	_, err = rbacClient.Create(ctx, scope, uuid.New().String(), armauthorization.RoleAssignmentCreateParameters{
		Properties: &armauthorization.RoleAssignmentProperties{
			PrincipalID:      &principalId,
			RoleDefinitionID: &roleDefinitionId,
			Scope:            &scope,
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

	return nil
}

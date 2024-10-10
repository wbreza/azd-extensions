package account

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/wbreza/azd-extensions/sdk/azure"
)

type Account azure.TokenClaims

var credential azcore.TokenCredential
var currentUser *Account

func Credential() (azcore.TokenCredential, error) {
	if credential == nil {
		azdCredential, err := azidentity.NewAzureDeveloperCLICredential(nil)
		if err != nil {
			return nil, err
		}

		credential = azdCredential
	}

	return credential, nil
}

func CurrentPrincipal(ctx context.Context) (*Account, error) {
	if currentUser != nil {
		return currentUser, nil
	}

	credential, err := Credential()
	if err != nil {
		return nil, err
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

	account := Account(claims)

	return &account, nil
}

package internal

import (
	"context"
	"errors"
	"fmt"

	"dario.cat/mergo"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/cognitiveservices/armcognitiveservices"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storage/armstorage"
	"github.com/fatih/color"
	"github.com/wbreza/azd-extensions/sdk/azure"
	"github.com/wbreza/azd-extensions/sdk/ext"
	"github.com/wbreza/azd-extensions/sdk/ext/prompt"
	"github.com/wbreza/azd-extensions/sdk/ux"
)

func PromptAccount(ctx context.Context, azdContext *ext.Context, aiConfig *AiConfig) (*armcognitiveservices.Account, error) {
	subscription, err := LoadOrPromptSubscription(ctx, azdContext, aiConfig)
	if err != nil {
		return nil, err
	}

	credential, err := azdContext.Credential()
	if err != nil {
		return nil, err
	}

	accountsClient, err := armcognitiveservices.NewAccountsClient(subscription.Id, credential, nil)
	if err != nil {
		return nil, err
	}

	var aiService *armcognitiveservices.Account

	selectedResource, err := prompt.PromptSubscriptionResource(ctx, subscription, prompt.PromptResourceOptions{
		ResourceType:            to.Ptr(azure.ResourceTypeCognitiveServiceAccount),
		Kinds:                   []string{"OpenAI", "AIService", "CognitiveServices"},
		ResourceTypeDisplayName: "Azure AI service",
		CreateResource: func(ctx context.Context) (*azure.ResourceExtended, error) {
			resourceGroup, err := LoadOrPromptResourceGroup(ctx, azdContext, aiConfig)
			if err != nil {
				return nil, err
			}

			namePrompt := ux.NewPrompt(&ux.PromptConfig{
				Message: "Enter the name for the Azure AI service account",
			})

			accountName, err := namePrompt.Ask()
			if err != nil {
				return nil, err
			}

			taskName := fmt.Sprintf("Creating Azure AI service account %s", color.CyanString(accountName))

			fmt.Println()
			err = ux.NewTaskList(nil).AddTask(ux.TaskOptions{
				Title: taskName,
				Action: func() (ux.TaskState, error) {
					account := armcognitiveservices.Account{
						Name: &accountName,
						Identity: &armcognitiveservices.Identity{
							Type: to.Ptr(armcognitiveservices.ResourceIdentityTypeSystemAssigned),
						},
						Location: &resourceGroup.Location,
						Kind:     to.Ptr("OpenAI"),
						SKU: &armcognitiveservices.SKU{
							Name: to.Ptr("S0"),
						},
						Properties: &armcognitiveservices.AccountProperties{
							CustomSubDomainName: &accountName,
							PublicNetworkAccess: to.Ptr(armcognitiveservices.PublicNetworkAccessEnabled),
							DisableLocalAuth:    to.Ptr(false),
						},
					}

					poller, err := accountsClient.BeginCreate(ctx, resourceGroup.Name, accountName, account, nil)
					if err != nil {
						return ux.Error, err
					}

					accountResponse, err := poller.PollUntilDone(ctx, nil)
					if err != nil {
						return ux.Error, err
					}

					aiService = &accountResponse.Account

					return ux.Success, nil
				},
			}).Run()

			if err != nil {
				return nil, err
			}

			return &azure.ResourceExtended{
				Resource: azure.Resource{
					Id:       *aiService.ID,
					Name:     *aiService.Name,
					Type:     *aiService.Type,
					Location: *aiService.Location,
				},
				Kind: *aiService.Kind,
			}, nil
		},
	})
	if err != nil {
		return nil, err
	}

	if aiService == nil {
		parsedResource, err := arm.ParseResourceID(selectedResource.Id)
		if err != nil {
			return nil, err
		}

		existingAccount, err := accountsClient.Get(ctx, parsedResource.ResourceGroupName, parsedResource.Name, nil)
		if err != nil {
			return nil, err
		}

		aiService = &existingAccount.Account
	}

	return aiService, nil
}

func PromptModelDeployment(ctx context.Context, azdContext *ext.Context, aiConfig *AiConfig, options *prompt.PromptSelectOptions) (*armcognitiveservices.Deployment, error) {
	mergedOptions := &prompt.PromptSelectOptions{}

	if options == nil {
		options = &prompt.PromptSelectOptions{}
	}

	defaultOptions := &prompt.PromptSelectOptions{
		Message:            "Select an AI model deployment",
		LoadingMessage:     "Loading model deployments...",
		AllowNewResource:   to.Ptr(true),
		NewResourceMessage: "Deploy a new AI model",
		CreatingMessage:    "Deploying AI model...",
	}

	mergo.Merge(mergedOptions, options, mergo.WithoutDereference)
	mergo.Merge(mergedOptions, defaultOptions, mergo.WithoutDereference)

	credential, err := azdContext.Credential()
	if err != nil {
		return nil, err
	}

	deploymentsClient, err := armcognitiveservices.NewDeploymentsClient(aiConfig.Subscription, credential, nil)
	if err != nil {
		return nil, err
	}

	return prompt.PromptCustomResource(ctx, prompt.PromptCustomResourceOptions[armcognitiveservices.Deployment]{
		SelectorOptions: mergedOptions,
		LoadData: func(ctx context.Context) ([]*armcognitiveservices.Deployment, error) {
			deploymentList := []*armcognitiveservices.Deployment{}
			modelPager := deploymentsClient.NewListPager(aiConfig.ResourceGroup, aiConfig.Service, nil)
			for modelPager.More() {
				models, err := modelPager.NextPage(ctx)
				if err != nil {
					return nil, err
				}

				deploymentList = append(deploymentList, models.Value...)
			}

			return deploymentList, nil
		},
		DisplayResource: func(resource *armcognitiveservices.Deployment) (string, error) {
			return fmt.Sprintf("%s (Model: %s, Version: %s)", *resource.Name, *resource.Properties.Model.Name, *resource.Properties.Model.Version), nil
		},
		CreateResource: func(ctx context.Context) (*armcognitiveservices.Deployment, error) {
			selectedModel, err := PromptModel(ctx, azdContext, aiConfig)
			if err != nil {
				return nil, err
			}

			selectedSku, err := PromptModelSku(ctx, azdContext, aiConfig, selectedModel)
			if err != nil {
				return nil, err
			}
			var deploymentName string

			namePrompt := ux.NewPrompt(&ux.PromptConfig{
				Message:      "Enter the name for the deployment",
				DefaultValue: *selectedModel.Model.Name,
			})

			deploymentName, err = namePrompt.Ask()
			if err != nil {
				return nil, err
			}

			deployment := armcognitiveservices.Deployment{
				Name: &deploymentName,
				SKU: &armcognitiveservices.SKU{
					Name:     selectedSku.Name,
					Capacity: selectedSku.Capacity.Default,
				},
				Properties: &armcognitiveservices.DeploymentProperties{
					Model: &armcognitiveservices.DeploymentModel{
						Format:  selectedModel.Model.Format,
						Name:    selectedModel.Model.Name,
						Version: selectedModel.Model.Version,
					},
					RaiPolicyName:        to.Ptr("Microsoft.DefaultV2"),
					VersionUpgradeOption: to.Ptr(armcognitiveservices.DeploymentModelVersionUpgradeOptionOnceNewDefaultVersionAvailable),
				},
			}

			var modelDeployment *armcognitiveservices.Deployment

			taskName := fmt.Sprintf("Creating model deployment %s", color.CyanString(deploymentName))

			fmt.Println()
			err = ux.NewTaskList(nil).
				AddTask(ux.TaskOptions{
					Title: taskName,
					Action: func() (ux.TaskState, error) {
						existingDeployment, err := deploymentsClient.Get(ctx, aiConfig.ResourceGroup, aiConfig.Service, deploymentName, nil)
						if err == nil && *existingDeployment.Name == deploymentName {
							return ux.Error, errors.New("deployment with the same name already exists")
						}

						poller, err := deploymentsClient.BeginCreateOrUpdate(ctx, aiConfig.ResourceGroup, aiConfig.Service, deploymentName, deployment, nil)
						if err != nil {
							return ux.Error, err
						}

						deploymentResponse, err := poller.PollUntilDone(ctx, nil)
						if err != nil {
							return ux.Error, err
						}

						modelDeployment = &deploymentResponse.Deployment
						return ux.Success, nil
					},
				}).
				Run()

			if err != nil {
				return nil, err
			}

			return modelDeployment, nil
		},
	})
}

func PromptModel(ctx context.Context, azdContext *ext.Context, aiConfig *AiConfig) (*armcognitiveservices.Model, error) {
	return prompt.PromptCustomResource(ctx, prompt.PromptCustomResourceOptions[armcognitiveservices.Model]{
		SelectorOptions: &prompt.PromptSelectOptions{
			Message:          "Select a model",
			LoadingMessage:   "Loading models...",
			AllowNewResource: to.Ptr(false),
		},
		LoadData: func(ctx context.Context) ([]*armcognitiveservices.Model, error) {
			credential, err := azdContext.Credential()
			if err != nil {
				return nil, err
			}

			clientFactory, err := armcognitiveservices.NewClientFactory(aiConfig.Subscription, credential, nil)
			if err != nil {
				return nil, err
			}

			accountsClient := clientFactory.NewAccountsClient()
			modelsClient := clientFactory.NewModelsClient()

			aiService, err := accountsClient.Get(ctx, aiConfig.ResourceGroup, aiConfig.Service, nil)
			if err != nil {
				return nil, err
			}

			models := []*armcognitiveservices.Model{}

			modelPager := modelsClient.NewListPager(*aiService.Location, nil)
			for modelPager.More() {
				pageResponse, err := modelPager.NextPage(ctx)
				if err != nil {
					return nil, err
				}

				for _, model := range pageResponse.Value {
					if *model.Kind == *aiService.Kind {
						models = append(models, model)
					}
				}
			}

			return models, nil
		},
		DisplayResource: func(model *armcognitiveservices.Model) (string, error) {
			return fmt.Sprintf("%s (Version: %s)", *model.Model.Name, *model.Model.Version), nil
		},
	})
}

func PromptModelSku(ctx context.Context, azdContext *ext.Context, aiConfig *AiConfig, model *armcognitiveservices.Model) (*armcognitiveservices.ModelSKU, error) {
	return prompt.PromptCustomResource(ctx, prompt.PromptCustomResourceOptions[armcognitiveservices.ModelSKU]{
		SelectorOptions: &prompt.PromptSelectOptions{
			Message:          "Select a deployment type",
			LoadingMessage:   "Loading deployment types...",
			AllowNewResource: to.Ptr(false),
		},
		LoadData: func(ctx context.Context) ([]*armcognitiveservices.ModelSKU, error) {
			return model.Model.SKUs, nil
		},
		DisplayResource: func(sku *armcognitiveservices.ModelSKU) (string, error) {
			return *sku.Name, nil
		},
	})
}

func PromptStorage(ctx context.Context, azdContext *ext.Context, aiConfig *AiConfig) (*armstorage.Account, error) {
	credential, err := azdContext.Credential()
	if err != nil {
		return nil, err
	}

	principal, err := azdContext.Principal(ctx)
	if err != nil {
		return nil, err
	}

	accountsClient, err := armstorage.NewAccountsClient(aiConfig.Subscription, credential, nil)
	if err != nil {
		return nil, err
	}

	return prompt.PromptCustomResource(ctx, prompt.PromptCustomResourceOptions[armstorage.Account]{
		SelectorOptions: &prompt.PromptSelectOptions{
			Message:            "Select a storage account",
			LoadingMessage:     "Loading storage accounts...",
			AllowNewResource:   to.Ptr(true),
			NewResourceMessage: "Create a new storage account",
			CreatingMessage:    "Creating storage account...",
		},
		LoadData: func(ctx context.Context) ([]*armstorage.Account, error) {
			storageAccounts := []*armstorage.Account{}

			pager := accountsClient.NewListPager(nil)
			for pager.More() {
				pageResponse, err := pager.NextPage(ctx)
				if err != nil {
					return nil, err
				}

				storageAccounts = append(storageAccounts, pageResponse.Value...)
			}

			return storageAccounts, nil
		},
		DisplayResource: func(resource *armstorage.Account) (string, error) {
			return *resource.Name, nil
		},
		CreateResource: func(ctx context.Context) (*armstorage.Account, error) {
			resourceGroup, err := LoadOrPromptResourceGroup(ctx, azdContext, aiConfig)
			if err != nil {
				return nil, err
			}

			namePrompt := ux.NewPrompt(&ux.PromptConfig{
				Message: "Enter the name for the storage account",
			})

			accountName, err := namePrompt.Ask()
			if err != nil {
				return nil, err
			}

			taskName := fmt.Sprintf("Creating storage account %s", color.CyanString(accountName))

			var storageAccount *armstorage.Account

			fmt.Println()
			err = ux.NewTaskList(nil).
				AddTask(ux.TaskOptions{
					Title: taskName,
					Action: func() (ux.TaskState, error) {
						accountCreateParams := armstorage.AccountCreateParameters{
							Location: &resourceGroup.Location,
							SKU: &armstorage.SKU{
								Name: to.Ptr(armstorage.SKUNameStandardLRS),
							},
							Kind: to.Ptr(armstorage.KindStorageV2),
							Properties: &armstorage.AccountPropertiesCreateParameters{
								AccessTier:            to.Ptr(armstorage.AccessTierHot),
								AllowBlobPublicAccess: to.Ptr(true),
								MinimumTLSVersion:     to.Ptr(armstorage.MinimumTLSVersionTLS12),
								PublicNetworkAccess:   to.Ptr(armstorage.PublicNetworkAccessEnabled),
							},
						}

						poller, err := accountsClient.BeginCreate(ctx, resourceGroup.Name, accountName, accountCreateParams, nil)
						if err != nil {
							return ux.Error, err
						}

						createResponse, err := poller.PollUntilDone(ctx, nil)
						if err != nil {
							return ux.Error, err
						}

						storageAccount = &createResponse.Account
						return ux.Success, nil
					},
				}).
				AddTask(ux.TaskOptions{
					Title: "Assigning Storage Blob Data Contributor role",
					Action: func() (ux.TaskState, error) {
						rbacClient := azure.NewEntraIdService(credential, nil)
						err := rbacClient.EnsureRoleAssignment(ctx, aiConfig.Subscription, *storageAccount.ID, principal.Oid, azure.RoleDefinitionStorageBlobDataContributor)
						if err != nil {
							return ux.Error, err
						}

						if err = rbacClient.EnsureRoleAssignment(
							ctx,
							aiConfig.Subscription,
							*storageAccount.ID,
							principal.Oid,
							azure.RoleDefinitionStorageBlobDataContributor,
						); err != nil {
							return ux.Error, err
						}

						return ux.Success, nil
					},
				}).
				Run()

			if err != nil {
				return nil, err
			}

			return storageAccount, nil
		},
	})
}

func PromptStorageContainer(ctx context.Context, azdContext *ext.Context, aiConfig *AiConfig) (*armstorage.BlobContainer, error) {
	credential, err := azdContext.Credential()
	if err != nil {
		return nil, err
	}

	containersClient, err := armstorage.NewBlobContainersClient(aiConfig.Subscription, credential, nil)
	if err != nil {
		return nil, err
	}

	return prompt.PromptCustomResource(ctx, prompt.PromptCustomResourceOptions[armstorage.BlobContainer]{
		SelectorOptions: &prompt.PromptSelectOptions{
			Message:            "Select a blob container",
			LoadingMessage:     "Loading blob containers...",
			AllowNewResource:   to.Ptr(true),
			NewResourceMessage: "Create a new blob container",
			CreatingMessage:    "Creating blob container...",
		},
		LoadData: func(ctx context.Context) ([]*armstorage.BlobContainer, error) {
			blobContainers := []*armstorage.BlobContainer{}

			pager := containersClient.NewListPager(aiConfig.ResourceGroup, aiConfig.StorageAccount, nil)
			for pager.More() {
				pageResponse, err := pager.NextPage(ctx)
				if err != nil {
					return nil, err
				}

				for _, container := range pageResponse.Value {
					blobContainers = append(blobContainers, &armstorage.BlobContainer{
						ID:                  container.ID,
						Name:                container.Name,
						Type:                container.Type,
						Etag:                container.Etag,
						ContainerProperties: container.Properties,
					})
				}
			}

			return blobContainers, nil
		},
		DisplayResource: func(resource *armstorage.BlobContainer) (string, error) {
			return *resource.Name, nil
		},
		CreateResource: func(ctx context.Context) (*armstorage.BlobContainer, error) {
			namePrompt := ux.NewPrompt(&ux.PromptConfig{
				Message: "Enter the name for the blob container",
			})

			containerName, err := namePrompt.Ask()
			if err != nil {
				return nil, err
			}

			taskName := fmt.Sprintf("Creating blob container %s", color.CyanString(containerName))

			var blobContainer *armstorage.BlobContainer

			fmt.Println()
			err = ux.NewTaskList(nil).
				AddTask(ux.TaskOptions{
					Title: taskName,
					Action: func() (ux.TaskState, error) {
						newContainer := armstorage.BlobContainer{
							Name: &containerName,
							ContainerProperties: &armstorage.ContainerProperties{
								PublicAccess: to.Ptr(armstorage.PublicAccessNone),
							},
						}

						createResponse, err := containersClient.Create(ctx, aiConfig.ResourceGroup, aiConfig.StorageAccount, containerName, newContainer, nil)
						if err != nil {
							return ux.Error, err
						}

						blobContainer = &createResponse.BlobContainer
						return ux.Success, nil
					},
				}).
				Run()

			if err != nil {
				return nil, err
			}

			return blobContainer, nil
		},
	})
}

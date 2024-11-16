package internal

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"dario.cat/mergo"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/cognitiveservices/armcognitiveservices"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/search/armsearch"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storage/armstorage"
	"github.com/fatih/color"
	"github.com/wbreza/azd-extensions/sdk/azure"
	"github.com/wbreza/azd-extensions/sdk/common"
	"github.com/wbreza/azd-extensions/sdk/ext"
	"github.com/wbreza/azd-extensions/sdk/ux"
	"github.com/wbreza/azure-sdk-for-go/sdk/data/azsearch"
)

func PromptAIServiceAccount(ctx context.Context, azdContext *ext.Context, azureContext *ext.AzureContext) (*armcognitiveservices.Account, error) {
	if azureContext == nil {
		azureContext = ext.NewEmptyAzureContext()
	}

	if err := azureContext.EnsureSubscription(ctx); err != nil {
		return nil, err
	}

	principal, err := azdContext.Principal(ctx)
	if err != nil {
		return nil, err
	}

	credential, err := azdContext.Credential()
	if err != nil {
		return nil, err
	}

	var armClientOptions *arm.ClientOptions

	azdContext.Invoke(func(clientOptions *arm.ClientOptions) error {
		armClientOptions = clientOptions
		return nil
	})

	accountsClient, err := armcognitiveservices.NewAccountsClient(azureContext.Scope.SubscriptionId, credential, armClientOptions)
	if err != nil {
		return nil, err
	}

	var aiService *armcognitiveservices.Account

	selectedResource, err := ext.PromptSubscriptionResource(ctx, azureContext, ext.ResourceOptions{
		ResourceType:            to.Ptr(azure.ResourceTypeCognitiveServiceAccount),
		Kinds:                   []string{"OpenAI", "AIServices", "CognitiveServices"},
		ResourceTypeDisplayName: "Azure AI service",
		CreateResource: func(ctx context.Context) (*azure.ResourceExtended, error) {
			if err := azureContext.EnsureResourceGroup(ctx); err != nil {
				return nil, err
			}

			namePrompt := ux.NewPrompt(&ux.PromptOptions{
				Message: "Enter the name for the Azure AI service account",
			})

			accountName, err := namePrompt.Ask()
			if err != nil {
				return nil, err
			}

			taskName := fmt.Sprintf("Creating Azure AI service account %s", color.CyanString(accountName))

			err = ux.NewTaskList(nil).
				AddTask(ux.TaskOptions{
					Title: taskName,
					Action: func(setProgress ux.SetProgressFunc) (ux.TaskState, error) {
						account := armcognitiveservices.Account{
							Name: &accountName,
							Identity: &armcognitiveservices.Identity{
								Type: to.Ptr(armcognitiveservices.ResourceIdentityTypeSystemAssigned),
							},
							Location: &azureContext.Scope.Location,
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

						poller, err := accountsClient.BeginCreate(ctx, azureContext.Scope.ResourceGroup, accountName, account, nil)
						if err != nil {
							return ux.Error, common.NewDetailedError("Failed to create Azure AI service account", err)
						}

						accountResponse, err := poller.PollUntilDone(ctx, nil)
						if err != nil {
							return ux.Error, common.NewDetailedError("Failed to create Azure AI service account", err)
						}

						aiService = &accountResponse.Account

						return ux.Success, nil
					},
				}).
				AddTask(ux.TaskOptions{
					Title: "Creating role assignments",
					Action: func(setProgress ux.SetProgressFunc) (ux.TaskState, error) {
						if aiService == nil {
							return ux.Skipped, errors.New("Azure AI service account creation failed")
						}

						err := azdContext.Invoke(func(rbacClient *azure.EntraIdService) error {
							err := rbacClient.EnsureRoleAssignment(
								ctx,
								azureContext.Scope.SubscriptionId,
								*aiService.ID,
								principal.Oid,
								azure.RoleCognitiveServicesOpenAIContributor,
							)
							if err != nil {
								return err
							}

							return nil
						})

						if err != nil {
							return ux.Error, err
						}

						return ux.Success, nil
					},
				}).
				Run()

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

type PromptModelDeploymentOptions struct {
	SelectorOptions *ext.SelectOptions
	Capabilities    []string
}

func PromptModelDeployment(ctx context.Context, azdContext *ext.Context, azureContext *ext.AzureContext, options *PromptModelDeploymentOptions) (*armcognitiveservices.Deployment, error) {
	if azureContext == nil {
		azureContext = ext.NewEmptyAzureContext()
	}

	var aiService *arm.ResourceID

	kinds := []string{"OpenAI", "AIServices", "CognitiveServices"}
	aiService, has := azureContext.Resources.FindByTypeAndKind(ctx, azure.ResourceTypeCognitiveServiceAccount, kinds)
	if !has {
		selectedAiService, err := PromptAIServiceAccount(ctx, azdContext, azureContext)
		if err != nil {
			return nil, err
		}

		if match, has := azureContext.Resources.FindById(*selectedAiService.ID); has {
			aiService = match
		}
	}

	if options == nil {
		options = &PromptModelDeploymentOptions{}
	}

	if options.SelectorOptions == nil {
		options.SelectorOptions = &ext.SelectOptions{}
	}

	mergedSelectorOptions := &ext.SelectOptions{}

	defaultSelectorOptions := &ext.SelectOptions{
		Message:            "Select an AI model deployment",
		LoadingMessage:     "Loading model deployments...",
		AllowNewResource:   to.Ptr(true),
		NewResourceMessage: "Deploy a new AI model",
		CreatingMessage:    "Deploying AI model...",
	}

	mergo.Merge(mergedSelectorOptions, options.SelectorOptions, mergo.WithoutDereference)
	mergo.Merge(mergedSelectorOptions, defaultSelectorOptions, mergo.WithoutDereference)

	credential, err := azdContext.Credential()
	if err != nil {
		return nil, err
	}

	var armClientOptions *arm.ClientOptions
	azdContext.Invoke(func(clientOptions *arm.ClientOptions) error {
		armClientOptions = clientOptions
		return nil
	})

	deploymentsClient, err := armcognitiveservices.NewDeploymentsClient(azureContext.Scope.SubscriptionId, credential, armClientOptions)
	if err != nil {
		return nil, err
	}

	existingResource, _ := azureContext.Resources.FindByType(azure.ResourceTypeCognitiveServiceAccountDeployment)

	return ext.PromptCustomResource(ctx, ext.CustomResourceOptions[armcognitiveservices.Deployment]{
		Selected: func(resource *armcognitiveservices.Deployment) bool {
			if existingResource == nil {
				return false
			}

			return strings.EqualFold(*resource.ID, existingResource.String())
		},
		SelectorOptions: mergedSelectorOptions,
		LoadData: func(ctx context.Context) ([]*armcognitiveservices.Deployment, error) {
			deploymentList := []*armcognitiveservices.Deployment{}
			modelPager := deploymentsClient.NewListPager(aiService.ResourceGroupName, aiService.Name, nil)
			for modelPager.More() {
				models, err := modelPager.NextPage(ctx)
				if err != nil {
					return nil, err
				}

				for _, model := range models.Value {
					if len(options.Capabilities) == 0 {
						deploymentList = append(deploymentList, model)
						continue
					}

					// Filter models by specified capabilities
					// Must match all capabilities
					for _, capability := range options.Capabilities {
						if _, has := model.Properties.Capabilities[capability]; has {
							deploymentList = append(deploymentList, model)
						}
					}
				}
			}

			return deploymentList, nil
		},
		DisplayResource: func(resource *armcognitiveservices.Deployment) (string, error) {
			return fmt.Sprintf("%s %s", *resource.Name, color.HiBlackString("(Model: %s, Version: %s)", *resource.Properties.Model.Name, *resource.Properties.Model.Version)), nil
		},
		CreateResource: func(ctx context.Context) (*armcognitiveservices.Deployment, error) {
			selectedModel, err := PromptModel(ctx, azdContext, azureContext, &PromptModelOptions{
				Capabilities: options.Capabilities,
			})
			if err != nil {
				return nil, err
			}

			selectedSku, err := PromptModelSku(ctx, azdContext, selectedModel)
			if err != nil {
				return nil, err
			}
			var deploymentName string

			namePrompt := ux.NewPrompt(&ux.PromptOptions{
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

			err = ux.NewTaskList(nil).
				AddTask(ux.TaskOptions{
					Title: taskName,
					Action: func(setProgress ux.SetProgressFunc) (ux.TaskState, error) {
						existingDeployment, err := deploymentsClient.Get(ctx, aiService.ResourceGroupName, aiService.Name, deploymentName, nil)
						if err == nil && *existingDeployment.Name == deploymentName {
							return ux.Error, errors.New("deployment with the same name already exists")
						}

						poller, err := deploymentsClient.BeginCreateOrUpdate(ctx, aiService.ResourceGroupName, aiService.Name, deploymentName, deployment, nil)
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

type PromptModelOptions struct {
	Capabilities []string
}

func PromptModel(ctx context.Context, azdContext *ext.Context, azureContext *ext.AzureContext, options *PromptModelOptions) (*armcognitiveservices.Model, error) {
	if azureContext == nil {
		azureContext = ext.NewEmptyAzureContext()
	}

	kinds := []string{"OpenAI", "AIServices", "CognitiveServices"}
	aiService, has := azureContext.Resources.FindByTypeAndKind(ctx, azure.ResourceTypeCognitiveServiceAccount, kinds)
	if !has {
		selectedAiAccount, err := PromptAIServiceAccount(ctx, azdContext, azureContext)
		if err != nil {
			return nil, err
		}

		if match, has := azureContext.Resources.FindById(*selectedAiAccount.ID); has {
			aiService = match
		}
	}

	if options == nil {
		options = &PromptModelOptions{
			Capabilities: []string{},
		}
	}

	var armClientOptions *arm.ClientOptions
	azdContext.Invoke(func(clientOptions *arm.ClientOptions) error {
		armClientOptions = clientOptions
		return nil
	})

	return ext.PromptCustomResource(ctx, ext.CustomResourceOptions[armcognitiveservices.Model]{
		SelectorOptions: &ext.SelectOptions{
			Message:          "Select a model",
			LoadingMessage:   "Loading models...",
			AllowNewResource: to.Ptr(false),
		},
		LoadData: func(ctx context.Context) ([]*armcognitiveservices.Model, error) {
			credential, err := azdContext.Credential()
			if err != nil {
				return nil, err
			}

			clientFactory, err := armcognitiveservices.NewClientFactory(aiService.SubscriptionID, credential, armClientOptions)
			if err != nil {
				return nil, err
			}

			accountsClient := clientFactory.NewAccountsClient()
			modelsClient := clientFactory.NewModelsClient()

			aiService, err := accountsClient.Get(ctx, aiService.ResourceGroupName, aiService.Name, nil)
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
					if *model.Kind != *aiService.Kind {
						continue
					}

					if len(options.Capabilities) == 0 {
						models = append(models, model)
						continue
					}

					// Filter models by specified capabilities
					// Must match all capabilities
					for _, capability := range options.Capabilities {
						if _, has := model.Model.Capabilities[capability]; has {
							models = append(models, model)
						}
					}
				}
			}

			return models, nil
		},
		DisplayResource: func(model *armcognitiveservices.Model) (string, error) {
			return fmt.Sprintf("%s %s", *model.Model.Name, color.HiBlackString("(Version: %s)", *model.Model.Version)), nil
		},
	})
}

func PromptModelSku(ctx context.Context, azdContext *ext.Context, model *armcognitiveservices.Model) (*armcognitiveservices.ModelSKU, error) {
	return ext.PromptCustomResource(ctx, ext.CustomResourceOptions[armcognitiveservices.ModelSKU]{
		SelectorOptions: &ext.SelectOptions{
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

func PromptStorageAccount(ctx context.Context, azdContext *ext.Context, azureContext *ext.AzureContext) (*armstorage.Account, error) {
	if azureContext == nil {
		azureContext = ext.NewEmptyAzureContext()
	}

	if err := azureContext.EnsureSubscription(ctx); err != nil {
		return nil, err
	}

	credential, err := azdContext.Credential()
	if err != nil {
		return nil, err
	}

	principal, err := azdContext.Principal(ctx)
	if err != nil {
		return nil, err
	}

	var armClientOptions *arm.ClientOptions

	err = azdContext.Invoke(func(clientOptions *arm.ClientOptions) error {
		armClientOptions = clientOptions
		return nil
	})

	if err != nil {
		return nil, err
	}

	accountsClient, err := armstorage.NewAccountsClient(azureContext.Scope.SubscriptionId, credential, armClientOptions)
	if err != nil {
		return nil, err
	}

	selectedResource, err := ext.PromptSubscriptionResource(ctx, azureContext, ext.ResourceOptions{
		ResourceType:            to.Ptr(azure.ResourceTypeStorageAccount),
		ResourceTypeDisplayName: "Azure Storage account",
		CreateResource: func(ctx context.Context) (*azure.ResourceExtended, error) {
			if err := azureContext.EnsureResourceGroup(ctx); err != nil {
				return nil, err
			}

			namePrompt := ux.NewPrompt(&ux.PromptOptions{
				Message: "Enter the name for the storage account",
			})

			accountName, err := namePrompt.Ask()
			if err != nil {
				return nil, err
			}

			taskName := fmt.Sprintf("Creating storage account %s", color.CyanString(accountName))

			var storageAccount *armstorage.Account

			err = ux.NewTaskList(nil).
				AddTask(ux.TaskOptions{
					Title: taskName,
					Action: func(setProgress ux.SetProgressFunc) (ux.TaskState, error) {
						accountCreateParams := armstorage.AccountCreateParameters{
							Location: &azureContext.Scope.Location,
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

						poller, err := accountsClient.BeginCreate(ctx, azureContext.Scope.ResourceGroup, accountName, accountCreateParams, nil)
						if err != nil {
							return ux.Error, common.NewDetailedError("Failed to create storage account", err)
						}

						createResponse, err := poller.PollUntilDone(ctx, nil)
						if err != nil {
							return ux.Error, common.NewDetailedError("Failed to create storage account", err)
						}

						storageAccount = &createResponse.Account
						return ux.Success, nil
					},
				}).
				AddTask(ux.TaskOptions{
					Title: "Assigning Storage Blob Data Contributor role",
					Action: func(setProgress ux.SetProgressFunc) (ux.TaskState, error) {
						if storageAccount == nil {
							return ux.Skipped, errors.New("Storage account creation failed")
						}

						err := azdContext.Invoke(func(rbacClient *azure.EntraIdService) error {
							err := rbacClient.EnsureRoleAssignment(
								ctx,
								azureContext.Scope.SubscriptionId,
								*storageAccount.ID,
								principal.Oid,
								azure.RoleDefinitionStorageBlobDataContributor,
							)
							if err != nil {
								return err
							}

							return nil
						})

						if err != nil {
							return ux.Error, err
						}

						return ux.Success, nil
					},
				}).
				Run()

			if err != nil {
				return nil, err
			}

			return &azure.ResourceExtended{
				Resource: azure.Resource{
					Id:       *storageAccount.ID,
					Name:     *storageAccount.Name,
					Type:     *storageAccount.Type,
					Location: *storageAccount.Location,
				},
				Kind: string(*storageAccount.Kind),
			}, nil
		},
	})

	if err != nil {
		return nil, err
	}

	parsedResource, err := arm.ParseResourceID(selectedResource.Id)
	if err != nil {
		return nil, err
	}

	storageAccount, err := accountsClient.GetProperties(ctx, parsedResource.ResourceGroupName, parsedResource.Name, nil)
	if err != nil {
		return nil, err
	}

	return &storageAccount.Account, nil
}

func PromptStorageContainer(ctx context.Context, azdContext *ext.Context, azureContext *ext.AzureContext) (*armstorage.BlobContainer, error) {
	if azureContext == nil {
		azureContext = ext.NewEmptyAzureContext()
	}

	storageAccount, has := azureContext.Resources.FindByType(azure.ResourceTypeStorageAccount)
	if !has {
		selctedStorageAccount, err := PromptStorageAccount(ctx, azdContext, azureContext)
		if err != nil {
			return nil, err
		}

		if match, has := azureContext.Resources.FindById(*selctedStorageAccount.ID); has {
			storageAccount = match
		}
	}

	credential, err := azdContext.Credential()
	if err != nil {
		return nil, err
	}

	var armClientOptions *arm.ClientOptions
	err = azdContext.Invoke(func(clientOptions *arm.ClientOptions) error {
		armClientOptions = clientOptions
		return nil
	})

	if err != nil {
		return nil, err
	}

	containersClient, err := armstorage.NewBlobContainersClient(storageAccount.SubscriptionID, credential, armClientOptions)
	if err != nil {
		return nil, err
	}

	return ext.PromptCustomResource(ctx, ext.CustomResourceOptions[armstorage.BlobContainer]{
		Selected: func(resource *armstorage.BlobContainer) bool {
			// TODO: preselect container?
			return false
		},
		SelectorOptions: &ext.SelectOptions{
			Message:            "Select a blob container",
			LoadingMessage:     "Loading blob containers...",
			AllowNewResource:   to.Ptr(true),
			NewResourceMessage: "Create a new blob container",
			CreatingMessage:    "Creating blob container...",
		},
		LoadData: func(ctx context.Context) ([]*armstorage.BlobContainer, error) {
			blobContainers := []*armstorage.BlobContainer{}

			pager := containersClient.NewListPager(storageAccount.ResourceGroupName, storageAccount.Name, nil)
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
			namePrompt := ux.NewPrompt(&ux.PromptOptions{
				Message: "Enter the name for the blob container",
			})

			containerName, err := namePrompt.Ask()
			if err != nil {
				return nil, err
			}

			taskName := fmt.Sprintf("Creating blob container %s", color.CyanString(containerName))

			var blobContainer *armstorage.BlobContainer

			err = ux.NewTaskList(nil).
				AddTask(ux.TaskOptions{
					Title: taskName,
					Action: func(setProgress ux.SetProgressFunc) (ux.TaskState, error) {
						newContainer := armstorage.BlobContainer{
							Name: &containerName,
							ContainerProperties: &armstorage.ContainerProperties{
								PublicAccess: to.Ptr(armstorage.PublicAccessNone),
							},
						}

						createResponse, err := containersClient.Create(ctx, storageAccount.ResourceGroupName, storageAccount.Name, containerName, newContainer, nil)
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

func PromptSearchService(ctx context.Context, azdContext *ext.Context, azureContext *ext.AzureContext) (*armsearch.Service, error) {
	if azureContext == nil {
		azureContext = ext.NewEmptyAzureContext()
	}

	if err := azureContext.EnsureSubscription(ctx); err != nil {
		return nil, err
	}

	kinds := []string{"OpenAI", "AIServices", "CognitiveServices"}
	aiService, has := azureContext.Resources.FindByTypeAndKind(ctx, azure.ResourceTypeCognitiveServiceAccount, kinds)
	if !has {
		selectedAiAccount, err := PromptAIServiceAccount(ctx, azdContext, azureContext)
		if err != nil {
			return nil, err
		}

		if match, has := azureContext.Resources.FindById(*selectedAiAccount.ID); has {
			aiService = match
		}
	}

	principal, err := azdContext.Principal(ctx)
	if err != nil {
		return nil, err
	}

	credential, err := azdContext.Credential()
	if err != nil {
		return nil, err
	}

	var armClientOptions *arm.ClientOptions

	azdContext.Invoke(func(clientOptions *arm.ClientOptions) error {
		armClientOptions = clientOptions
		return nil
	})

	searchClient, err := armsearch.NewServicesClient(azureContext.Scope.SubscriptionId, credential, armClientOptions)
	if err != nil {
		return nil, err
	}

	var aiSearchService *armsearch.Service

	selectedResource, err := ext.PromptSubscriptionResource(ctx, azureContext, ext.ResourceOptions{
		ResourceType:            to.Ptr(azure.ResourceTypeSearchService),
		ResourceTypeDisplayName: "Azure AI Search",
		CreateResource: func(ctx context.Context) (*azure.ResourceExtended, error) {
			if err := azureContext.EnsureResourceGroup(ctx); err != nil {
				return nil, err
			}

			namePrompt := ux.NewPrompt(&ux.PromptOptions{
				Message: "Enter the name for the Azure AI Search service",
			})

			searchName, err := namePrompt.Ask()
			if err != nil {
				return nil, err
			}

			taskName := fmt.Sprintf("Creating Azure AI Search service %s", color.CyanString(searchName))

			err = ux.NewTaskList(nil).
				AddTask(ux.TaskOptions{
					Title: taskName,
					Action: func(setProgress ux.SetProgressFunc) (ux.TaskState, error) {
						searchService := armsearch.Service{
							Name:     &searchName,
							Location: &azureContext.Scope.Location,
							Identity: &armsearch.Identity{
								Type: to.Ptr(armsearch.IdentityTypeSystemAssigned),
							},
							SKU: &armsearch.SKU{
								Name: to.Ptr(armsearch.SKUNameStandard),
							},
							Properties: &armsearch.ServiceProperties{
								PublicNetworkAccess: to.Ptr(armsearch.PublicNetworkAccessEnabled),
								HostingMode:         to.Ptr(armsearch.HostingModeDefault),
								ReplicaCount:        to.Ptr(int32(1)),
								PartitionCount:      to.Ptr(int32(1)),
								AuthOptions: &armsearch.DataPlaneAuthOptions{
									AADOrAPIKey: &armsearch.DataPlaneAADOrAPIKeyAuthOption{
										AADAuthFailureMode: to.Ptr(armsearch.AADAuthFailureModeHttp403),
									},
								},
							},
						}

						poller, err := searchClient.BeginCreateOrUpdate(ctx, azureContext.Scope.ResourceGroup, searchName, searchService, nil, nil)
						if err != nil {
							return ux.Error, common.NewDetailedError("Failed to create Azure AI Search service", err)
						}

						accountResponse, err := poller.PollUntilDone(ctx, nil)
						if err != nil {
							return ux.Error, common.NewDetailedError("Failed to create Azure AI Search service", err)
						}

						aiSearchService = &accountResponse.Service

						return ux.Success, nil
					},
				}).
				AddTask(ux.TaskOptions{
					Title: "Creating role assignments for current user",
					Action: func(setProgress ux.SetProgressFunc) (ux.TaskState, error) {
						if aiSearchService == nil {
							return ux.Skipped, errors.New("Azure AI Service service creation failed")
						}

						// Assign roles to current logged in user
						err := azdContext.Invoke(func(rbacClient *azure.EntraIdService) error {
							err := rbacClient.EnsureRoleAssignment(
								ctx,
								aiService.SubscriptionID,
								*aiSearchService.ID,
								principal.Oid,
								azure.RoleSearchIndexDataContributor,
								azure.RoleSearchServiceContributor,
							)
							if err != nil {
								return err
							}

							return nil
						})

						if err != nil {
							return ux.Error, err
						}

						return ux.Success, nil
					},
				}).
				AddTask(ux.TaskOptions{
					Title: "Creating role assignments for AI service account",
					Action: func(setProgress ux.SetProgressFunc) (ux.TaskState, error) {
						if aiSearchService == nil {
							return ux.Skipped, errors.New("Azure AI Service service creation failed")
						}

						accountsClient, err := armcognitiveservices.NewAccountsClient(aiService.SubscriptionID, credential, armClientOptions)
						if err != nil {
							return ux.Error, err
						}

						aiAccount, err := accountsClient.Get(ctx, aiService.ResourceGroupName, aiService.Name, nil)
						if err != nil {
							return ux.Error, err
						}

						aiSearchResource, err := arm.ParseResourceID(*aiSearchService.ID)
						if err != nil {
							return ux.Error, err
						}

						// Assign roles to AI service account
						err = azdContext.Invoke(func(rbacClient *azure.EntraIdService) error {
							err := rbacClient.EnsureRoleAssignment(
								ctx,
								aiSearchResource.SubscriptionID,
								*aiSearchService.ID,
								*aiAccount.Identity.PrincipalID,
								azure.RoleSearchIndexDataReader,
								azure.RoleSearchServiceContributor,
							)
							if err != nil {
								return err
							}

							return nil
						})

						if err != nil {
							return ux.Error, err
						}

						return ux.Success, nil
					},
				}).
				Run()

			if err != nil {
				return nil, err
			}

			return &azure.ResourceExtended{
				Resource: azure.Resource{
					Id:       *aiSearchService.ID,
					Name:     *aiSearchService.Name,
					Type:     *aiSearchService.Type,
					Location: *aiSearchService.Location,
				},
			}, nil
		},
	})
	if err != nil {
		return nil, err
	}

	if aiSearchService == nil {
		parsedResource, err := arm.ParseResourceID(selectedResource.Id)
		if err != nil {
			return nil, err
		}

		existingAccount, err := searchClient.Get(ctx, parsedResource.ResourceGroupName, parsedResource.Name, nil, nil)
		if err != nil {
			return nil, err
		}

		aiSearchService = &existingAccount.Service
	}

	return aiSearchService, nil
}

func PromptSearchIndex(ctx context.Context, azdContext *ext.Context, azureContext *ext.AzureContext) (*azsearch.Index, error) {
	if azureContext == nil {
		azureContext = ext.NewEmptyAzureContext()
	}

	searchService, has := azureContext.Resources.FindByType(azure.ResourceTypeSearchService)
	if !has {
		selectedSearchService, err := PromptSearchService(ctx, azdContext, azureContext)
		if err != nil {
			return nil, err
		}

		if match, has := azureContext.Resources.FindById(*selectedSearchService.ID); has {
			searchService = match
		}
	}

	credential, err := azdContext.Credential()
	if err != nil {
		return nil, err
	}

	var azClientOptions *azcore.ClientOptions

	azdContext.Invoke(func(clientOptions *azcore.ClientOptions) error {
		azClientOptions = clientOptions
		return nil
	})

	endpoint := fmt.Sprintf("https://%s.%s", searchService.Name, "search.windows.net")
	indexesClient, err := azsearch.NewIndexesClient(endpoint, credential, azClientOptions)
	if err != nil {
		return nil, err
	}

	return ext.PromptCustomResource(ctx, ext.CustomResourceOptions[azsearch.Index]{
		Selected: func(resource *azsearch.Index) bool {
			// TODO: preselect index?
			return false
		},
		SelectorOptions: &ext.SelectOptions{
			Message:            "Select a search index",
			LoadingMessage:     "Loading search indexes...",
			AllowNewResource:   to.Ptr(true),
			NewResourceMessage: "Create a new search index",
			CreatingMessage:    "Creating search index...",
		},
		LoadData: func(ctx context.Context) ([]*azsearch.Index, error) {
			indexPager := indexesClient.NewListPager(nil, nil)

			indexList := []*azsearch.Index{}
			for indexPager.More() {
				page, err := indexPager.NextPage(ctx)
				if err != nil {
					return nil, err
				}

				indexList = append(indexList, page.Indexes...)
			}

			return indexList, nil
		},
		DisplayResource: func(index *azsearch.Index) (string, error) {
			return *index.Name, nil
		},
		CreateResource: func(ctx context.Context) (*azsearch.Index, error) {
			namePrompt := ux.NewPrompt(&ux.PromptOptions{
				Message: "Enter the name for the Azure Search index",
			})

			indexName, err := namePrompt.Ask()
			if err != nil {
				return nil, err
			}

			taskName := fmt.Sprintf("Creating Azure AI Search index %s", color.CyanString(indexName))

			indexSpec := defaultSearchIndex(indexName)
			var newIndex *azsearch.Index

			err = ux.NewTaskList(nil).
				AddTask(ux.TaskOptions{
					Title: taskName,
					Action: func(setProgress ux.SetProgressFunc) (ux.TaskState, error) {
						createResponse, err := indexesClient.CreateOrUpdate(ctx, indexName, azsearch.Enum0ReturnRepresentation, *indexSpec, nil, nil)
						if err != nil {
							return ux.Error, common.NewDetailedError("Failed to create Azure Search index", err)
						}

						newIndex = &createResponse.Index

						return ux.Success, nil
					},
				}).
				Run()

			if err != nil {
				return nil, err
			}

			return newIndex, nil
		},
	})
}

func defaultSearchIndex(indexName string) *azsearch.Index {
	return &azsearch.Index{
		Name: &indexName,
		Fields: []*azsearch.Field{
			{
				Name:        to.Ptr("id"),
				Type:        to.Ptr(azsearch.SearchFieldDataTypeString),
				Key:         to.Ptr(true),
				Analyzer:    to.Ptr(azsearch.LexicalAnalyzerNameKeyword),
				Retrievable: to.Ptr(true),
				Filterable:  to.Ptr(false),
				Sortable:    to.Ptr(true),
				Facetable:   to.Ptr(false),
				Searchable:  to.Ptr(true),
			},
			{
				Name:        to.Ptr("parentId"),
				Type:        to.Ptr(azsearch.SearchFieldDataTypeString),
				Analyzer:    nil,
				Retrievable: to.Ptr(true),
				Filterable:  to.Ptr(true),
				Sortable:    to.Ptr(false),
				Facetable:   to.Ptr(false),
				Searchable:  to.Ptr(false),
			},
			{
				Name:        to.Ptr("summary"),
				Type:        to.Ptr(azsearch.SearchFieldDataTypeString),
				Retrievable: to.Ptr(true),
				Filterable:  to.Ptr(false),
				Sortable:    to.Ptr(false),
				Facetable:   to.Ptr(false),
				Searchable:  to.Ptr(true),
			},
			{
				Name:        to.Ptr("content"),
				Type:        to.Ptr(azsearch.SearchFieldDataTypeString),
				Retrievable: to.Ptr(true),
				Filterable:  to.Ptr(false),
				Sortable:    to.Ptr(false),
				Facetable:   to.Ptr(false),
				Searchable:  to.Ptr(true),
			},
			{
				Name:        to.Ptr("path"),
				Type:        to.Ptr(azsearch.SearchFieldDataTypeString),
				Retrievable: to.Ptr(true),
				Filterable:  to.Ptr(false),
				Sortable:    to.Ptr(false),
				Facetable:   to.Ptr(false),
				Searchable:  to.Ptr(true),
			},
			{
				Name:                    to.Ptr("vector"),
				Type:                    to.Ptr(azsearch.SearchFieldDataType("Collection(Edm.Single)")),
				VectorSearchDimensions:  to.Ptr(int32(1536)),
				VectorSearchProfileName: to.Ptr("default"),
				Retrievable:             to.Ptr(true),
				Filterable:              to.Ptr(false),
				Sortable:                to.Ptr(false),
				Facetable:               to.Ptr(false),
				Searchable:              to.Ptr(true),
			},
		},
		VectorSearch: &azsearch.VectorSearch{
			Profiles: []*azsearch.VectorSearchProfile{
				{
					Name:                       to.Ptr("default"),
					AlgorithmConfigurationName: to.Ptr("hnsw"),
				},
			},
			Algorithms: []azsearch.VectorSearchAlgorithmConfigurationClassification{
				&azsearch.HnswAlgorithmConfiguration{
					Name: to.Ptr("hnsw"),
					Kind: to.Ptr(azsearch.VectorSearchAlgorithmKindHnsw),
					Parameters: &azsearch.HnswParameters{
						Metric:         to.Ptr(azsearch.VectorSearchAlgorithmMetricCosine),
						M:              to.Ptr(int32(4)),
						EfConstruction: to.Ptr(int32(400)),
						EfSearch:       to.Ptr(int32(500)),
					},
				},
			},
		},
	}
}

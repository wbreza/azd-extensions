package cmd

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/wbreza/azd-extensions/extensions/ai/internal"
	"github.com/wbreza/azd-extensions/sdk/common"
	"github.com/wbreza/azd-extensions/sdk/common/permissions"
	"github.com/wbreza/azd-extensions/sdk/ext"
	"github.com/wbreza/azd-extensions/sdk/ext/output"
	"github.com/wbreza/azd-extensions/sdk/ux"
)

// Flag structs for the azd ai document commands
type SetupFlags struct {
}

// Command to initialize `azd ai document` command group
func newSetupCommand() *cobra.Command {
	// Main `document` command
	setupCmd := &cobra.Command{
		Use:   "setup",
		Short: "Configure your AI project and preps documents.",
		RunE: func(cmd *cobra.Command, args []string) error {
			header := output.CommandHeader{
				Title:       "Initializes AI project (azd ai setup)",
				Description: "Configure your AI project and preps documents.",
			}
			header.Print()

			ctx := cmd.Context()

			azdContext, err := ext.CurrentContext(ctx)
			if err != nil {
				return err
			}

			azureContext, err := azdContext.AzureContext(ctx)
			if err != nil {
				return err
			}

			fmt.Println("Let's get started by setting up your AI project")
			fmt.Println()

			extensionConfig, err := internal.LoadExtensionConfig(ctx, azdContext)
			if err != nil {
				aiAccount, err := internal.PromptAIServiceAccount(ctx, azdContext, azureContext)
				if err != nil {
					return err
				}

				extensionConfig = &internal.ExtensionConfig{
					Ai: internal.AiConfig{
						Service:  *aiAccount.Name,
						Endpoint: *aiAccount.Properties.Endpoint,
					},
				}
			}

			if extensionConfig.Ai.Models.ChatCompletion == "" {
				chatConfirm := ux.NewConfirm(&ux.ConfirmConfig{
					Message:      "Would you like to enable chat for your AI project?",
					DefaultValue: to.Ptr(true),
				})

				fmt.Println()
				color.Yellow("A chat completion model was not found. Lets get that setup for you.")

				userChatConfirmed, err := chatConfirm.Ask()
				if err != nil {
					return err
				}

				if *userChatConfirmed {
					chatModelDeployment, err := internal.PromptModelDeployment(ctx, azdContext, azureContext, &internal.PromptModelDeploymentOptions{
						Capabilities: []string{"chatCompletion"},
					})
					if err != nil {
						return err
					}

					extensionConfig.Ai.Models.ChatCompletion = *chatModelDeployment.Name
				}
			}

			customDataConfirm := ux.NewConfirm(&ux.ConfirmConfig{
				Message:      "Would you like to load custom data for your AI project?",
				DefaultValue: to.Ptr(true),
			})

			userCustomDataConfirmed, err := customDataConfirm.Ask()
			if err != nil {
				return err
			}

			if *userCustomDataConfirmed {
				if extensionConfig.Storage.Account == "" || extensionConfig.Storage.Container == "" {
					fmt.Println()
					color.Yellow("A storage account was not found. Lets get that setup for you.")

					storageAccount, err := internal.PromptStorageAccount(ctx, azdContext, azureContext)
					if err != nil {
						return err
					}

					extensionConfig.Storage.Account = *storageAccount.Name
					extensionConfig.Storage.Endpoint = *storageAccount.Properties.PrimaryEndpoints.Blob

					storageContainer, err := internal.PromptStorageContainer(ctx, azdContext, azureContext)
					if err != nil {
						return err
					}

					extensionConfig.Storage.Container = *storageContainer.Name
				}

				if extensionConfig.Search.Service == "" || extensionConfig.Search.Index == "" {
					fmt.Println()
					color.Yellow("An AI Search service was not found. Lets get that setup for you.")

					searchService, err := internal.PromptSearchService(ctx, azdContext, azureContext)
					if err != nil {
						return err
					}

					extensionConfig.Search.Service = *searchService.Name
					extensionConfig.Search.Endpoint = fmt.Sprintf("https://%s.search.windows.net", extensionConfig.Search.Service)

					searchIndex, err := internal.PromptSearchIndex(ctx, azdContext, azureContext)
					if err != nil {
						return err
					}

					extensionConfig.Search.Index = *searchIndex.Name
				}

				prepDataConfirm := ux.NewConfirm(&ux.ConfirmConfig{
					Message:      "Would you like to prep documents for your AI project?",
					DefaultValue: to.Ptr(true),
				})

				userPrepDataConfirmed, err := prepDataConfirm.Ask()
				if err != nil {
					return err
				}

				if *userPrepDataConfirmed {
					sourcePrompt := ux.NewPrompt(&ux.PromptConfig{
						Message:      "Enter the path to the source data",
						DefaultValue: "./data",
						Required:     true,
					})

					userSourcePath, err := sourcePrompt.Ask()
					if err != nil {
						return err
					}

					embeddingsOutputPrompt := ux.NewPrompt(&ux.PromptConfig{
						Message:      "Enter the path for the embeddings output",
						DefaultValue: "./embeddings",
						Required:     true,
					})

					userOutputPath, err := embeddingsOutputPrompt.Ask()
					if err != nil {
						return err
					}

					cwd, err := os.Getwd()
					if err != nil {
						log.Fatalf("Error getting current working directory: %v", err)
					}

					// Combine the current working directory with the relative path
					absSourcePath := filepath.Join(cwd, userSourcePath)
					matchingFiles, err := getMatchingFiles(absSourcePath, "*", true)
					if err != nil {
						return err
					}

					if len(matchingFiles) == 0 {
						return fmt.Errorf("no files found at source location")
					}

					absOutputPath := filepath.Join(cwd, userOutputPath)
					if err := os.MkdirAll(absOutputPath, permissions.PermissionDirectory); err != nil {
						return err
					}

					if extensionConfig.Ai.Models.Embeddings == "" {
						fmt.Println()
						color.Yellow("A text embedding model was not found. Lets get that setup for you.")

						embeddingModelDeployment, err := internal.PromptModelDeployment(ctx, azdContext, azureContext, &internal.PromptModelDeploymentOptions{
							Capabilities: []string{"embeddings"},
						})
						if err != nil {
							return err
						}

						extensionConfig.Ai.Models.Embeddings = *embeddingModelDeployment.Name
					}

					fmt.Println()
					fmt.Printf("AI Service: %s\n", color.CyanString(extensionConfig.Ai.Service))
					fmt.Printf("Chat Completion Model: %s\n", color.CyanString(extensionConfig.Ai.Models.ChatCompletion))
					fmt.Println()
					fmt.Printf("Storage Account: %s\n", color.CyanString(extensionConfig.Storage.Account))
					fmt.Printf("Storage Container: %s\n", color.CyanString(extensionConfig.Storage.Container))
					fmt.Println()
					fmt.Printf("Search Service: %s\n", color.CyanString(extensionConfig.Search.Service))
					fmt.Printf("Search Index: %s\n", color.CyanString(extensionConfig.Search.Index))
					fmt.Printf("Embeddings Model: %s\n", color.CyanString(extensionConfig.Ai.Models.Embeddings))
					fmt.Println()
					fmt.Printf("Source Data: %s\n", color.CyanString(absSourcePath))
					fmt.Printf("Embeddings Output: %s\n", color.CyanString(absOutputPath))
					fmt.Println()

					readyConfirm := ux.NewConfirm(&ux.ConfirmConfig{
						Message:      "Are you ready to proceed?",
						DefaultValue: to.Ptr(true),
					})

					if err := internal.SaveExtensionConfig(ctx, azdContext, extensionConfig); err != nil {
						return err
					}

					userReadyConfirmed, err := readyConfirm.Ask()
					if err != nil {
						return err
					}

					if userReadyConfirmed == nil || !*userReadyConfirmed {
						return ux.ErrCancelled
					}

					docPrepService, err := internal.NewDocumentPrepService(ctx, azdContext, extensionConfig)
					if err != nil {
						return err
					}

					err = ux.NewTaskList(nil).
						AddTask(ux.TaskOptions{
							Title: "Uploading documents",
							Action: func(setProgress ux.SetProgressFunc) (ux.TaskState, error) {
								setProgress(fmt.Sprintf("%d/%d", 0, len(matchingFiles)))

								for index, file := range matchingFiles {
									relativePath, err := filepath.Rel(cwd, file)
									if err != nil {
										return ux.Error, err
									}

									if docPrepService.Upload(ctx, file, relativePath); err != nil {
										return ux.Error, common.NewDetailedError("Failed to upload document", err)
									}

									setProgress(fmt.Sprintf("%d/%d", index+1, len(matchingFiles)))
								}

								return ux.Success, nil
							},
						}).
						AddTask(ux.TaskOptions{
							Title: "Generating text embeddings",
							Action: func(setProgress ux.SetProgressFunc) (ux.TaskState, error) {
								setProgress(fmt.Sprintf("%d/%d", 0, len(matchingFiles)))

								for index, file := range matchingFiles {
									if _, err := docPrepService.GenerateEmbedding(ctx, file, absOutputPath); err != nil {
										return ux.Error, common.NewDetailedError("Failed generating embedding", err)
									}

									setProgress(fmt.Sprintf("%d/%d", index+1, len(matchingFiles)))
								}

								return ux.Success, nil
							},
						}).
						AddTask(ux.TaskOptions{
							Title: "Populating search index",
							Action: func(setProgress ux.SetProgressFunc) (ux.TaskState, error) {
								embeddingDocuments, err := getMatchingFiles(absOutputPath, "*.json", true)
								if err != nil {
									return ux.Error, common.NewDetailedError("Failed fetching embedding documents", err)
								}

								setProgress(fmt.Sprintf("%d/%d", 0, len(embeddingDocuments)))

								for index, file := range embeddingDocuments {
									if docPrepService.IngestEmbedding(ctx, file); err != nil {
										return ux.Error, common.NewDetailedError("Failed ingesting embedding", err)
									}

									setProgress(fmt.Sprintf("%d/%d", index+1, len(embeddingDocuments)))
								}

								return ux.Success, nil
							},
						}).
						Run()

					if err != nil {
						return err
					}
				}
			}

			color.Green("SUCCESS: AI project setup completed successfully")
			fmt.Printf("Run %s to start chatting with your AI model\n", color.CyanString("azd ai chat"))

			return nil
		},
	}

	return setupCmd
}

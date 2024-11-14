package cmd

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/wbreza/azd-extensions/extensions/ai/internal"
	"github.com/wbreza/azd-extensions/sdk/common"
	"github.com/wbreza/azd-extensions/sdk/common/permissions"
	"github.com/wbreza/azd-extensions/sdk/ext"
	"github.com/wbreza/azd-extensions/sdk/ext/output"
	"github.com/wbreza/azd-extensions/sdk/ux"
)

// Flag structs for the azd ai embedding commands
type GenerateFlags struct {
	Source              string
	ServiceName         string
	ChatCompletionModel string
	EmbeddingModel      string
	Output              string
	Pattern             string
	Force               bool
}

type IngestFlags struct {
	ServiceName string
	IndexName   string
	Source      string
	Pattern     string
	Force       bool
}

// Command to initialize `azd ai embedding` command group
func newEmbeddingCommand() *cobra.Command {
	// Main `embedding` command
	embeddingCmd := &cobra.Command{
		Use:   "embedding",
		Short: "Manage embeddings, including generation, listing, and ingestion",
	}

	// Add subcommands to the `embedding` command
	embeddingCmd.AddCommand(newGenerateCommand())
	embeddingCmd.AddCommand(newIngestCommand())

	return embeddingCmd
}

// Subcommand `generate` for generating embeddings
func newGenerateCommand() *cobra.Command {
	flags := &GenerateFlags{}
	generateCmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate embeddings from documents in Azure",
		RunE: func(cmd *cobra.Command, args []string) error {
			header := output.CommandHeader{
				Title:       "Generate text embeddings for documents (azd ai embedding generate)",
				Description: "Generate text embeddings for documents using text embedding AI model",
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

			if flags.ChatCompletionModel != "" {
				extensionConfig.Ai.Models.Embeddings = flags.ChatCompletionModel
			}

			if flags.Output == "" {
				flags.Output = "embeddings"
			}

			if flags.Pattern == "" {
				flags.Pattern = "*"
			}

			if flags.ServiceName != "" {
				extensionConfig.Ai.Service = flags.ServiceName
			}

			if flags.ChatCompletionModel != "" {
				extensionConfig.Ai.Models.ChatCompletion = flags.ChatCompletionModel
			}

			if flags.EmbeddingModel != "" {
				extensionConfig.Ai.Models.Embeddings = flags.EmbeddingModel
			}

			if extensionConfig.Ai.Models.ChatCompletion == "" {
				color.Yellow("No chat completion model was found. Please select or create a chat completion model.")

				selectedModelDeployment, err := internal.PromptModelDeployment(ctx, azdContext, azureContext, &internal.PromptModelDeploymentOptions{
					Capabilities: []string{
						"chatCompletion",
					},
				})
				if err != nil {
					return err
				}

				extensionConfig.Ai.Models.ChatCompletion = *selectedModelDeployment.Name
			}

			if extensionConfig.Ai.Models.Embeddings == "" {
				color.Yellow("No text embedding model was found. Please select or create a text embedding model.")

				selectedModelDeployment, err := internal.PromptModelDeployment(ctx, azdContext, azureContext, &internal.PromptModelDeploymentOptions{
					Capabilities: []string{
						"embeddings",
					},
				})
				if err != nil {
					return err
				}

				extensionConfig.Ai.Models.Embeddings = *selectedModelDeployment.Name
			}

			if err := internal.SaveExtensionConfig(ctx, azdContext, extensionConfig); err != nil {
				return err
			}

			cwd, err := os.Getwd()
			if err != nil {
				log.Fatalf("Error getting current working directory: %v", err)
			}

			// Combine the current working directory with the relative path
			absSourcePath := filepath.Join(cwd, flags.Source)
			matchingFiles, err := getMatchingFiles(absSourcePath, flags.Pattern, true)
			if err != nil {
				return err
			}

			absOutputPath := filepath.Join(cwd, flags.Output)

			fmt.Printf("Source Data: %s\n", color.CyanString(absSourcePath))
			fmt.Printf("Output Path: %s\n", color.CyanString(absOutputPath))

			if !flags.Force {
				fmt.Println()
				fmt.Printf("Found %s matching files with pattern %s.\n", color.CyanString(fmt.Sprint(len(matchingFiles))), color.CyanString(flags.Pattern))

				continueConfirm := ux.NewConfirm(&ux.ConfirmConfig{
					DefaultValue: ux.Ptr(true),
					Message:      "Continue generating embeddings?",
				})

				userConfirmed, err := continueConfirm.Ask()
				if err != nil {
					return err
				}

				if !*userConfirmed {
					return ux.ErrCancelled
				}
			}

			if err := os.MkdirAll(absOutputPath, permissions.PermissionDirectory); err != nil {
				return err
			}

			docPrepService, err := internal.NewDocumentPrepService(ctx, azdContext, extensionConfig)
			if err != nil {
				return err
			}

			taskList := ux.NewTaskList(nil)

			for _, sourceDocumentPath := range matchingFiles {
				relativePath, err := filepath.Rel(cwd, sourceDocumentPath)
				if err != nil {
					return err
				}

				relativePath = strings.ReplaceAll(relativePath, "\\", "/")

				taskList.AddTask(ux.TaskOptions{
					Title: fmt.Sprintf("Generating embeddings for document %s", relativePath),
					Async: true,
					Action: func(setProgress ux.SetProgressFunc) (ux.TaskState, error) {
						if _, err := docPrepService.GenerateEmbedding(ctx, sourceDocumentPath, absOutputPath); err != nil {
							return ux.Error, common.NewDetailedError("Failed to generate embeddings", err)
						}

						return ux.Success, nil
					},
				})
			}

			if err := taskList.Run(); err != nil {
				return err
			}

			if err != nil {
				return err
			}

			return nil
		},
	}

	// Define flags for `generate` command
	generateCmd.Flags().StringVar(&flags.Source, "source", "", "Path to the local file or directory to upload (required)")
	generateCmd.Flags().StringVar(&flags.ServiceName, "service", "", "Azure AI service name")
	generateCmd.Flags().StringVar(&flags.EmbeddingModel, "embedding-model", "", "Model name to use for embedding (e.g., 'text-embedding-ada-002')")
	generateCmd.Flags().StringVar(&flags.ChatCompletionModel, "chat-completion-model", "", "Model name to use for summary generation (e.g., 'gpt-4')")
	generateCmd.Flags().StringVar(&flags.Output, "output", "", "Path or container to save generated embeddings")
	generateCmd.Flags().StringVarP(&flags.Pattern, "pattern", "p", "", "Specify file types to process (e.g., '.pdf', '.txt')")
	generateCmd.Flags().BoolVarP(&flags.Force, "force", "f", false, "Generate embeddings without confirmation")

	_ = generateCmd.MarkFlagRequired("source")

	return generateCmd
}

// Subcommand `ingest` for ingesting embeddings into a vector store
func newIngestCommand() *cobra.Command {
	flags := &IngestFlags{}
	ingestCmd := &cobra.Command{
		Use:   "ingest",
		Short: "Ingest embeddings into a vector store",
		RunE: func(cmd *cobra.Command, args []string) error {
			header := output.CommandHeader{
				Title:       "Ingest text embeddings into a vector store (azd ai embedding ingest)",
				Description: "Ingests text embeddings into the specified vector store such as Azure AI search index",
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

			if flags.Pattern == "" {
				flags.Pattern = "*"
			}

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

			if flags.ServiceName != "" {
				extensionConfig.Search.Service = flags.ServiceName
			}

			if flags.IndexName != "" {
				extensionConfig.Search.Index = flags.IndexName
			}

			if extensionConfig.Search.Service == "" {
				searchService, err := internal.PromptSearchService(ctx, azdContext, azureContext)
				if err != nil {
					return err
				}

				extensionConfig.Search.Service = *searchService.Name
				extensionConfig.Search.Endpoint = fmt.Sprintf("https://%s.search.windows.net", extensionConfig.Search.Service)
			}

			if flags.IndexName != "" {
				extensionConfig.Search.Index = flags.IndexName
			}

			if extensionConfig.Search.Index == "" {
				searchIndex, err := internal.PromptSearchIndex(ctx, azdContext, azureContext)
				if err != nil {
					return err
				}

				extensionConfig.Search.Index = *searchIndex.Name
			}

			if err := internal.SaveExtensionConfig(ctx, azdContext, extensionConfig); err != nil {
				return err
			}

			cwd, err := os.Getwd()
			if err != nil {
				log.Fatalf("Error getting current working directory: %v", err)
			}

			// Combine the current working directory with the relative path
			absSourcePath := filepath.Join(cwd, flags.Source)
			matchingFiles, err := getMatchingFiles(absSourcePath, flags.Pattern, true)
			if err != nil {
				return err
			}

			fmt.Printf("Source Data: %s\n", color.CyanString(absSourcePath))
			fmt.Printf("Search Service: %s\n", color.CyanString(extensionConfig.Search.Service))
			fmt.Printf("Search Index: %s\n", color.CyanString(extensionConfig.Search.Index))

			if !flags.Force {
				fmt.Println()
				fmt.Printf("Found %s matching files with pattern %s.\n", color.CyanString(fmt.Sprint(len(matchingFiles))), color.CyanString(flags.Pattern))

				continueConfirm := ux.NewConfirm(&ux.ConfirmConfig{
					DefaultValue: ux.Ptr(true),
					Message:      "Continue to ingesting embeddings?",
				})

				userConfirmed, err := continueConfirm.Ask()
				if err != nil {
					return err
				}

				if !*userConfirmed {
					return ux.ErrCancelled
				}
			}

			docPrepService, err := internal.NewDocumentPrepService(ctx, azdContext, extensionConfig)
			if err != nil {
				return err
			}

			taskList := ux.NewTaskList(nil)

			for _, file := range matchingFiles {
				relativePath, err := filepath.Rel(cwd, file)
				if err != nil {
					return err
				}

				relativePath = strings.ReplaceAll(relativePath, "\\", "/")

				taskList.AddTask(ux.TaskOptions{
					Title: fmt.Sprintf("Ingesting embeddings for document %s", relativePath),
					Action: func(setProgress ux.SetProgressFunc) (ux.TaskState, error) {
						if err := docPrepService.IngestEmbedding(ctx, file); err != nil {
							return ux.Error, common.NewDetailedError("Failed to ingest embedding", err)
						}

						return ux.Success, nil
					},
				})
			}

			if err := taskList.Run(); err != nil {
				return err
			}

			if err != nil {
				return err
			}

			return nil
		},
	}

	// Define flags for `ingest` command
	ingestCmd.Flags().StringVar(&flags.Source, "source", "", "Source of the embeddings (e.g., local file system path)")
	ingestCmd.Flags().StringVarP(&flags.Pattern, "pattern", "p", "", "Specify file types to process (e.g., '.pdf', '.txt')")
	ingestCmd.Flags().StringVar(&flags.ServiceName, "service", "", "Azure Cognitive Search service name")
	ingestCmd.Flags().StringVar(&flags.IndexName, "index", "", "Azure AI Search index name")
	ingestCmd.Flags().BoolVarP(&flags.Force, "force", "f", false, "Ingest embeddings without confirmation")

	_ = ingestCmd.MarkFlagRequired("source")

	return ingestCmd
}

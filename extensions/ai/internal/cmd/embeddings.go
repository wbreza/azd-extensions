package cmd

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/ai/azopenai"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/cognitiveservices/armcognitiveservices"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/wbreza/azd-extensions/extensions/ai/internal"
	"github.com/wbreza/azd-extensions/sdk/common"
	"github.com/wbreza/azd-extensions/sdk/common/permissions"
	"github.com/wbreza/azd-extensions/sdk/ext"
	"github.com/wbreza/azd-extensions/sdk/ext/output"
	"github.com/wbreza/azd-extensions/sdk/ux"
	"github.com/wbreza/azure-sdk-for-go/sdk/data/azsearchindex"
)

// Flag structs for the azd ai embedding commands
type GenerateFlags struct {
	Container string
	Source    string
	Model     string
	Output    string
	BatchSize int
	Pattern   string
	Normalize bool
	Language  string
}

type IngestFlags struct {
	IndexName string
	Source    string
	Pattern   string
	BatchSize int
	Overwrite bool
	Transform string
}

type EmbeddingDocument struct {
	ChunkId    string    `json:"chunk_id"`
	ParentId   string    `json:"parent_id"`
	Chunk      string    `json:"chunk"`
	Title      string    `json:"title"`
	TextVector []float32 `json:"text_vector"`
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

			credential, err := azdContext.Credential()
			if err != nil {
				return err
			}

			aiConfig, err := internal.LoadOrPromptAiConfig(ctx, azdContext)
			if err != nil {
				return err
			}

			if flags.Model != "" {
				aiConfig.Models.Embeddings = flags.Model
			}

			if aiConfig.Models.Embeddings == "" {
				color.Yellow("No text embedding model was found. Please select or create a text embedding model.")

				selectedModelDeployment, err := internal.PromptModelDeployment(ctx, azdContext, aiConfig, &internal.PromptModelDeploymentOptions{
					Capabilities: []string{
						"embeddings",
					},
				})
				if err != nil {
					return err
				}

				aiConfig.Models.Embeddings = *selectedModelDeployment.Name

				if err := internal.SaveAiConfig(ctx, azdContext, aiConfig); err != nil {
					return err
				}
			}

			cwd, err := os.Getwd()
			if err != nil {
				log.Fatalf("Error getting current working directory: %v", err)
			}

			// Combine the current working directory with the relative path
			absolutePath := filepath.Join(cwd, flags.Source)

			matchingFiles, err := getMatchingFiles(absolutePath, flags.Pattern, true)
			if err != nil {
				return err
			}

			embeddingsOutputPath := filepath.Join(cwd, "embeddings")
			if err := os.MkdirAll(embeddingsOutputPath, permissions.PermissionDirectory); err != nil {
				return err
			}

			err = azdContext.Invoke(func(clientOptions *arm.ClientOptions) error {
				accountClient, err := armcognitiveservices.NewAccountsClient(aiConfig.Subscription, credential, clientOptions)
				if err != nil {
					return err
				}

				account, err := accountClient.Get(ctx, aiConfig.ResourceGroup, aiConfig.Service, nil)
				if err != nil {
					return err
				}

				openAiClient, err := azopenai.NewClient(*account.Properties.Endpoint, credential, nil)
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

					jsonBytes, err := os.ReadFile(file)
					if err != nil {
						return err
					}

					content := string(jsonBytes)

					taskList.AddTask(ux.TaskOptions{
						Title: fmt.Sprintf("Generating embeddings for document %s", relativePath),
						Async: true,
						Action: func() (ux.TaskState, error) {
							completionsResponse, err := openAiClient.GetChatCompletions(ctx, azopenai.ChatCompletionsOptions{
								Messages: []azopenai.ChatRequestMessageClassification{
									&azopenai.ChatRequestSystemMessage{
										Content: azopenai.NewChatRequestSystemMessageContent("You are helping generate summary embeddings for specified document. Please provide a summary of the document."),
									},
									&azopenai.ChatRequestUserMessage{
										Content: azopenai.NewChatRequestUserMessageContent(content),
									},
								},
								DeploymentName: &aiConfig.Models.ChatCompletion,
							}, nil)
							if err != nil {
								return ux.Error, common.NewDetailedError("Failed to generate embeddings", err)
							}

							embeddingText := *completionsResponse.ChatCompletions.Choices[0].Message.Content

							response, err := openAiClient.GetEmbeddings(ctx, azopenai.EmbeddingsOptions{
								Input: []string{
									embeddingText,
								},
								DeploymentName: &aiConfig.Models.Embeddings,
							}, nil)

							if err != nil {
								return ux.Error, common.NewDetailedError("Failed to generate embeddings", err)
							}

							contentHash := sha256.Sum256([]byte(relativePath))

							embeddingDoc := EmbeddingDocument{
								Title:      relativePath,
								ChunkId:    hex.EncodeToString(contentHash[:]),
								Chunk:      embeddingText,
								TextVector: response.Embeddings.Data[0].Embedding,
							}

							base := filepath.Base(file)
							outputFileNameBase := strings.TrimSuffix(base, filepath.Ext(base))
							outputFilePath := filepath.Join(embeddingsOutputPath, fmt.Sprintf("%s.json", outputFileNameBase))

							jsonData, err := json.MarshalIndent(embeddingDoc, "", "  ")
							if err != nil {
								return ux.Error, err
							}

							if err := os.WriteFile(outputFilePath, jsonData, permissions.PermissionFile); err != nil {
								return ux.Error, common.NewDetailedError("Failed to write embeddings to file", err)
							}

							return ux.Success, nil
						},
					})
				}

				return taskList.Run()
			})

			if err != nil {
				return err
			}

			return nil
		},
	}

	// Define flags for `generate` command
	generateCmd.Flags().StringVar(&flags.Source, "source", "", "Path to the local file or directory to upload (required)")
	generateCmd.Flags().StringVar(&flags.Container, "container", "", "Source container in Blob Storage with documents to embed (required)")
	generateCmd.Flags().StringVar(&flags.Model, "model", "", "Model name to use for embedding (e.g., 'embedding-ada-002') (required)")
	generateCmd.Flags().StringVar(&flags.Output, "output", "", "Path or container to save generated embeddings")
	generateCmd.Flags().IntVar(&flags.BatchSize, "batch-size", 0, "Number of documents to process in each batch")
	generateCmd.Flags().StringVarP(&flags.Pattern, "pattern", "p", "", "Specify file types to process (e.g., '.pdf', '.txt')")
	generateCmd.Flags().BoolVar(&flags.Normalize, "normalize", false, "Normalize text before embedding (e.g., lowercase, remove special characters)")
	generateCmd.Flags().StringVar(&flags.Language, "language", "", "Specify language if the model supports multilingual text")

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
				Title:       "Generate text embeddings for documents (azd ai embedding generate)",
				Description: "Generate text embeddings for documents using text embedding AI model",
			}
			header.Print()

			ctx := cmd.Context()
			azdContext, err := ext.CurrentContext(ctx)
			if err != nil {
				return err
			}

			credential, err := azdContext.Credential()
			if err != nil {
				return err
			}

			aiConfig, err := internal.LoadOrPromptAiConfig(ctx, azdContext)
			if err != nil {
				return err
			}

			if aiConfig.Search.Service == "" {
				searchService, err := internal.PromptSearchService(ctx, azdContext, aiConfig)
				if err != nil {
					return err
				}

				aiConfig.Search.Service = *searchService.Name
			}

			if aiConfig.Search.Index == "" {
				searchIndex, err := internal.PromptSearchIndex(ctx, azdContext, aiConfig)
				if err != nil {
					return err
				}

				aiConfig.Search.Index = *searchIndex.Name
			}

			if err := internal.SaveAiConfig(ctx, azdContext, aiConfig); err != nil {
				return err
			}

			cwd, err := os.Getwd()
			if err != nil {
				log.Fatalf("Error getting current working directory: %v", err)
			}

			// Combine the current working directory with the relative path
			absolutePath := filepath.Join(cwd, flags.Source)

			matchingFiles, err := getMatchingFiles(absolutePath, flags.Pattern, true)
			if err != nil {
				return err
			}

			err = azdContext.Invoke(func(clientOptions *azcore.ClientOptions) error {
				endpoint := fmt.Sprintf("https://%s.search.windows.net", aiConfig.Search.Service)
				documentsClient, err := azsearchindex.NewDocumentsClient(endpoint, aiConfig.Search.Index, credential, clientOptions)
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
						Action: func() (ux.TaskState, error) {
							jsonBytes, err := os.ReadFile(file)
							if err != nil {
								return ux.Error, common.NewDetailedError("Failed to read embeddings from file", err)
							}

							embeddingDoc := map[string]any{}
							if err := json.Unmarshal(jsonBytes, &embeddingDoc); err != nil {
								return ux.Error, common.NewDetailedError("Failed to parse embeddings from file", err)
							}

							batch := azsearchindex.IndexBatch{
								Actions: []*azsearchindex.IndexAction{
									{
										ActionType:           to.Ptr(azsearchindex.IndexActionTypeMergeOrUpload),
										AdditionalProperties: embeddingDoc,
									},
								},
							}

							_, err = documentsClient.Index(ctx, batch, nil, nil)
							if err != nil {
								return ux.Error, common.NewDetailedError("Failed to ingest embeddings", err)
							}

							return ux.Success, nil
						},
					})
				}

				return taskList.Run()
			})

			if err != nil {
				return err
			}

			return nil
		},
	}

	// Define flags for `ingest` command
	ingestCmd.Flags().StringVar(&flags.IndexName, "index-name", "", "Target vector store or index name for ingestion (required)")
	ingestCmd.Flags().StringVar(&flags.Source, "source", "", "Source of the embeddings (e.g., directory or container) (required)")
	ingestCmd.Flags().StringVarP(&flags.Pattern, "pattern", "p", "", "Specify file types to process (e.g., '.pdf', '.txt')")
	ingestCmd.Flags().IntVar(&flags.BatchSize, "batch-size", 0, "Batch size for ingestion")
	ingestCmd.Flags().BoolVar(&flags.Overwrite, "overwrite", false, "Overwrite existing embeddings in the vector store")

	_ = ingestCmd.MarkFlagRequired("source")

	return ingestCmd
}

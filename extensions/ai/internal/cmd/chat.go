package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/ai/azopenai"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/cognitiveservices/armcognitiveservices"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/wbreza/azd-extensions/extensions/ai/internal"
	"github.com/wbreza/azd-extensions/sdk/ext"
	"github.com/wbreza/azd-extensions/sdk/ext/output"
	"github.com/wbreza/azd-extensions/sdk/ux"
	"github.com/wbreza/azure-sdk-for-go/sdk/data/azsearchindex"
)

type chatUsageFlags struct {
	message       string
	systemMessage string
	modelName     string
	temperature   float32
	maxTokens     int32
	useSearch     bool
}

var (
	defaultSystemMessage = "You are an AI assistant that helps people find information."
	defaultTemperature   = float32(0.7)
	defaultMaxTokens     = int32(800)
)

func newChatCommand() *cobra.Command {
	flags := &chatUsageFlags{}

	chatCmd := &cobra.Command{
		Use:   "chat",
		Short: "Commands for managing chat",
		RunE: func(cmd *cobra.Command, args []string) error {
			header := output.CommandHeader{
				Title:       "Chat with AI Model (azd ai chat)",
				Description: "Start a chat with an AI model from your Azure AI service model deployment.",
			}
			header.Print()

			ctx := cmd.Context()
			azdContext, err := ext.CurrentContext(ctx)
			if err != nil {
				return err
			}

			var armClientOptions *arm.ClientOptions
			var azClientOptions *azcore.ClientOptions

			azdContext.Invoke(func(options1 *arm.ClientOptions, options2 *azcore.ClientOptions) error {
				armClientOptions = options1
				azClientOptions = options2
				return nil
			})

			credential, err := azdContext.Credential()
			if err != nil {
				return err
			}

			aiConfig, err := internal.LoadOrPromptAiConfig(ctx, azdContext)
			if err != nil {
				return err
			}

			if flags.modelName != "" {
				aiConfig.Models.ChatCompletion = flags.modelName
			}

			if aiConfig.Models.ChatCompletion == "" {
				color.Yellow("No chat completion model was found. Please select or create a chat completion model.")

				chatDeployment, err := internal.PromptModelDeployment(ctx, azdContext, aiConfig, &internal.PromptModelDeploymentOptions{
					Capabilities: []string{
						"chatCompletion",
					},
				})
				if err != nil {
					if errors.Is(err, internal.ErrNoModelDeployments) {
						return &ext.ErrorWithSuggestion{
							Err:        err,
							Suggestion: fmt.Sprintf("Run %s to create a model deployment", color.CyanString("azd ai model deployment create")),
						}
					}
					return err
				}

				aiConfig.Models.ChatCompletion = *chatDeployment.Name
				fmt.Println()
			}

			if flags.useSearch {
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

				if aiConfig.Models.Embeddings == "" {
					embeddingDeployment, err := internal.PromptModelDeployment(ctx, azdContext, aiConfig, &internal.PromptModelDeploymentOptions{
						Capabilities: []string{
							"embeddings",
						},
					})
					if err != nil {
						return err
					}

					aiConfig.Models.Embeddings = *embeddingDeployment.Name
				}
			}

			if err := internal.SaveAiConfig(ctx, azdContext, aiConfig); err != nil {
				return err
			}

			loadingSpinner := ux.NewSpinner(&ux.SpinnerConfig{
				Text:        "Starting chat...",
				ClearOnStop: true,
			})

			loadingSpinner.Start(ctx)

			accountClient, err := armcognitiveservices.NewAccountsClient(aiConfig.Subscription, credential, armClientOptions)
			if err != nil {
				return err
			}

			account, err := accountClient.Get(ctx, aiConfig.ResourceGroup, aiConfig.Service, nil)
			if err != nil {
				return err
			}

			aiEndpoint := *account.Properties.Endpoint
			openAiClient, err := azopenai.NewClient(aiEndpoint, credential, &azopenai.ClientOptions{ClientOptions: *azClientOptions})
			if err != nil {
				return err
			}

			deploymentsClient, err := armcognitiveservices.NewDeploymentsClient(aiConfig.Subscription, credential, armClientOptions)
			if err != nil {
				return err
			}

			deployment, err := deploymentsClient.Get(ctx, aiConfig.ResourceGroup, aiConfig.Service, aiConfig.Models.ChatCompletion, nil)
			if err != nil {
				return err
			}

			hasVectorSearch := aiConfig.Search.Service != "" && aiConfig.Search.Index != "" && aiConfig.Models.Embeddings != ""

			loadingSpinner.Stop(ctx)

			fmt.Printf("AI Service: %s %s\n", color.CyanString(aiConfig.Service), color.HiBlackString("(%s)", aiConfig.ResourceGroup))
			fmt.Printf("Chat Model: %s %s\n", color.CyanString(aiConfig.Models.ChatCompletion), color.HiBlackString("(Model: %s, Version: %s)", *deployment.Properties.Model.Name, *deployment.Properties.Model.Version))
			fmt.Println()
			if hasVectorSearch {
				fmt.Printf("Search Service: %s %s\n", color.CyanString(aiConfig.Search.Service), color.HiBlackString("(%s)", aiConfig.Search.Index))
				fmt.Printf("Search Index: %s\n", color.CyanString(aiConfig.Search.Index))
				fmt.Printf("Embeddings Model: %s\n", color.CyanString(aiConfig.Models.Embeddings))
				fmt.Println()
			}
			fmt.Printf("System Message: %s\n", color.CyanString(flags.systemMessage))
			fmt.Printf("Temperature: %s %s\n", color.CyanString(fmt.Sprint(flags.temperature)), color.HiBlackString("(Controls randomness)"))
			fmt.Printf("Max Tokens: %s %s\n", color.CyanString(fmt.Sprint(flags.maxTokens)), color.HiBlackString("(Maximum number of tokens to generate)"))
			fmt.Println()

			messages := []azopenai.ChatRequestMessageClassification{}
			messages = append(messages, &azopenai.ChatRequestSystemMessage{
				Content: azopenai.NewChatRequestSystemMessageContent(flags.systemMessage),
			})

			thinkingSpinner := ux.NewSpinner(&ux.SpinnerConfig{
				Text: "Thinking...",
			})

			totalTokenCount := 0
			userMessage := flags.message

			for {
				var err error

				if userMessage == "" {
					chatPrompt := ux.NewPrompt(&ux.PromptConfig{
						Message:           "User",
						PlaceHolder:       "Press `Ctrl+X` to cancel",
						Required:          true,
						RequiredMessage:   "Please enter a message",
						ClearOnCompletion: true,
						IgnoreHintKeys:    true,
					})

					userMessage, err = chatPrompt.Ask()
					if err != nil {
						if errors.Is(err, ux.ErrCancelled) {
							break
						}

						return err
					}
				}

				fmt.Printf("%s: %s\n", color.GreenString("User"), userMessage)
				fmt.Println()

				if hasVectorSearch {
					embeddingsResponse, err := openAiClient.GetEmbeddings(ctx, azopenai.EmbeddingsOptions{
						Input:          []string{userMessage},
						DeploymentName: &aiConfig.Models.Embeddings,
					}, nil)
					if err != nil {
						return err
					}

					searchEndpoint := fmt.Sprintf("https://%s.search.windows.net", aiConfig.Search.Service)
					searchClient, err := azsearchindex.NewDocumentsClient(searchEndpoint, aiConfig.Search.Index, credential, azClientOptions)
					if err != nil {
						return err
					}

					searchResponse, err := searchClient.SearchPost(ctx, azsearchindex.SearchRequest{
						Select:    to.Ptr("chunk_id, chunk, title"),
						QueryType: to.Ptr(azsearchindex.QueryTypeSimple),
						VectorQueries: []azsearchindex.VectorQueryClassification{
							&azsearchindex.VectorizedQuery{
								Kind:       to.Ptr(azsearchindex.VectorQueryKindVector),
								Fields:     to.Ptr("text_vector"),
								Exhaustive: to.Ptr(true),
								K:          to.Ptr(int32(3)),
								Vector:     convertToFloatPtrSlice(embeddingsResponse.Data[0].Embedding),
							},
						},
					}, nil, nil)
					if err != nil {
						return nil
					}

					if len(searchResponse.Results) > 0 {
						contextResults := make([]string, len(searchResponse.Results))

						for i, result := range searchResponse.Results {
							contextResults[i] = fmt.Sprintf("- [%d] %s", i+1, fmt.Sprint(result.AdditionalProperties["chunk"]))
							log.Printf("Title: %s, Score: %f\n", result.AdditionalProperties["title"], *result.Score)
						}

						userMessage = fmt.Sprintf("Question: \"%s\"\n\nContext:\n%s", userMessage, strings.Join(contextResults, "\n\n"))
					}
				}

				if totalTokenCount > 1500 {
					summarizedMessage, err := summarizeMessages(ctx, openAiClient, messages, aiConfig)
					if err != nil {
						return err
					}

					messages = []azopenai.ChatRequestMessageClassification{&azopenai.ChatRequestUserMessage{
						Content: azopenai.NewChatRequestUserMessageContent(summarizedMessage),
					}}

					log.Printf("Summarized message: %s\n", summarizedMessage)
				}

				var chatResponse *azopenai.ChatCompletions
				messages = append(messages, &azopenai.ChatRequestUserMessage{
					Content: azopenai.NewChatRequestUserMessageContent(userMessage),
				})

				err = thinkingSpinner.Run(ctx, func(ctx context.Context) error {
					response, err := openAiClient.GetChatCompletions(ctx, azopenai.ChatCompletionsOptions{
						Messages:       messages,
						DeploymentName: &aiConfig.Models.ChatCompletion,
						Temperature:    &flags.temperature,
						ResponseFormat: &azopenai.ChatCompletionsTextResponseFormat{},
						MaxTokens:      to.Ptr(flags.maxTokens),
						Functions: []azopenai.FunctionDefinition{
							{
								Name:        to.Ptr("get-current-time"),
								Description: to.Ptr("Get the current date and time"),
								Parameters:  nil,
							},
						},
					}, nil)
					if err != nil {
						return err
					}

					chatResponse = &response.ChatCompletions
					if chatResponse.Choices[0].Message.FunctionCall != nil && *chatResponse.Choices[0].Message.FunctionCall.Name == "get-current-time" {
						data := getDateTime()

						messages = append(messages, &azopenai.ChatRequestAssistantMessage{
							Name:    to.Ptr("get-current-time"),
							Content: azopenai.NewChatRequestAssistantMessageContent(data),
						})

						functionResponse, err := openAiClient.GetChatCompletions(ctx, azopenai.ChatCompletionsOptions{
							Messages:       messages,
							DeploymentName: &aiConfig.Models.ChatCompletion,
							Temperature:    &flags.temperature,
							ResponseFormat: &azopenai.ChatCompletionsTextResponseFormat{},
							MaxTokens:      to.Ptr(flags.maxTokens),
							Functions: []azopenai.FunctionDefinition{
								{
									Name:        to.Ptr("get-current-time"),
									Description: to.Ptr("Get the current date and time"),
									Parameters:  nil,
								},
							},
						}, nil)
						if err != nil {
							return err
						}

						chatResponse = &functionResponse.ChatCompletions
					}
					return nil
				})

				if err != nil {
					return err
				}

				var assistantMessage string

				for _, choice := range chatResponse.Choices {
					if choice.Message != nil && choice.Message.Content != nil {
						assistantMessage = *choice.Message.Content
						fmt.Printf("%s: %s\n", color.CyanString("AI"), assistantMessage)
					}
				}

				messages = append(messages, &azopenai.ChatRequestAssistantMessage{
					Content: azopenai.NewChatRequestAssistantMessageContent(assistantMessage),
				})

				color.HiBlack("(Usage: Completion: %d, Prompt: %d, Total: %d)\n", *chatResponse.Usage.CompletionTokens, *chatResponse.Usage.PromptTokens, *chatResponse.Usage.TotalTokens)
				fmt.Println()

				totalTokenCount += ((len(userMessage) / 4) + (len(assistantMessage) / 4))
				userMessage = ""
			}

			return nil
		},
	}

	chatCmd.Flags().StringVar(&flags.systemMessage, "system-message", defaultSystemMessage, "System message to send to the AI model")
	chatCmd.Flags().Float32Var(&flags.temperature, "temperature", defaultTemperature, "Temperature for sampling")
	chatCmd.Flags().Int32Var(&flags.maxTokens, "max-tokens", defaultMaxTokens, "Maximum number of tokens to generate")
	chatCmd.Flags().StringVarP(&flags.message, "message", "m", "", "Message to send to the AI model")
	chatCmd.Flags().StringVarP(&flags.modelName, "model deployment name", "d", "", "Name of the model to use")
	chatCmd.Flags().BoolVar(&flags.useSearch, "use-search", false, "Use Azure Cognitive Search for search results")

	return chatCmd
}

func getDateTime() string {
	return time.Now().Format(time.RFC1123)
}

func convertToFloatPtrSlice(input []float32) []*float32 {
	result := make([]*float32, len(input))
	for i := range input {
		result[i] = &input[i]
	}

	return result
}

func summarizeMessages(ctx context.Context, openAiClient *azopenai.Client, messages []azopenai.ChatRequestMessageClassification, aiConfig *internal.AiConfig) (string, error) {
	// Concatenate message content to form a single text
	var content []string
	for _, msg := range messages {
		switch m := msg.(type) {

		case *azopenai.ChatRequestUserMessage:
			contentBytes, err := json.Marshal(m.Content)
			if err != nil {
				continue
			}

			content = append(content, fmt.Sprintf("User: %s", string(contentBytes)))
		case *azopenai.ChatRequestAssistantMessage:
			contentBytes, err := json.Marshal(m.Content)
			if err != nil {
				continue
			}

			content = append(content, fmt.Sprintf("AI: %s", string(contentBytes)))
		}
	}

	// Create a summarization prompt
	summarizationPrompt := fmt.Sprintf("Summarize the following conversation:\n\n%s", strings.Join(content, "\n"))
	log.Printf("Summarization prompt: %s\n", summarizationPrompt)

	// Send to OpenAI for summarization
	response, err := openAiClient.GetChatCompletions(ctx, azopenai.ChatCompletionsOptions{
		Messages: []azopenai.ChatRequestMessageClassification{
			&azopenai.ChatRequestUserMessage{
				Content: azopenai.NewChatRequestUserMessageContent(summarizationPrompt),
			},
		},
		DeploymentName: &aiConfig.Models.ChatCompletion,
		MaxTokens:      to.Ptr(int32(500)), // Adjust token limit for summary response
	}, nil)
	if err != nil {
		return "", err
	}

	// Extract summarized content
	if len(response.Choices) > 0 {
		return *response.Choices[0].Message.Content, nil
	}
	return "", errors.New("no summary generated")
}

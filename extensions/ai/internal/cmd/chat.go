package cmd

import (
	"context"
	"errors"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/ai/azopenai"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/cognitiveservices/armcognitiveservices"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/wbreza/azd-extensions/extensions/ai/internal"
	"github.com/wbreza/azd-extensions/sdk/ext"
	"github.com/wbreza/azd-extensions/sdk/ext/output"
	"github.com/wbreza/azd-extensions/sdk/ux"
)

type chatUsageFlags struct {
	message        string
	systemMessage  string
	subscriptionId string
	resourceGroup  string
	serviceName    string
	modelName      string
	temperature    float32
	maxTokens      int32
}

var (
	defaultSystemMessage = "You are an AI assistant that helps people find information."
	defaultTemperature   = float32(0.7)
	defaultMaxTokens     = int32(800)
)

func newChatCommand() *cobra.Command {
	chatFlags := &chatUsageFlags{}

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

			azdContext.Invoke(func(clientOptions *arm.ClientOptions) error {
				armClientOptions = clientOptions
				return nil
			})

			credential, err := azdContext.Credential()
			if err != nil {
				return err
			}

			var aiConfig *internal.AiConfig
			if chatFlags.subscriptionId != "" && chatFlags.resourceGroup != "" && chatFlags.serviceName != "" {
				aiConfig = &internal.AiConfig{
					Subscription:  chatFlags.subscriptionId,
					ResourceGroup: chatFlags.resourceGroup,
					Service:       chatFlags.serviceName,
				}
			} else {
				aiConfig, err = internal.LoadOrPromptAiConfig(ctx, azdContext)
				if err != nil {
					return err
				}
			}

			if chatFlags.modelName != "" {
				aiConfig.Models.ChatCompletion = chatFlags.modelName
			}

			if aiConfig.Models.ChatCompletion == "" {
				color.Yellow("No chat completion model was found. Please select or create a chat completion model.")

				selectedDeployment, err := internal.PromptModelDeployment(ctx, azdContext, aiConfig, &internal.PromptModelDeploymentOptions{
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

				aiConfig.Models.ChatCompletion = *selectedDeployment.Name
				if err := internal.SaveAiConfig(ctx, azdContext, aiConfig); err != nil {
					return err
				}

				fmt.Println()
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

			// keysResponse, err := accountClient.ListKeys(ctx, aiConfig.ResourceGroup, aiConfig.Service, nil)
			// if err != nil {
			// 	return err
			// }

			// keyCredential := azcore.NewKeyCredential(*keysResponse.Key1)

			endpoint := *account.Properties.Endpoint
			openAiClient, err := azopenai.NewClient(endpoint, credential, nil)
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

			loadingSpinner.Stop(ctx)

			fmt.Printf("AI Service: %s %s\n", color.CyanString(aiConfig.Service), color.HiBlackString("(%s)", aiConfig.ResourceGroup))
			fmt.Printf("Model: %s %s\n", color.CyanString(aiConfig.Models.ChatCompletion), color.HiBlackString("(Model: %s, Version: %s)", *deployment.Properties.Model.Name, *deployment.Properties.Model.Version))
			fmt.Printf("System Message: %s\n", color.CyanString(chatFlags.systemMessage))
			fmt.Printf("Temperature: %s %s\n", color.CyanString(fmt.Sprint(chatFlags.temperature)), color.HiBlackString("(Controls randomness)"))
			fmt.Printf("Max Tokens: %s %s\n", color.CyanString(fmt.Sprint(chatFlags.maxTokens)), color.HiBlackString("(Maximum number of tokens to generate)"))
			fmt.Println()

			messages := []azopenai.ChatRequestMessageClassification{}
			messages = append(messages, &azopenai.ChatRequestSystemMessage{
				Content: azopenai.NewChatRequestSystemMessageContent(chatFlags.systemMessage),
			})

			thinkingSpinner := ux.NewSpinner(&ux.SpinnerConfig{
				Text: "Thinking...",
			})

			userMessage := chatFlags.message

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

				fmt.Printf("%s: %s\n", color.GreenString("User"), color.HiBlackString(userMessage))
				fmt.Println()

				messages = append(messages, &azopenai.ChatRequestUserMessage{
					Content: azopenai.NewChatRequestUserMessageContent(userMessage),
				})

				var chatResponse *azopenai.ChatCompletions

				err = thinkingSpinner.Run(ctx, func(ctx context.Context) error {
					response, err := openAiClient.GetChatCompletions(ctx, azopenai.ChatCompletionsOptions{
						Messages:       messages,
						DeploymentName: &aiConfig.Models.ChatCompletion,
						Temperature:    &chatFlags.temperature,
						ResponseFormat: &azopenai.ChatCompletionsTextResponseFormat{},
						MaxTokens:      &chatFlags.maxTokens,
					}, nil)
					if err != nil {
						return err
					}

					chatResponse = &response.ChatCompletions
					return nil
				})

				if err != nil {
					return err
				}

				for _, choice := range chatResponse.Choices {
					fmt.Printf("%s: %s\n", color.CyanString("AI"), *choice.Message.Content)
				}

				fmt.Println()
				userMessage = ""
			}

			return nil
		},
	}

	chatCmd.Flags().StringVar(&chatFlags.systemMessage, "system-message", defaultSystemMessage, "System message to send to the AI model")
	chatCmd.Flags().Float32Var(&chatFlags.temperature, "temperature", defaultTemperature, "Temperature for sampling")
	chatCmd.Flags().Int32Var(&chatFlags.maxTokens, "max-tokens", defaultMaxTokens, "Maximum number of tokens to generate")
	chatCmd.Flags().StringVarP(&chatFlags.message, "message", "m", "", "Message to send to the AI model")
	chatCmd.Flags().StringVarP(&chatFlags.modelName, "model deployment name", "d", "", "Name of the model to use")
	chatCmd.Flags().StringVarP(&chatFlags.resourceGroup, "resource-group", "g", "", "Azure resource group")
	chatCmd.Flags().StringVarP(&chatFlags.serviceName, "name", "n", "", "Azure AI service name")
	chatCmd.Flags().StringVarP(&chatFlags.subscriptionId, "subscription", "s", "", "Azure subscription ID")

	return chatCmd
}

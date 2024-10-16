package cmd

import (
	"errors"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/ai/azopenai"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/cognitiveservices/armcognitiveservices"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/wbreza/azd-extensions/extensions/ai/internal/service"
	"github.com/wbreza/azd-extensions/sdk/ext"
	"github.com/wbreza/azd-extensions/sdk/ext/output"
	"github.com/wbreza/azd-extensions/sdk/ux"
)

type chatUsageFlags struct {
	systemMessage string
	temperature   float32
	maxTokens     int32
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

			credential, err := azdContext.Credential()
			if err != nil {
				return err
			}

			aiConfig, err := service.LoadOrPrompt(ctx, azdContext)
			if err != nil {
				return err
			}

			if aiConfig.Model == "" {
				selectedDeployment, err := service.PromptModelDeployment(ctx, azdContext)
				if err != nil {
					if errors.Is(err, service.ErrNoModelDeployments) {
						return &ext.ErrorWithSuggestion{
							Err:        err,
							Suggestion: fmt.Sprintf("Run %s to create a model deployment", color.CyanString("azd ai model deployment create")),
						}
					}
					return err
				}

				aiConfig.Model = *selectedDeployment.Name
				if err := service.Save(ctx, azdContext, aiConfig); err != nil {
					return err
				}

				fmt.Println()
			}

			loadingSpinner := ux.NewSpinner(&ux.SpinnerConfig{
				Text:        "Starting chat...",
				ClearOnStop: true,
			})

			loadingSpinner.Start(ctx)

			accountClient, err := armcognitiveservices.NewAccountsClient(aiConfig.Subscription, credential, nil)
			if err != nil {
				return err
			}

			account, err := accountClient.Get(ctx, aiConfig.ResourceGroup, aiConfig.Service, nil)
			if err != nil {
				return err
			}

			keysResponse, err := accountClient.ListKeys(ctx, aiConfig.ResourceGroup, aiConfig.Service, nil)
			if err != nil {
				return err
			}

			keyCredential := azcore.NewKeyCredential(*keysResponse.Key1)

			endpointName := "OpenAI Language Model Instance API"
			endpoint := *account.Properties.Endpoints[endpointName]
			chatClient, err := azopenai.NewClientWithKeyCredential(endpoint, keyCredential, nil)
			if err != nil {
				return err
			}

			deploymentsClient, err := armcognitiveservices.NewDeploymentsClient(aiConfig.Subscription, credential, nil)
			if err != nil {
				return err
			}

			deployment, err := deploymentsClient.Get(ctx, aiConfig.ResourceGroup, aiConfig.Service, aiConfig.Model, nil)
			if err != nil {
				return err
			}

			loadingSpinner.Stop(ctx)

			fmt.Printf("AI Service: %s %s\n", color.CyanString(aiConfig.Service), color.HiBlackString("(%s)", aiConfig.ResourceGroup))
			fmt.Printf("Model: %s %s\n", color.CyanString(aiConfig.Model), color.HiBlackString("(Model: %s, Version: %s)", *deployment.Properties.Model.Name, *deployment.Properties.Model.Version))
			fmt.Printf("System Message: %s\n", color.CyanString(chatFlags.systemMessage))
			fmt.Println()

			messages := []azopenai.ChatRequestMessageClassification{}
			messages = append(messages, &azopenai.ChatRequestSystemMessage{
				Content: azopenai.NewChatRequestSystemMessageContent(chatFlags.systemMessage),
			})

			for {
				chatPrompt := ux.NewPrompt(&ux.PromptConfig{
					Message:           "User",
					PlaceHolder:       "Press `Ctrl+C` to cancel",
					ClearOnCompletion: true,
					CaptureHintKeys:   false,
				})

				chatMessage, err := chatPrompt.Ask()
				if err != nil {
					if errors.Is(err, ux.ErrCancelled) {
						break
					}

					return err
				}

				fmt.Printf("%s: %s\n", color.GreenString("User"), color.HiBlackString(chatMessage))
				fmt.Println()

				messages = append(messages, &azopenai.ChatRequestUserMessage{
					Content: azopenai.NewChatRequestUserMessageContent(chatMessage),
				})

				chatResponse, err := chatClient.GetChatCompletions(ctx, azopenai.ChatCompletionsOptions{
					Messages:       messages,
					DeploymentName: &aiConfig.Model,
					Temperature:    &chatFlags.temperature,
					ResponseFormat: &azopenai.ChatCompletionsTextResponseFormat{},
					MaxTokens:      &chatFlags.maxTokens,
				}, nil)
				if err != nil {
					return err
				}

				for _, choice := range chatResponse.Choices {
					fmt.Printf("%s: %s\n", color.CyanString("AI"), *choice.Message.Content)
				}

				fmt.Println()
			}

			return nil
		},
	}

	chatCmd.Flags().StringVar(&chatFlags.systemMessage, "system-message", defaultSystemMessage, "System message to send to the AI model")
	chatCmd.Flags().Float32Var(&chatFlags.temperature, "temperature", defaultTemperature, "Temperature for sampling")
	chatCmd.Flags().Int32Var(&chatFlags.maxTokens, "max-tokens", defaultMaxTokens, "Maximum number of tokens to generate")

	return chatCmd
}

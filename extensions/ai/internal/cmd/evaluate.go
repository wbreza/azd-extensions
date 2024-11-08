package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/wbreza/azd-extensions/extensions/ai/internal"
	"github.com/wbreza/azd-extensions/sdk/common"
	"github.com/wbreza/azd-extensions/sdk/common/permissions"
	"github.com/wbreza/azd-extensions/sdk/ext"
	"github.com/wbreza/azd-extensions/sdk/ext/output"
	"github.com/wbreza/azd-extensions/sdk/ux"
)

var ErrTestCaseFailed = errors.New("evaluation failed")

// Function to create the `evaluate` command group
func newEvaluateCommand() *cobra.Command {
	evaluateCmd := &cobra.Command{
		Use:   "evaluate",
		Short: "Run evaluations to assess model performance",
	}

	// Add subcommands to the `evaluate` command group
	evaluateCmd.AddCommand(newEvaluateFlowCommand())
	evaluateCmd.AddCommand(newEvaluateModelCommand())

	return evaluateCmd
}

func newEvaluateFlowCommand() *cobra.Command {
	flags := &EvaluateFlowFlags{}

	flowCmd := &cobra.Command{
		Use:   "flow",
		Short: "Evaluate model flow based on a test dataset",
		RunE: func(cmd *cobra.Command, args []string) error {
			header := output.CommandHeader{
				Title:       "Evaluate an AI flow (azd ai evaluate flow)",
				Description: "Evaluates an AI flow for accuracy, latency, and token usage based on a test dataset.",
			}
			header.Print()

			ctx := cmd.Context()

			if flags.Output == "" {
				currentTime := time.Now()
				timestamp := currentTime.Format("20060102_150405")
				filename := fmt.Sprintf("flow_report_%s.json", timestamp)
				flags.Output = filepath.Join("evaluations", filename)
			}

			folderPath := filepath.Dir(flags.Output)
			if err := os.MkdirAll(folderPath, permissions.PermissionDirectory); err != nil {
				return err
			}

			testData, err := loadTestData(flags.TestData)
			if err != nil {
				return fmt.Errorf("failed to load test data: %w", err)
			}

			evalOptions := internal.EvaluationOptions{
				EvaluationType:           internal.EvaluationTypeFlow,
				ChatCompletionModel:      flags.ChatDeploymentName,
				EmbeddingModel:           flags.EmbeddingDeploymentName,
				IndexName:                flags.IndexName,
				FuzzyMatchThreshold:      testData.FuzzyMatchThreshold,
				SimilarityMatchThreshold: testData.SimilarityMatchThreshold,
				BatchSize:                flags.BatchSize,
			}

			fmt.Printf("Running evaluation against %s\n", color.CyanString(flags.TestData))

			evalReport, err := runEvaluation(ctx, testData, evalOptions)
			if err != nil {
				return err
			}

			if err := saveEvaluationReport(evalReport, flags.Output); err != nil {
				return fmt.Errorf("failed to save evaluation report: %w", err)
			}

			printEvaluationReportResults(evalReport)

			fmt.Println()
			fmt.Printf("Evaluation report saved to: %s\n", color.CyanString(flags.Output))
			fmt.Println()

			color.Green("SUCCESS: Flow evaluation completed.")
			return nil
		},
	}

	// Define flags for the `accuracy` command
	flowCmd.Flags().StringVar(&flags.ChatDeploymentName, "chat-deployment-name", "", "Name of the chat completion model deployment to evaluate")
	flowCmd.Flags().StringVar(&flags.EmbeddingDeploymentName, "embedding-deployment-name", "", "Name of the embedding model deployment to evaluate")
	flowCmd.Flags().StringVar(&flags.IndexName, "index-name", "", "Name of the search index to evaluate")
	flowCmd.Flags().StringVar(&flags.TestData, "test-data", "", "Path to JSON file with test questions and expected answers (required)")
	flowCmd.Flags().StringVar(&flags.Output, "output", "", "Path to save the accuracy evaluation report")
	flowCmd.Flags().IntVar(&flags.BatchSize, "batch-size", 1, "Number of test cases to evaluate in parallel")

	_ = flowCmd.MarkFlagRequired("test-data")

	return flowCmd
}

func newEvaluateModelCommand() *cobra.Command {
	flags := &EvaluateModelFlags{}

	modelCmd := &cobra.Command{
		Use:   "model",
		Short: "Evaluate model based on a test dataset",
		RunE: func(cmd *cobra.Command, args []string) error {
			header := output.CommandHeader{
				Title:       "Evaluate an AI model (azd ai evaluate model)",
				Description: "Evaluates an AI model for accuracy, latency, and token usage based on a test dataset.",
			}
			header.Print()

			ctx := cmd.Context()

			if flags.Output == "" {
				currentTime := time.Now()
				timestamp := currentTime.Format("20060102_150405")
				filename := fmt.Sprintf("model_report_%s.json", timestamp)
				flags.Output = filepath.Join("evaluations", filename)
			}

			folderPath := filepath.Dir(flags.Output)
			if err := os.MkdirAll(folderPath, permissions.PermissionDirectory); err != nil {
				return err
			}

			testData, err := loadTestData(flags.TestData)
			if err != nil {
				return fmt.Errorf("failed to load test data: %w", err)
			}

			evalOptions := internal.EvaluationOptions{
				EvaluationType:           internal.EvaluationTypeModel,
				ChatCompletionModel:      flags.DeploymentName,
				FuzzyMatchThreshold:      testData.FuzzyMatchThreshold,
				SimilarityMatchThreshold: testData.SimilarityMatchThreshold,
				BatchSize:                flags.BatchSize,
			}

			fmt.Printf("Running evaluation against %s\n", color.CyanString(flags.TestData))

			evalReport, err := runEvaluation(ctx, testData, evalOptions)
			if err != nil {
				return err
			}

			if err := saveEvaluationReport(evalReport, flags.Output); err != nil {
				return fmt.Errorf("failed to save evaluation report: %w", err)
			}

			printEvaluationReportResults(evalReport)

			fmt.Println()
			fmt.Printf("Evaluation report saved to: %s\n", color.CyanString(flags.Output))
			fmt.Println()

			color.Green("SUCCESS: Model evaluation completed.")
			return nil
		},
	}

	// Define flags for the `accuracy` command
	modelCmd.Flags().StringVar(&flags.DeploymentName, "chat-deployment-name", "", "Name of the chat completion model deployment to evaluate")
	modelCmd.Flags().StringVar(&flags.TestData, "test-data", "", "Path to JSON file with test questions and expected answers (required)")
	modelCmd.Flags().StringVar(&flags.Output, "output", "", "Path to save the accuracy evaluation report")
	modelCmd.Flags().IntVar(&flags.BatchSize, "batch-size", 1, "Number of test cases to evaluate in parallel")

	_ = modelCmd.MarkFlagRequired("test-data")

	return modelCmd
}

// Flag structs for each evaluation command
type EvaluateFlowFlags struct {
	ChatDeploymentName      string
	EmbeddingDeploymentName string
	IndexName               string
	TestData                string
	Output                  string
	BatchSize               int
}

// Flag structs for each evaluation command
type EvaluateModelFlags struct {
	DeploymentName string
	TestData       string
	Output         string
	BatchSize      int
}

func runEvaluation(ctx context.Context, testData *internal.EvaluationTestData, options internal.EvaluationOptions) (*internal.EvaluationReport, error) {
	azdContext, err := ext.CurrentContext(ctx)
	if err != nil {
		return nil, err
	}

	aiConfig, err := internal.LoadOrPromptAiConfig(ctx, azdContext)
	if err != nil {
		return nil, err
	}

	if options.ChatCompletionModel == "" && aiConfig.Models.ChatCompletion == "" {
		chatModel, err := internal.PromptModelDeployment(ctx, azdContext, aiConfig, &internal.PromptModelDeploymentOptions{
			Capabilities: []string{"chatCompletion"},
		})
		if err != nil {
			return nil, err
		}

		aiConfig.Models.ChatCompletion = *chatModel.Name
	}

	if options.EvaluationType == internal.EvaluationTypeFlow {
		if options.EmbeddingModel == "" && aiConfig.Models.Embeddings == "" {
			embeddingModel, err := internal.PromptModelDeployment(ctx, azdContext, aiConfig, &internal.PromptModelDeploymentOptions{
				Capabilities: []string{"embeddings"},
			})
			if err != nil {
				return nil, err
			}

			aiConfig.Models.Embeddings = *embeddingModel.Name
		}

		if aiConfig.Search.Service == "" {
			searchService, err := internal.PromptSearchService(ctx, azdContext, aiConfig)
			if err != nil {
				return nil, err
			}

			aiConfig.Search.Service = *searchService.Name
		}

		if options.IndexName == "" && aiConfig.Search.Index == "" {
			searchIndex, err := internal.PromptSearchIndex(ctx, azdContext, aiConfig)
			if err != nil {
				return nil, err
			}

			aiConfig.Search.Index = *searchIndex.Name
		}
	}

	if options.ChatCompletionModel == "" {
		options.ChatCompletionModel = aiConfig.Models.ChatCompletion
	}

	if options.EmbeddingModel == "" {
		options.EmbeddingModel = aiConfig.Models.Embeddings
	}

	if options.IndexName == "" {
		options.IndexName = aiConfig.Search.Index
	}

	fmt.Printf("Chat Completion Model: %s\n", color.CyanString(options.ChatCompletionModel))
	if options.EvaluationType == internal.EvaluationTypeFlow {
		fmt.Printf("Embedding Model: %s\n", color.CyanString(options.EmbeddingModel))
	}

	if err := internal.SaveAiConfig(ctx, azdContext, aiConfig); err != nil {
		return nil, err
	}

	evalService, err := internal.NewEvalService(ctx, azdContext, aiConfig)
	if err != nil {
		return nil, err
	}

	testCaseResults := []*internal.EvaluationTestCaseResult{}
	taskList := ux.NewTaskList(&ux.TaskListConfig{
		MaxConcurrentAsync: options.BatchSize,
	})

	for _, testCase := range testData.TestCases {
		taskList.AddTask(ux.TaskOptions{
			Title: fmt.Sprintf("%s: %s", testCase.Id, testCase.Question),
			Async: true,
			Action: func(spf ux.SetProgressFunc) (ux.TaskState, error) {
				testCaseResult, err := evalService.EvaluateTestCase(ctx, testCase, options)
				if err != nil {
					return ux.Error, common.NewDetailedError("Evaluation Failed", err)
				}

				testCaseResults = append(testCaseResults, testCaseResult)

				if !testCaseResult.Correct {
					return ux.Warning, common.NewDetailedError("Incorrect", ErrTestCaseFailed)
				}

				return ux.Success, nil
			},
		})
	}

	if err := taskList.Run(); err != nil && !errors.Is(err, ErrTestCaseFailed) {
		return nil, err
	}

	evalReport := evalService.GenerateReport(testCaseResults)

	return evalReport, nil
}

func printEvaluationReportResults(evalReport *internal.EvaluationReport) {
	// Print out the eval result
	color.Cyan("Accuracy")
	fmt.Printf("Accuracy: %.2f\n", evalReport.Metrics.Accuracy)
	fmt.Printf("Precision: %.2f\n", evalReport.Metrics.Precision)
	fmt.Printf("Recall: %.2f\n", evalReport.Metrics.Recall)
	fmt.Printf("F1 Score: %.2f\n", evalReport.Metrics.F1)
	fmt.Println()

	color.Cyan("Latency")
	fmt.Printf("Total Duration: %d ms\n", evalReport.Metrics.Latency.TotalDuration)
	fmt.Printf("Average Latency: %.2f ms\n", evalReport.Metrics.Latency.AvgDuration)
	fmt.Printf("Median Duration: %d ms\n", evalReport.Metrics.Latency.MedianLatency)
	fmt.Printf("Max Duration: %d ms\n", evalReport.Metrics.Latency.MaxDuration)
	fmt.Printf("Min Duration: %d ms\n", evalReport.Metrics.Latency.MinDuration)
	fmt.Println()

	color.Cyan("Token Usage")
	fmt.Printf("Total Tokens: %d\n", evalReport.Metrics.TokenUsage.TotalTokens)
	fmt.Printf("Average Tokens: %.2f\n", evalReport.Metrics.TokenUsage.AvgTokens)
	fmt.Printf("Median Tokens: %d\n", evalReport.Metrics.TokenUsage.MedianTokens)
	fmt.Printf("Max Tokens: %d\n", evalReport.Metrics.TokenUsage.MaxTokens)
	fmt.Printf("Min Tokens: %d\n", evalReport.Metrics.TokenUsage.MinTokens)
}

// Function to load test data from JSON file
func loadTestData(filename string) (*internal.EvaluationTestData, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	bytes, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}

	var testData internal.EvaluationTestData
	if err := json.Unmarshal(bytes, &testData); err != nil {
		return nil, err
	}

	return &testData, nil
}

// Stub for querying the model with a question

// Function to save the evaluation report as a JSON file
func saveEvaluationReport(results *internal.EvaluationReport, filename string) error {
	bytes, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filename, bytes, permissions.PermissionFile)
}

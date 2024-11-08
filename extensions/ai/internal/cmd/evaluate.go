package cmd

import (
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
	"github.com/wbreza/azd-extensions/sdk/ux"
)

// Function to create the `evaluate` command group
func newEvaluateCommand() *cobra.Command {
	evaluateCmd := &cobra.Command{
		Use:   "evaluate",
		Short: "Run evaluations to assess model performance",
	}

	// Add subcommands to the `evaluate` command group
	evaluateCmd.AddCommand(newEvaluateAccuracyCommand())
	evaluateCmd.AddCommand(newEvaluateLatencyCommand())
	evaluateCmd.AddCommand(newEvaluateTokensCommand())

	return evaluateCmd
}

// `evaluate accuracy` command stub (fully implemented as previously discussed)
func newEvaluateAccuracyCommand() *cobra.Command {
	flags := &AccuracyFlags{}

	accuracyCmd := &cobra.Command{
		Use:   "accuracy",
		Short: "Evaluate model accuracy based on a test dataset",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			if flags.Output == "" {
				currentTime := time.Now()
				timestamp := currentTime.Format("20060102_150405")
				filename := fmt.Sprintf("accuracy_report_%s.json", timestamp)
				flags.Output = filepath.Join("evaluations", filename)
			}

			folderPath := filepath.Dir(flags.Output)
			if err := os.MkdirAll(folderPath, permissions.PermissionDirectory); err != nil {
				return err
			}

			azdContext, err := ext.CurrentContext(ctx)
			if err != nil {
				return err
			}

			aiConfig, err := internal.LoadOrPromptAiConfig(ctx, azdContext)
			if err != nil {
				return err
			}

			if aiConfig.Models.ChatCompletion == "" {
				chatModel, err := internal.PromptModelDeployment(ctx, azdContext, aiConfig, &internal.PromptModelDeploymentOptions{
					Capabilities: []string{"chatCompletion"},
				})
				if err != nil {
					return err
				}

				aiConfig.Models.ChatCompletion = *chatModel.Name
			}

			if aiConfig.Models.Embeddings == "" {
				embeddingModel, err := internal.PromptModelDeployment(ctx, azdContext, aiConfig, &internal.PromptModelDeploymentOptions{
					Capabilities: []string{"embeddings"},
				})
				if err != nil {
					return err
				}

				aiConfig.Models.Embeddings = *embeddingModel.Name
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

			// Step 1: Load test data
			testData, err := loadTestData(flags.TestData)
			if err != nil {
				return fmt.Errorf("failed to load test data: %w", err)
			}

			// Step 2: Initialize EvaluationResult
			evaluationResult := &EvaluationResult{}
			evalService, err := internal.NewEvalService(ctx, azdContext, aiConfig)
			if err != nil {
				return err
			}

			evalOptions := internal.EvaluateOptions{
				ChatCompletionModel: aiConfig.Models.ChatCompletion,
				EmbeddingModel:      aiConfig.Models.Embeddings,
			}

			taskList := ux.NewTaskList(nil)

			// Step 3: Evaluate each test case
			for _, testCase := range testData.TestCases {
				taskList.AddTask(ux.TaskOptions{
					Title: fmt.Sprintf("%s: %s", testCase.Id, testCase.Question),
					Action: func(spf ux.SetProgressFunc) (ux.TaskState, error) {
						isCorrect, err := evalService.Evaluate(ctx, testCase, evalOptions)
						if err != nil {
							return ux.Error, common.NewDetailedError("Evaluation Failed", err)
						}

						updateEvaluationMetrics(evaluationResult, isCorrect)

						if !isCorrect {
							return ux.Warning, common.NewDetailedError("Incorrect", errors.New("model response did not match expected answers"))
						}

						return ux.Success, nil
					},
				})
			}

			if err := taskList.Run(); err != nil {
				return err
			}

			// Step 4: Calculate metrics
			accuracy := calculateAccuracy(evaluationResult)
			precision := calculatePrecision(evaluationResult)
			recall := calculateRecall(evaluationResult)
			f1Score := calculateF1Score(precision, recall)

			// Step 5: Save evaluation report
			if err := saveEvaluationReport(flags.Output, accuracy, precision, recall, f1Score); err != nil {
				return fmt.Errorf("failed to save evaluation report: %w", err)
			}

			// Print out the eval result
			fmt.Printf("Accuracy: %.2f\n", accuracy)
			fmt.Printf("Precision: %.2f\n", precision)
			fmt.Printf("Recall: %.2f\n", recall)
			fmt.Printf("F1 Score: %.2f\n", f1Score)

			fmt.Println()
			color.Green("Accuracy evaluation completed.")
			return nil
		},
	}

	// Define flags for the `accuracy` command
	accuracyCmd.Flags().StringVar(&flags.DeploymentName, "deployment-name", "", "Name of the model deployment to evaluate (required)")
	accuracyCmd.Flags().StringVar(&flags.TestData, "test-data", "", "Path to JSON file with test questions and expected answers (required)")
	accuracyCmd.Flags().StringVar(&flags.Metrics, "metrics", "accuracy,precision,recall,f1", "Metrics to calculate (e.g., 'accuracy,precision,recall,f1')")
	accuracyCmd.Flags().StringVar(&flags.Output, "output", "", "Path to save the accuracy evaluation report")

	_ = accuracyCmd.MarkFlagRequired("test-data")

	return accuracyCmd
}

// `evaluate latency` command stub
func newEvaluateLatencyCommand() *cobra.Command {
	flags := &LatencyFlags{}

	latencyCmd := &cobra.Command{
		Use:   "latency",
		Short: "Evaluate model latency performance for specified queries",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Stub implementation for evaluating latency
			fmt.Println("Running latency evaluation...")
			return nil
		},
	}

	// Define flags for the `latency` command
	latencyCmd.Flags().StringVar(&flags.DeploymentName, "deployment-name", "", "Name of the model deployment to evaluate (required)")
	latencyCmd.Flags().IntVar(&flags.NumQueries, "num-queries", 100, "Number of test queries to run")
	latencyCmd.Flags().IntVar(&flags.Concurrency, "concurrency", 1, "Number of concurrent requests to simulate load")
	latencyCmd.Flags().StringVar(&flags.Output, "output", "latency_report.json", "Path to save the latency evaluation report")

	return latencyCmd
}

// `evaluate tokens` command stub
func newEvaluateTokensCommand() *cobra.Command {
	flags := &TokensFlags{}

	tokensCmd := &cobra.Command{
		Use:   "tokens",
		Short: "Evaluate token usage and estimate cost based on specified queries",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Stub implementation for evaluating token usage
			fmt.Println("Running token usage evaluation...")
			return nil
		},
	}

	// Define flags for the `tokens` command
	tokensCmd.Flags().StringVar(&flags.DeploymentName, "deployment-name", "", "Name of the model deployment to evaluate (required)")
	tokensCmd.Flags().StringVar(&flags.TestData, "test-data", "", "Path to JSON file with sample prompts to evaluate token usage (required)")
	tokensCmd.Flags().Float64Var(&flags.CostPerToken, "cost-per-token", 0.00003, "Cost per token for estimating expenses")
	tokensCmd.Flags().StringVar(&flags.Output, "output", "token_usage.json", "Path to save the token usage report")

	_ = tokensCmd.MarkFlagRequired("test-data")

	return tokensCmd
}

// Flag structs for each evaluation command
type AccuracyFlags struct {
	DeploymentName string
	TestData       string
	Metrics        string
	Output         string
}

type LatencyFlags struct {
	DeploymentName string
	NumQueries     int
	Concurrency    int
	Output         string
}

type TokensFlags struct {
	DeploymentName string
	TestData       string
	CostPerToken   float64
	Output         string
}

type EvaluationResult struct {
	CorrectCount   int
	TotalCount     int
	TruePositives  int
	FalsePositives int
	FalseNegatives int
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
func saveEvaluationReport(filename string, accuracy, precision, recall, f1Score float64) error {
	report := map[string]interface{}{
		"accuracy":  accuracy,
		"precision": precision,
		"recall":    recall,
		"f1_score":  f1Score,
	}

	bytes, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filename, bytes, permissions.PermissionFile)
}

// Function to update evaluation metrics
func updateEvaluationMetrics(result *EvaluationResult, isCorrect bool) {
	result.TotalCount++
	if isCorrect {
		result.CorrectCount++
		result.TruePositives++
	} else {
		result.FalseNegatives++
	}
}

// Function to calculate accuracy
func calculateAccuracy(result *EvaluationResult) float64 {
	if result.TotalCount == 0 {
		return 0
	}
	return float64(result.CorrectCount) / float64(result.TotalCount)
}

// Function to calculate precision
func calculatePrecision(result *EvaluationResult) float64 {
	if (result.TruePositives + result.FalsePositives) == 0 {
		return 0
	}
	return float64(result.TruePositives) / float64(result.TruePositives+result.FalsePositives)
}

// Function to calculate recall
func calculateRecall(result *EvaluationResult) float64 {
	if (result.TruePositives + result.FalseNegatives) == 0 {
		return 0
	}
	return float64(result.TruePositives) / float64(result.TruePositives+result.FalseNegatives)
}

// Function to calculate F1 score
func calculateF1Score(precision, recall float64) float64 {
	if (precision + recall) == 0 {
		return 0
	}
	return 2 * (precision * recall) / (precision + recall)
}

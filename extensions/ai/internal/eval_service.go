package internal

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/ai/azopenai"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/texttheater/golang-levenshtein/levenshtein"
	"github.com/wbreza/azd-extensions/sdk/ext"
	"github.com/wbreza/azure-sdk-for-go/sdk/data/azsearchindex"
)

type EvalService struct {
	azdContext   *ext.Context
	aiConfig     *ExtensionConfig
	openAiClient *azopenai.Client
	searchClient *azsearchindex.DocumentsClient
}

func NewEvalService(ctx context.Context, azdContext *ext.Context, extensionConfig *ExtensionConfig) (*EvalService, error) {
	credential, err := azdContext.Credential()
	if err != nil {
		return nil, err
	}

	var azClientOptions *azcore.ClientOptions
	azdContext.Invoke(func(azOptions *azcore.ClientOptions) error {
		azClientOptions = azOptions
		return nil
	})

	openAiClient, err := azopenai.NewClient(extensionConfig.Ai.Endpoint, credential, &azopenai.ClientOptions{
		ClientOptions: *azClientOptions,
	})
	if err != nil {
		return nil, err
	}

	searchClient, err := azsearchindex.NewDocumentsClient(extensionConfig.Search.Endpoint, extensionConfig.Search.Index, credential, azClientOptions)
	if err != nil {
		return nil, err
	}

	return &EvalService{
		azdContext:   azdContext,
		aiConfig:     extensionConfig,
		openAiClient: openAiClient,
		searchClient: searchClient,
	}, nil
}

func (s *EvalService) EvaluateTestCase(ctx context.Context, testCase *EvaluationTestCase, options EvaluationOptions) (*EvaluationTestCaseResult, error) {
	if options.FuzzyMatchThreshold == nil {
		options.FuzzyMatchThreshold = to.Ptr(float32(0.8))
	}

	if options.SimilarityMatchThreshold == nil {
		options.SimilarityMatchThreshold = to.Ptr(float32(0.8))
	}

	modelResponse, err := s.queryModel(ctx, testCase, options)
	if err != nil {
		return nil, err
	}

	result, err := s.evaluateResponse(ctx, modelResponse, testCase, options)
	if err != nil {
		return nil, err
	}

	return result, nil
}

func (s *EvalService) GenerateReport(results []*EvaluationTestCaseResult) *EvaluationReport {
	overallResult := &EvaluationReport{}
	if len(results) == 0 {
		return overallResult
	}

	totalCount := len(results)
	latencies := make([]int, totalCount)
	tokens := make([]int, totalCount)

	var correctCount int
	var truePositives int
	var falsePositives int
	var falseNegatives int
	var totalDuration int
	var totalTokens int

	for i, testCaseResult := range results {
		latencies[i] = testCaseResult.Response.Duration
		tokens[i] = int(testCaseResult.Response.TokenUsage.TotalTokens)
		totalDuration += testCaseResult.Response.Duration
		totalTokens += int(testCaseResult.Response.TokenUsage.TotalTokens)

		if testCaseResult.Correct {
			correctCount++
			truePositives++
		} else {
			falseNegatives++
		}

		overallResult.Results = append(overallResult.Results, testCaseResult)
	}

	// Accuracy Metrics
	if totalCount > 0 {
		overallResult.Metrics.Accuracy = float32(correctCount) / float32(totalCount)
	}
	if truePositives+falsePositives > 0 {
		overallResult.Metrics.Precision = float32(truePositives) / float32(truePositives+falsePositives)
	}
	if truePositives+falseNegatives > 0 {
		overallResult.Metrics.Recall = float32(truePositives) / float32(truePositives+falseNegatives)
	}
	if overallResult.Metrics.Precision+overallResult.Metrics.Recall > 0 {
		overallResult.Metrics.F1 = 2 * (overallResult.Metrics.Precision * overallResult.Metrics.Recall) /
			(overallResult.Metrics.Precision + overallResult.Metrics.Recall)
	}

	// Latency Metrics
	overallResult.Metrics.Latency.TotalDuration = int64(totalDuration)
	if totalCount > 0 {
		overallResult.Metrics.Latency.AvgDuration = float64(totalDuration) / float64(totalCount)
	}

	sort.Slice(latencies, func(i, j int) bool {
		return latencies[i] < latencies[j]
	})

	// Set Min and Max Duration
	overallResult.Metrics.Latency.MinDuration = latencies[0]
	overallResult.Metrics.Latency.MaxDuration = latencies[totalCount-1]

	// Calculate Median Latency
	if totalCount > 0 {
		if totalCount%2 == 0 {
			// Even number of latencies: average of two middle values
			overallResult.Metrics.Latency.MedianLatency = (latencies[totalCount/2-1] + latencies[totalCount/2]) / 2
		} else {
			// Odd number of latencies: middle value
			overallResult.Metrics.Latency.MedianLatency = latencies[totalCount/2]
		}
	}

	// Token Usage Metrics
	overallResult.Metrics.TokenUsage.TotalTokens = int32(totalTokens)
	if totalCount > 0 {
		overallResult.Metrics.TokenUsage.AvgTokens = float32(totalTokens) / float32(totalCount)
	}

	sort.Slice(tokens, func(i, j int) bool {
		return tokens[i] < tokens[j]
	})

	// Set Min and Max Tokens
	overallResult.Metrics.TokenUsage.MinTokens = int32(tokens[0])
	overallResult.Metrics.TokenUsage.MaxTokens = int32(tokens[totalCount-1])

	// Calculate Median Tokens
	if totalCount > 0 {
		if totalCount%2 == 0 {
			// Even number of tokens: average of two middle values
			overallResult.Metrics.TokenUsage.MedianTokens = int32((tokens[totalCount/2-1] + tokens[totalCount/2]) / 2)
		} else {
			// Odd number of tokens: middle value
			overallResult.Metrics.TokenUsage.MedianTokens = int32(tokens[totalCount/2])
		}
	}

	return overallResult
}

func (s *EvalService) queryModel(ctx context.Context, testCase *EvaluationTestCase, options EvaluationOptions) (*ModelResponse, error) {
	startTime := time.Now()
	tokenUsage := TokenUsage{}
	chatMessage := testCase.Question

	// Only use embeddings for flow evaluation type
	if options.EvaluationType == EvaluationTypeFlow {
		embeddingsResponse, err := s.openAiClient.GetEmbeddings(ctx, azopenai.EmbeddingsOptions{
			Input:          []string{testCase.Question},
			DeploymentName: &options.EmbeddingModel,
		}, nil)
		if err != nil {
			return nil, err
		}

		tokenUsage.PromptTokens += *embeddingsResponse.Usage.PromptTokens
		tokenUsage.TotalTokens += *embeddingsResponse.Usage.TotalTokens

		searchResponse, err := s.searchClient.SearchPost(ctx, azsearchindex.SearchRequest{
			Select:    to.Ptr("chunk_id, chunk, title"),
			QueryType: to.Ptr(azsearchindex.QueryTypeSimple),
			VectorQueries: []azsearchindex.VectorQueryClassification{
				&azsearchindex.VectorizedQuery{
					Kind:       to.Ptr(azsearchindex.VectorQueryKindVector),
					Fields:     to.Ptr("text_vector"),
					Exhaustive: to.Ptr(true),
					K:          to.Ptr(int32(3)),
					Vector:     ConvertToFloatPtrSlice(embeddingsResponse.Data[0].Embedding),
				},
			},
		}, nil, nil)
		if err != nil {
			return nil, nil
		}

		if len(searchResponse.Results) > 0 {
			contextResults := make([]string, len(searchResponse.Results))

			for i, result := range searchResponse.Results {
				contextResults[i] = fmt.Sprintf("- [%d] %s", i+1, fmt.Sprint(result.AdditionalProperties["chunk"]))
			}

			chatMessage = fmt.Sprintf("Question: \"%s\"\n\nContext:\n%s", testCase.Question, strings.Join(contextResults, "\n\n"))
		}
	}

	chatMessages := []azopenai.ChatRequestMessageClassification{
		&azopenai.ChatRequestSystemMessage{
			Content: azopenai.NewChatRequestSystemMessageContent("You are a helpful AI assistant."),
		},
		&azopenai.ChatRequestUserMessage{
			Content: azopenai.NewChatRequestUserMessageContent(chatMessage),
		},
	}

	chatResponse, err := s.openAiClient.GetChatCompletions(ctx, azopenai.ChatCompletionsOptions{
		DeploymentName: &options.ChatCompletionModel,
		Messages:       chatMessages,
	}, nil)
	if err != nil {
		return nil, err
	}

	tokenUsage.PromptTokens += *chatResponse.Usage.PromptTokens
	tokenUsage.CompletionTokens += *chatResponse.Usage.CompletionTokens
	tokenUsage.TotalTokens += *chatResponse.Usage.TotalTokens

	return &ModelResponse{
		Message:    *chatResponse.ChatCompletions.Choices[0].Message.Content,
		Duration:   int(time.Since(startTime).Milliseconds()),
		TokenUsage: tokenUsage,
	}, nil
}

// Function to evaluate if model response is correct based on expected answers
func (s *EvalService) evaluateResponse(ctx context.Context, response *ModelResponse, testCase *EvaluationTestCase, options EvaluationOptions) (*EvaluationTestCaseResult, error) {
	result := &EvaluationTestCaseResult{
		Id:            testCase.Id,
		Question:      testCase.Question,
		Response:      response,
		AnswerResults: []*EvaluationTestCaseAnswerResult{},
	}

	var isCorrect bool

	// Step 1: Exact match
	for _, answer := range testCase.ExpectedAnswers {
		isCorrect = normalizeText(response.Message) == normalizeText(answer)
		score := 0
		if isCorrect {
			score = 1
		}

		answerResult := &EvaluationTestCaseAnswerResult{
			Expected:  answer,
			Correct:   isCorrect,
			MatchType: "Exact",
			Score:     float32(score),
		}

		result.AnswerResults = append(result.AnswerResults, answerResult)
		if isCorrect {
			result.Correct = true
			return result, nil
		}
	}

	// Step 2: Partial match
	for _, answer := range testCase.ExpectedAnswers {
		isCorrect = strings.Contains(normalizeText(response.Message), normalizeText(answer))
		score := 0
		if isCorrect {
			score = 1
		}

		answerResult := &EvaluationTestCaseAnswerResult{
			Expected:  answer,
			Correct:   isCorrect,
			MatchType: "Partial",
			Score:     float32(score),
		}

		result.AnswerResults = append(result.AnswerResults, answerResult)
		if isCorrect {
			result.Correct = true
			return result, nil
		}
	}

	// Step 3: Fuzzy matching using Levenshtein distance
	for _, answer := range testCase.ExpectedAnswers {
		similarity := calculateFuzzySimilarity(response.Message, answer)
		isCorrect = similarity >= float64(*options.FuzzyMatchThreshold)
		answerResult := &EvaluationTestCaseAnswerResult{
			Expected:  answer,
			Correct:   isCorrect,
			MatchType: "Fuzzy",
			Score:     float32(similarity),
		}

		result.AnswerResults = append(result.AnswerResults, answerResult)
		if isCorrect {
			result.Correct = true
			return result, nil
		}
	}

	// Step 4: Semantic similarity using embeddings
	for _, answer := range testCase.ExpectedAnswers {
		similarity, err := s.calculateCosineSimilarity(ctx, options.EmbeddingModel, response.Message, answer)
		if err != nil {
			return nil, err
		}
		isCorrect = similarity >= *options.SimilarityMatchThreshold
		answerResult := &EvaluationTestCaseAnswerResult{
			Expected:  answer,
			Correct:   isCorrect,
			MatchType: "Semantic",
			Score:     similarity,
		}

		result.AnswerResults = append(result.AnswerResults, answerResult)
		if isCorrect {
			result.Correct = true
			return result, nil
		}
	}

	return result, nil
}

// calculateCosineSimilarity generates embeddings for both strings
// and returns the cosine similarity score between them.
func (s *EvalService) calculateCosineSimilarity(ctx context.Context, deploymentName string, text1 string, text2 string) (float32, error) {
	// Generate embeddings for text1 and text2 using OpenAI API
	embedding1, err := s.getEmbedding(ctx, deploymentName, text1)
	if err != nil {
		return 0, err
	}
	embedding2, err := s.getEmbedding(ctx, deploymentName, text2)
	if err != nil {
		return 0, err
	}

	// Calculate cosine similarity between the two embeddings
	return cosineSimilarity(embedding1, embedding2), nil
}

// getEmbedding generates an embedding vector for a given text using OpenAI API.
// This is a placeholder and should be replaced with the actual API call.
func (s *EvalService) getEmbedding(ctx context.Context, deploymentName string, text string) ([]float32, error) {
	embeddingResponse, err := s.openAiClient.GetEmbeddings(ctx, azopenai.EmbeddingsOptions{
		Input:          []string{text},
		DeploymentName: &deploymentName,
	}, nil)
	if err != nil {
		return nil, err
	}

	return embeddingResponse.Embeddings.Data[0].Embedding, nil
}

// cosineSimilarity calculates the cosine similarity between two embedding vectors.
func cosineSimilarity(vec1, vec2 []float32) float32 {
	if len(vec1) != len(vec2) {
		return 0
	}

	var dotProduct, magnitudeVec1, magnitudeVec2 float32
	for i := 0; i < len(vec1); i++ {
		dotProduct += vec1[i] * vec2[i]
		magnitudeVec1 += vec1[i] * vec1[i]
		magnitudeVec2 += vec2[i] * vec2[i]
	}

	if magnitudeVec1 == 0 || magnitudeVec2 == 0 {
		return 0
	}
	return dotProduct / (float32(math.Sqrt(float64(magnitudeVec1))) * float32(math.Sqrt(float64(magnitudeVec2))))
}

func normalizeText(text string) string {
	// Normalize text by removing punctuation, converting to lowercase, etc.
	return strings.ToLower(strings.TrimSpace(text))
}

// calculateFuzzySimilarity calculates a similarity score between two strings
// based on Levenshtein distance, normalized by the maximum possible length.
func calculateFuzzySimilarity(s1, s2 string) float64 {
	distance := levenshtein.DistanceForStrings([]rune(s1), []rune(s2), levenshtein.DefaultOptions)
	maxLen := math.Max(float64(len(s1)), float64(len(s2)))
	if maxLen == 0 {
		return 1 // Both strings are empty
	}
	return 1 - float64(distance)/maxLen
}

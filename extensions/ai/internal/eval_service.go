package internal

import (
	"context"
	"fmt"
	"log"
	"math"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/ai/azopenai"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/texttheater/golang-levenshtein/levenshtein"
	"github.com/wbreza/azd-extensions/sdk/ext"
	"github.com/wbreza/azure-sdk-for-go/sdk/data/azsearchindex"
)

type EvalService struct {
	azdContext   *ext.Context
	aiConfig     *AiConfig
	openAiClient *azopenai.Client
	searchClient *azsearchindex.DocumentsClient
}

func NewEvalService(ctx context.Context, azdContext *ext.Context, aiConfig *AiConfig) (*EvalService, error) {
	aiAccount, err := PromptAIServiceAccount(ctx, azdContext, aiConfig)
	if err != nil {
		return nil, err
	}

	credential, err := azdContext.Credential()
	if err != nil {
		return nil, err
	}

	var azClientOptions *azcore.ClientOptions
	azdContext.Invoke(func(azOptions *azcore.ClientOptions) error {
		azClientOptions = azOptions
		return nil
	})

	openAiClient, err := azopenai.NewClient(*aiAccount.Properties.Endpoint, credential, &azopenai.ClientOptions{
		ClientOptions: *azClientOptions,
	})
	if err != nil {
		return nil, err
	}

	searchEndpoint := fmt.Sprintf("https://%s.search.windows.net", aiConfig.Search.Service)
	searchClient, err := azsearchindex.NewDocumentsClient(searchEndpoint, aiConfig.Search.Index, credential, azClientOptions)
	if err != nil {
		return nil, err
	}

	return &EvalService{
		azdContext:   azdContext,
		aiConfig:     aiConfig,
		openAiClient: openAiClient,
		searchClient: searchClient,
	}, nil
}

type EvaluateOptions struct {
	ChatCompletionModel string
	EmbeddingModel      string
}

func (s *EvalService) Evaluate(ctx context.Context, testCase *EvaluationTestCase, options EvaluateOptions) (bool, error) {
	modelResponse, err := s.queryModel(ctx, testCase, options)
	if err != nil {
		return false, err
	}

	return s.evaluateResponse(ctx, modelResponse, options.EmbeddingModel, testCase)
}

func (s *EvalService) queryModel(ctx context.Context, testCase *EvaluationTestCase, options EvaluateOptions) (string, error) {
	embeddingsResponse, err := s.openAiClient.GetEmbeddings(ctx, azopenai.EmbeddingsOptions{
		Input:          []string{testCase.Question},
		DeploymentName: &options.EmbeddingModel,
	}, nil)
	if err != nil {
		return "", err
	}

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
		return "", nil
	}

	chatMessage := testCase.Question

	if len(searchResponse.Results) > 0 {
		contextResults := make([]string, len(searchResponse.Results))

		for i, result := range searchResponse.Results {
			contextResults[i] = fmt.Sprintf("- [%d] %s", i+1, fmt.Sprint(result.AdditionalProperties["chunk"]))
		}

		chatMessage = fmt.Sprintf("Question: \"%s\"\n\nContext:\n%s", testCase.Question, strings.Join(contextResults, "\n\n"))
	}

	log.Printf("Chat question & context: %s\n", chatMessage)

	chatMessages := []azopenai.ChatRequestMessageClassification{
		&azopenai.ChatRequestSystemMessage{
			Content: azopenai.NewChatRequestSystemMessageContent("You are a helpful AI assistant."),
		},
		&azopenai.ChatRequestUserMessage{
			Content: azopenai.NewChatRequestUserMessageContent(chatMessage),
		},
	}

	response, err := s.openAiClient.GetChatCompletions(ctx, azopenai.ChatCompletionsOptions{
		DeploymentName: &options.ChatCompletionModel,
		Messages:       chatMessages,
	}, nil)
	if err != nil {
		return "", err
	}

	chatResponse := *response.ChatCompletions.Choices[0].Message.Content
	log.Printf("Chat response: %s\n", chatResponse)

	return chatResponse, nil
}

// Function to evaluate if model response is correct based on expected answers
func (s *EvalService) evaluateResponse(ctx context.Context, responseText string, deploymentName string, testCase *EvaluationTestCase) (bool, error) {
	// Step 1: Exact match
	for _, answer := range testCase.ExpectedAnswers {
		if normalizeText(responseText) == normalizeText(answer) {
			return true, nil
		}
	}

	// Step 2: Partial match
	for _, answer := range testCase.ExpectedAnswers {
		if strings.Contains(normalizeText(responseText), normalizeText(answer)) {
			return true, nil
		}
	}

	// Step 3: Fuzzy matching using Levenshtein distance
	const fuzzyMatchThreshold = 0.8 // Threshold for fuzzy match similarity (0-1)
	for _, answer := range testCase.ExpectedAnswers {
		similarity := calculateFuzzySimilarity(responseText, answer)
		log.Printf("Fuzzy similarity: %f\n", similarity)
		if similarity >= fuzzyMatchThreshold {
			return true, nil
		}
	}

	// Step 4: Semantic similarity using embeddings
	const similarityThreshold = 0.8 // Set a threshold for cosine similarity (0-1)
	for _, answer := range testCase.ExpectedAnswers {
		similarity, err := s.calculateCosineSimilarity(ctx, deploymentName, responseText, answer)
		if err != nil {
			return false, err
		}
		log.Printf("Cosine similarity: %f\n", similarity)
		if similarity >= similarityThreshold {
			return true, nil
		}
	}

	return false, nil
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

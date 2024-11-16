package internal

type EvaluationType string

const (
	EvaluationTypeModel EvaluationType = "model"
	EvaluationTypeFlow  EvaluationType = "flow"
)

type EvaluationOptions struct {
	EvaluationType           EvaluationType
	ChatCompletionModel      string
	IndexName                string
	EmbeddingModel           string
	FuzzyMatchThreshold      *float32
	SimilarityMatchThreshold *float32
	BatchSize                int
}

type ModelResponse struct {
	Message    string     `json:"message"`
	Duration   int        `json:"duration"`
	TokenUsage TokenUsage `json:"totalUsage"`
}

type TokenUsage struct {
	PromptTokens     int32 `json:"promptTokens"`
	CompletionTokens int32 `json:"completionTokens"`
	TotalTokens      int32 `json:"totalTokens"`
}

type EvaluationTestData struct {
	SimilarityMatchThreshold *float32              `json:"similarityMatchThreshold"`
	FuzzyMatchThreshold      *float32              `json:"fuzzyMatchThreshold"`
	TestCases                []*EvaluationTestCase `json:"testCases"`
}

type EvaluationTestCase struct {
	Id              string                      `json:"id"`
	Question        string                      `json:"question"`
	ExpectedAnswers []string                    `json:"expectedAnswers"`
	Metadata        *EvaluationTestCaseMetadata `json:"metadata,omitempty"`
}

type EvaluationTestCaseMetadata struct {
	Difficulty string   `json:"difficulty,omitempty"`
	Category   string   `json:"category,omitempty"`
	Tags       []string `json:"tags,omitempty"`
}

type EvaluationReport struct {
	Metrics EvaluationMetrics           `json:"metrics"`
	Results []*EvaluationTestCaseResult `json:"results"`
}

type EvaluationMetrics struct {
	Accuracy   float32                  `json:"accuracy"`
	Precision  float32                  `json:"precision"`
	Recall     float32                  `json:"recall"`
	F1         float32                  `json:"f1"`
	Latency    EvaluationLatencyMetrics `json:"latency"`
	TokenUsage TokenUsageMetrics        `json:"tokenUsage"`
}

type EvaluationLatencyMetrics struct {
	TotalDuration int64   `json:"totalDuration"`
	AvgDuration   float64 `json:"avgDuration"`
	MedianLatency int     `json:"medianLatency"`
	MinDuration   int     `json:"minDuration"`
	MaxDuration   int     `json:"maxDuration"`
}

type TokenUsageMetrics struct {
	TotalTokens  int32   `json:"totalTokens"`
	AvgTokens    float32 `json:"avgTokens"`
	MedianTokens int32   `json:"medianTokens"`
	MinTokens    int32   `json:"minTokens"`
	MaxTokens    int32   `json:"maxTokens"`
}

type EvaluationTestCaseResult struct {
	Id            string                            `json:"id"`
	Question      string                            `json:"question"`
	Response      *ModelResponse                    `json:"response"`
	Correct       bool                              `json:"correct"`
	AnswerResults []*EvaluationTestCaseAnswerResult `json:"answerResults"`
}

type EvaluationTestCaseAnswerResult struct {
	Expected  string  `json:"expected"`
	Correct   bool    `json:"correct"`
	MatchType string  `json:"matchType"`
	Score     float32 `json:"score"`
}

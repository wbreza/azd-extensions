package internal

type EmbeddingDocument struct {
	ChunkId    string    `json:"chunk_id"`
	ParentId   string    `json:"parent_id"`
	Chunk      string    `json:"chunk"`
	Title      string    `json:"title"`
	TextVector []float32 `json:"text_vector"`
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

type EvaluationTestData struct {
	TestCases []*EvaluationTestCase `json:"testCases"`
}

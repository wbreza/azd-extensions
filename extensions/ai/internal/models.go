package internal

type EmbeddingDocument struct {
	ChunkId    string    `json:"chunk_id"`
	ParentId   string    `json:"parent_id"`
	Chunk      string    `json:"chunk"`
	Title      string    `json:"title"`
	TextVector []float32 `json:"text_vector"`
}

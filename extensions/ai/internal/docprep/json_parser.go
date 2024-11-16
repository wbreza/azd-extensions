package docprep

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
)

type JsonParser struct {
}

func NewJsonParser() *JsonParser {
	return &JsonParser{}
}

func (p *JsonParser) SuggestSummarization() bool {
	return true
}

func (p *JsonParser) Parse(document *Document) ([]*DocumentChunk, error) {
	rawBytes, err := os.ReadFile(document.Path)
	if err != nil {
		return nil, err
	}

	content := string(rawBytes)
	sourceHash := sha256.Sum256([]byte(document.Path))
	contentHash := sha256.Sum256([]byte(content))

	fileChunk := &DocumentChunk{
		Id:       hex.EncodeToString(sourceHash[:]),
		ParentId: hex.EncodeToString(sourceHash[:]),
		Hash:     hex.EncodeToString(contentHash[:]),
		Content:  content,
		Path:     document.Path,
	}

	return []*DocumentChunk{fileChunk}, nil
}

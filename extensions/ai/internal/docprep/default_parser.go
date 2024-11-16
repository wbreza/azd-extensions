package docprep

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
)

type DefaultParser struct {
}

func NewDefaultParser() *DefaultParser {
	return &DefaultParser{}
}

func (p *DefaultParser) SuggestSummarization() bool {
	return false
}

func (p *DefaultParser) Parse(document *Document) ([]*DocumentChunk, error) {
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

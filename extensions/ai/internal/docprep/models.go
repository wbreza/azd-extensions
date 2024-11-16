package docprep

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/google/uuid"
)

type EmbeddingDocument struct {
	Id       string    `json:"id"`
	ParentId string    `json:"parentId"`
	Summary  string    `json:"summary"`
	Content  string    `json:"content"`
	Path     string    `json:"path"`
	Vector   []float32 `json:"vector"`
}

type DocumentParser interface {
	Parse(document *Document) ([]*DocumentChunk, error)
	SuggestSummarization() bool
}

type Document struct {
	Id   string
	Name string
	Type string
	Path string
	Size int64
}

func ParseDocument(path string) (*Document, error) {
	fileInfo, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	if fileInfo.IsDir() {
		return nil, errors.New("document is a directory")
	}

	return &Document{
		Id:   uuid.NewString(),
		Name: fileInfo.Name(),
		Size: fileInfo.Size(),
		Type: filepath.Ext(path),
		Path: path,
	}, nil
}

type DocumentChunk struct {
	Id       string
	ParentId string
	Hash     string
	Content  string
	Path     string
}

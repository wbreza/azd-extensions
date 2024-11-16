package docprep

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

type MarkdownParser struct {
}

func NewMarkdownParser() *MarkdownParser {
	return &MarkdownParser{}
}

func (p *MarkdownParser) SuggestSummarization() bool {
	return false
}

func (p *MarkdownParser) Parse(document *Document) ([]*DocumentChunk, error) {
	rawBytes, err := os.ReadFile(document.Path)
	if err != nil {
		return nil, err
	}

	// Step 1: Remove code blocks and image links using regex
	content := string(rawBytes)
	content = regexp.MustCompile("(?s)```.*?```").ReplaceAllString(content, "")
	content = regexp.MustCompile(`!\[.*?\]\(.*?\)`).ReplaceAllString(content, "")

	contentBytes := []byte(content)

	// Step 2: Parse Markdown to identify headers and paragraphs
	md := goldmark.New()
	reader := text.NewReader(contentBytes)
	doc := md.Parser().Parse(reader)

	chunks := []*DocumentChunk{}

	completeChunk := func(chunk *DocumentChunk) {
		finalContent := strings.ReplaceAll(chunk.Content, "\n", " ")
		finalContent = strings.ReplaceAll(finalContent, "\r", " ")
		finalContent = strings.ReplaceAll(finalContent, "\t", " ")

		contentHash := sha256.Sum256([]byte(finalContent))
		chunk.Content = finalContent
		chunk.Hash = hex.EncodeToString(contentHash[:])
	}

	var currentChunk *DocumentChunk
	err = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		// If the current chunk is too long, complete it and add to the list
		if currentChunk != nil && len(currentChunk.Content) > 400 {
			completeChunk(currentChunk)
			chunks = append(chunks, currentChunk)
			currentChunk = nil
		}

		// If the current chunk is nil, create a new one
		if currentChunk == nil {
			currentChunk = &DocumentChunk{
				Id:       uuid.NewString(),
				ParentId: document.Id,
				Path:     fmt.Sprintf("%s#%d", document.Path, len(chunks)+1),
			}
		}

		// If the node is a heading, create a new chunk
		if n.Kind() == ast.KindHeading {
			if currentChunk != nil && len(currentChunk.Content) > 0 {
				completeChunk(currentChunk)
				chunks = append(chunks, currentChunk)
			}

			currentChunk = &DocumentChunk{
				Id:       uuid.NewString(),
				ParentId: document.Id,
				Path:     fmt.Sprintf("%s#%d", document.Path, len(chunks)+1),
			}
		}

		if n.Type() == ast.TypeBlock {
			// Add the text content to the current chunk
			text := string(n.Lines().Value(contentBytes))
			currentChunk.Content += fmt.Sprintf("\n%s\n", text)
		}

		return ast.WalkContinue, nil
	})

	if currentChunk != nil && len(currentChunk.Content) > 0 {
		completeChunk(currentChunk)
		chunks = append(chunks, currentChunk)
	}

	if err != nil {
		return nil, err
	}

	return chunks, nil
}

// splitBySentences splits a paragraph into sentences if it's too long.
func splitBySentences(paragraph string) []string {
	const maxLen = 200 // Define a max length for a paragraph
	var sentences []string

	// Basic sentence splitting using period followed by space or end of line
	re := regexp.MustCompile(`[.!?]\s+`)
	parts := re.Split(paragraph, -1)
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if len(part) == 0 {
			continue
		}
		if len(part) > maxLen {
			// Split further if still too long
			sentences = append(sentences, part[:maxLen])
			sentences = append(sentences, part[maxLen:])
		} else {
			sentences = append(sentences, part)
		}
	}
	return sentences
}

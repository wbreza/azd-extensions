package docprep

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_Markdown(t *testing.T) {
	parser := NewMarkdownParser()
	document, err := ParseDocument("D:\\dev\\azure\\azure-sdk-blog\\posts\\2024\\08-06-azd-august-2024.md")
	require.NoError(t, err)

	chunks, err := parser.Parse(document)
	require.NoError(t, err)
	require.Greater(t, len(chunks), 0)
}

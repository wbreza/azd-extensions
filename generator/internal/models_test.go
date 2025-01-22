package internal

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_ExtensionArtifact_Json(t *testing.T) {
	artifact := ExtensionArtifact{
		URL: "https://example.com",
		Checksum: ExtensionChecksum{
			Algorithm: "sha256",
			Value:     "1234567890",
		},
		AdditionalMetadata: map[string]any{
			"entrypoint": "main",
		},
	}

	jsonBytes, err := json.MarshalIndent(artifact, "", "  ")
	require.NotNil(t, jsonBytes)
	require.NoError(t, err)

	var value ExtensionArtifact
	err = json.Unmarshal(jsonBytes, &value)
	require.NoError(t, err)

	require.Equal(t, artifact.URL, value.URL)
	require.Equal(t, artifact.Checksum.Algorithm, value.Checksum.Algorithm)
	require.Equal(t, artifact.Checksum.Value, value.Checksum.Value)
	require.Equal(t, artifact.AdditionalMetadata["entrypoint"], value.AdditionalMetadata["entrypoint"])
}

package docprep

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Azure/azure-sdk-for-go/sdk/ai/azopenai"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/wbreza/azd-extensions/extensions/ai/internal"
	"github.com/wbreza/azd-extensions/sdk/azure/storage"
	"github.com/wbreza/azd-extensions/sdk/common/permissions"
	"github.com/wbreza/azd-extensions/sdk/ext"
	"github.com/wbreza/azure-sdk-for-go/sdk/data/azsearchindex"
)

type DocumentPrepService struct {
	azdContext *ext.Context
	aiConfig   *internal.ExtensionConfig

	cwd            string
	openAiClient   *azopenai.Client
	documentClient *azsearchindex.DocumentsClient
	blobClient     storage.BlobClient
}

func NewDocumentPrepService(ctx context.Context, azdContext *ext.Context, extensionConfig *internal.ExtensionConfig) (*DocumentPrepService, error) {
	var azClientOptions *azcore.ClientOptions

	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	err = azdContext.Invoke(func(azcoreOptions *azcore.ClientOptions) {
		azClientOptions = azcoreOptions
	})
	if err != nil {
		return nil, err
	}

	credential, err := azdContext.Credential()
	if err != nil {
		return nil, err
	}

	openAiClient, err := azopenai.NewClient(extensionConfig.Ai.Endpoint, credential, &azopenai.ClientOptions{
		ClientOptions: *azClientOptions,
	})
	if err != nil {
		return nil, err
	}

	documentClient, err := azsearchindex.NewDocumentsClient(extensionConfig.Search.Endpoint, extensionConfig.Search.Index, credential, azClientOptions)
	if err != nil {
		return nil, err
	}

	azBlobClient, err := azblob.NewClient(extensionConfig.Storage.Endpoint, credential, &azblob.ClientOptions{
		ClientOptions: *azClientOptions,
	})
	if err != nil {
		return nil, err
	}

	storageConfig := storage.AccountConfig{
		AccountName:   extensionConfig.Storage.Account,
		ContainerName: extensionConfig.Storage.Container,
		Endpoint:      extensionConfig.Storage.Endpoint,
	}
	blobClient := storage.NewBlobClient(&storageConfig, azBlobClient)

	return &DocumentPrepService{
		cwd:            cwd,
		azdContext:     azdContext,
		aiConfig:       extensionConfig,
		openAiClient:   openAiClient,
		documentClient: documentClient,
		blobClient:     blobClient,
	}, nil
}

func (d *DocumentPrepService) Upload(ctx context.Context, sourcePath string, targetPath string) error {
	file, err := os.Open(sourcePath)
	if err != nil {
		return err
	}

	defer file.Close()

	err = d.blobClient.Upload(ctx, targetPath, file)
	if err != nil {
		return err
	}

	return nil
}

func (d *DocumentPrepService) createParser(document *Document) (DocumentParser, error) {
	extension := filepath.Ext(document.Type)
	switch extension {
	case ".md":
		return NewMarkdownParser(), nil
	default:
		return NewDefaultParser(), nil
	}
}

func (d *DocumentPrepService) GenerateEmbedding(ctx context.Context, sourcePath string, outputDir string) (string, error) {
	sourceDoc, err := ParseDocument(sourcePath)
	if err != nil {
		return "", err
	}

	parser, err := d.createParser(sourceDoc)
	if err != nil {
		return "", err
	}

	chunks, err := parser.Parse(sourceDoc)
	if err != nil {
		return "", err
	}

	for _, chunk := range chunks {
		if chunk.Content == "" {
			continue
		}

		embeddingText := chunk.Content

		if parser.SuggestSummarization() {
			completionsResponse, err := d.openAiClient.GetChatCompletions(ctx, azopenai.ChatCompletionsOptions{
				Messages: []azopenai.ChatRequestMessageClassification{
					&azopenai.ChatRequestSystemMessage{
						Content: azopenai.NewChatRequestSystemMessageContent("You are helping generate summary embeddings for specified document. Please provide a summary of the document."),
					},
					&azopenai.ChatRequestUserMessage{
						Content: azopenai.NewChatRequestUserMessageContent(chunk.Content),
					},
				},
				DeploymentName: &d.aiConfig.Ai.Models.ChatCompletion,
			}, nil)
			if err != nil {
				return "", err
			}

			embeddingText = *completionsResponse.ChatCompletions.Choices[0].Message.Content
		}

		response, err := d.openAiClient.GetEmbeddings(ctx, azopenai.EmbeddingsOptions{
			Input: []string{
				embeddingText,
			},
			DeploymentName: &d.aiConfig.Ai.Models.Embeddings,
		}, nil)

		if err != nil {
			return "", err
		}

		relativeSourcePath, err := filepath.Rel(d.cwd, sourcePath)
		if err != nil {
			return "", err
		}

		embeddingDoc := EmbeddingDocument{
			Id:       chunk.Id,
			ParentId: chunk.ParentId,
			Path:     relativeSourcePath,
			Content:  chunk.Content,
			Summary:  embeddingText,
			Vector:   response.Embeddings.Data[0].Embedding,
		}

		jsonData, err := json.MarshalIndent(embeddingDoc, "", "  ")
		if err != nil {
			return "", err
		}

		outputFilePath := filepath.Join(outputDir, fmt.Sprintf("%s.json", chunk.Id))
		if err := os.WriteFile(outputFilePath, jsonData, permissions.PermissionFile); err != nil {
			return "", err
		}
	}

	return outputDir, nil
}

func (d *DocumentPrepService) IngestEmbedding(ctx context.Context, sourcePath string) error {
	jsonBytes, err := os.ReadFile(sourcePath)
	if err != nil {
		return err
	}

	embeddingDoc := map[string]any{}
	if err := json.Unmarshal(jsonBytes, &embeddingDoc); err != nil {
		return err
	}

	batch := azsearchindex.IndexBatch{
		Actions: []*azsearchindex.IndexAction{
			{
				ActionType:           to.Ptr(azsearchindex.IndexActionTypeMergeOrUpload),
				AdditionalProperties: embeddingDoc,
			},
		},
	}

	indexResponse, err := d.documentClient.Index(ctx, batch, nil, nil)
	if err != nil {
		return err
	}

	if len(indexResponse.Results) == 0 {
		return errors.New("no results returned from index operation")
	}

	return nil
}

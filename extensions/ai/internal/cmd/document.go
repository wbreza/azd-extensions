package cmd

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/wbreza/azd-extensions/extensions/ai/internal"
	"github.com/wbreza/azd-extensions/sdk/azure/storage"
	"github.com/wbreza/azd-extensions/sdk/ext"
	"github.com/wbreza/azd-extensions/sdk/ux"
)

// Flag structs for the azd ai document commands
type UploadFlags struct {
	Source    string
	Container string
	Pattern   string
}

type ListFlags struct {
	Container string
	Prefix    string
}

type DeleteFlags struct {
	Container string
	File      string
	Recursive bool
}

// Command to initialize `azd ai document` command group
func newDocumentCommand() *cobra.Command {
	// Main `document` command
	documentCmd := &cobra.Command{
		Use:   "document",
		Short: "Manage documents in Azure Blob Storage",
	}

	// Add subcommands to the `document` command
	documentCmd.AddCommand(newUploadCommand())

	return documentCmd
}

// Subcommand `upload` for uploading documents
func newUploadCommand() *cobra.Command {
	flags := &UploadFlags{}
	uploadCmd := &cobra.Command{
		Use:   "upload",
		Short: "Upload documents to Azure Blob Storage",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			azdContext, err := ext.CurrentContext(ctx)
			if err != nil {
				return err
			}

			credential, err := azdContext.Credential()
			if err != nil {
				return err
			}

			aiConfig, err := internal.LoadOrPromptAiConfig(ctx, azdContext)
			if err != nil {
				return err
			}

			if err != nil {
				return err
			}

			if aiConfig.StorageAccount == "" {
				storageAccount, err := internal.PromptStorage(ctx, azdContext, aiConfig)
				if err != nil {
					return err
				}

				aiConfig.StorageAccount = *storageAccount.Name
			}

			if flags.Container != "" {
				aiConfig.StorageContainer = flags.Container
			}

			if aiConfig.StorageContainer == "" {
				container, err := internal.PromptStorageContainer(ctx, azdContext, aiConfig)
				if err != nil {
					return err
				}

				aiConfig.StorageContainer = *container.Name
			}

			if err := internal.SaveAiConfig(ctx, azdContext, aiConfig); err != nil {
				return err
			}

			storageConfig := &storage.AccountConfig{
				AccountName:   aiConfig.StorageAccount,
				ContainerName: aiConfig.StorageContainer,
			}

			err = azdContext.Invoke(func(clientOptions *azcore.ClientOptions) error {
				cwd, err := os.Getwd()
				if err != nil {
					log.Fatalf("Error getting current working directory: %v", err)
				}

				// Combine the current working directory with the relative path
				absolutePath := filepath.Join(cwd, flags.Source)

				matchingFiles, err := getMatchingFiles(absolutePath, flags.Pattern, true)
				if err != nil {
					return err
				}

				if len(matchingFiles) == 0 {
					return fmt.Errorf("no files found matching the pattern '%s'", flags.Pattern)
				}

				blobClientOptions := &azblob.ClientOptions{
					ClientOptions: *clientOptions,
				}

				serviceUrl := fmt.Sprintf("https://%s.blob.core.windows.net", aiConfig.StorageAccount)
				azBlobClient, err := azblob.NewClient(serviceUrl, credential, blobClientOptions)
				if err != nil {
					return err
				}

				blobService := storage.NewBlobClient(storageConfig, azBlobClient)
				taskList := ux.NewTaskList(nil)

				// Walk through the directory and upload each file
				for _, file := range matchingFiles {
					relativePath, err := filepath.Rel(cwd, file)
					if err != nil {
						return err
					}

					taskList.AddTask(ux.TaskOptions{
						Title: fmt.Sprintf("Uploading document %s", color.CyanString(relativePath)),
						Async: true,
						Action: func() (ux.TaskState, error) {
							file, err := os.Open(file)
							if err != nil {
								return ux.Error, err
							}

							defer file.Close()

							err = blobService.Upload(ctx, relativePath, file)
							if err != nil {
								return ux.Error, err
							}

							return ux.Success, nil
						},
					})
				}

				if err := taskList.Run(); err != nil {
					return err
				}

				return nil
			})

			if err != nil {
				return err
			}

			return nil
		},
	}

	// Define flags for `upload` command
	uploadCmd.Flags().StringVar(&flags.Source, "source", "", "Path to the local file or directory to upload (required)")
	uploadCmd.Flags().StringVar(&flags.Container, "container", "", "Azure Blob Storage container name to upload to (required)")
	uploadCmd.Flags().StringVarP(&flags.Pattern, "pattern", "p", "", "Specify file type pattern to upload (e.g., '.pdf', '.txt')")

	return uploadCmd
}

func getMatchingFiles(root string, pattern string, recursive bool) ([]string, error) {
	var matchingFiles []string

	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory %s: %v", root, err)
	}

	for _, entry := range entries {
		fullPath := filepath.Join(root, entry.Name())

		// If the entry is a directory and recursion is enabled, process subdirectory
		if entry.IsDir() {
			if recursive {
				subFiles, err := getMatchingFiles(fullPath, pattern, recursive)
				if err != nil {
					return nil, err
				}
				matchingFiles = append(matchingFiles, subFiles...)
			}
			continue
		}

		if pattern == "" {
			matchingFiles = append(matchingFiles, fullPath)
			continue
		}

		// Check if the file matches the specified pattern
		matched, err := filepath.Match(pattern, entry.Name())
		if err != nil {
			return nil, fmt.Errorf("error matching pattern: %v", err)
		}
		if matched {
			matchingFiles = append(matchingFiles, fullPath)
		}
	}
	return matchingFiles, nil
}

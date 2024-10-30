package cmd

import (
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/spf13/cobra"
	"github.com/wbreza/azd-extensions/extensions/ai/internal"
	"github.com/wbreza/azd-extensions/sdk/azure/storage"
	"github.com/wbreza/azd-extensions/sdk/ext"
)

// Flag structs for the azd ai document commands
type UploadFlags struct {
	Source    string
	Container string
	Overwrite bool
	Recursive bool
	FileType  string
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
	documentCmd.AddCommand(newListCommand())
	documentCmd.AddCommand(newDeleteCommand())

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

			if flags.Container == "" {
				container, err := internal.PromptStorageContainer(ctx, azdContext, aiConfig)
				if err != nil {
					return err
				}

				flags.Container = *container.Name
			}

			storageConfig := &storage.AccountConfig{
				AccountName:   aiConfig.StorageAccount,
				ContainerName: flags.Container,
			}

			serviceUrl := fmt.Sprintf("https://%s.blob.core.windows.net", aiConfig.StorageAccount)
			azBlobClient, err := azblob.NewClient(serviceUrl, credential, nil)
			if err != nil {
				return err
			}

			blobService := storage.NewBlobClient(storageConfig, azBlobClient)
			blobs, err := blobService.Items(ctx)
			if err != nil {
				return err
			}

			fmt.Print(blobs)

			return nil
		},
	}

	// Define flags for `upload` command
	uploadCmd.Flags().StringVar(&flags.Source, "source", "", "Path to the local file or directory to upload (required)")
	uploadCmd.Flags().StringVar(&flags.Container, "container", "", "Azure Blob Storage container name to upload to (required)")
	uploadCmd.Flags().BoolVar(&flags.Overwrite, "overwrite", false, "Overwrite existing documents in the container")
	uploadCmd.Flags().BoolVar(&flags.Recursive, "recursive", false, "Upload all files in subdirectories if source is a directory")
	uploadCmd.Flags().StringVar(&flags.FileType, "file-type", "", "Specify file types to upload (e.g., '.pdf', '.txt')")

	return uploadCmd
}

// Subcommand `list` for listing documents
func newListCommand() *cobra.Command {
	flags := &ListFlags{}
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List documents in a specified container",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Stub for listing documents
			fmt.Printf("Listing documents in container %s\n", flags.Container)
			return nil
		},
	}

	// Define flags for `list` command
	listCmd.Flags().StringVar(&flags.Container, "container", "", "Name of the container to list documents (required)")
	listCmd.Flags().StringVar(&flags.Prefix, "prefix", "", "Filter documents by a prefix (e.g., 'reports/')")

	_ = listCmd.MarkFlagRequired("container")

	return listCmd
}

// Subcommand `delete` for deleting documents
func newDeleteCommand() *cobra.Command {
	flags := &DeleteFlags{}
	deleteCmd := &cobra.Command{
		Use:   "delete",
		Short: "Remove documents from storage",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Stub for deleting documents
			fmt.Printf("Deleting documents in container %s\n", flags.Container)
			return nil
		},
	}

	// Define flags for `delete` command
	deleteCmd.Flags().StringVar(&flags.Container, "container", "", "Container name where the documents are stored (required)")
	deleteCmd.Flags().StringVar(&flags.File, "file", "", "Specific file or pattern to delete (e.g., 'doc1.txt' or 'reports/*')")
	deleteCmd.Flags().BoolVar(&flags.Recursive, "recursive", false, "Delete all files within a directory or prefix")

	_ = deleteCmd.MarkFlagRequired("container")

	return deleteCmd
}

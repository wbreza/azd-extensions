package cmd

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/wbreza/azd-extensions/extensions/ai/internal"
	"github.com/wbreza/azd-extensions/extensions/ai/internal/docprep"
	"github.com/wbreza/azd-extensions/sdk/common"
	"github.com/wbreza/azd-extensions/sdk/ext"
	"github.com/wbreza/azd-extensions/sdk/ext/output"
	"github.com/wbreza/azd-extensions/sdk/ux"
)

// Flag structs for the azd ai document commands
type UploadFlags struct {
	Source        string
	Force         bool
	AccountName   string
	ContainerName string
	Pattern       string
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
			header := output.CommandHeader{
				Title:       "Upload documents to storage (azd ai document upload)",
				Description: "Upload a set of documents to Azure Blob Storage.",
			}
			header.Print()

			ctx := cmd.Context()

			azdContext, err := ext.CurrentContext(ctx)
			if err != nil {
				return err
			}

			azureContext, err := azdContext.AzureContext(ctx)
			if err != nil {
				return err
			}

			extensionConfig, err := internal.LoadExtensionConfig(ctx, azdContext)
			if err != nil {
				aiAccount, err := internal.PromptAIServiceAccount(ctx, azdContext, azureContext)
				if err != nil {
					return err
				}

				extensionConfig = &internal.ExtensionConfig{
					Ai: internal.AiConfig{
						Service:  *aiAccount.Name,
						Endpoint: *aiAccount.Properties.Endpoint,
					},
				}
			}

			if flags.Pattern == "" {
				flags.Pattern = "*"
			}

			if flags.AccountName != "" {
				extensionConfig.Storage.Account = flags.AccountName
			}

			if flags.ContainerName != "" {
				extensionConfig.Storage.Container = flags.ContainerName
			}

			if extensionConfig.Storage.Account == "" {
				storageAccount, err := internal.PromptStorageAccount(ctx, azdContext, azureContext)
				if err != nil {
					return err
				}

				extensionConfig.Storage.Account = *storageAccount.Name
				extensionConfig.Storage.Endpoint = *storageAccount.Properties.PrimaryEndpoints.Blob
			}

			if flags.ContainerName != "" {
				extensionConfig.Storage.Container = flags.ContainerName
			}

			if extensionConfig.Storage.Container == "" {
				container, err := internal.PromptStorageContainer(ctx, azdContext, azureContext)
				if err != nil {
					return err
				}

				extensionConfig.Storage.Container = *container.Name
			}

			if err := internal.SaveExtensionConfig(ctx, azdContext, extensionConfig); err != nil {
				return err
			}

			cwd, err := os.Getwd()
			if err != nil {
				log.Fatalf("Error getting current working directory: %v", err)
			}

			// Combine the current working directory with the relative path
			absSourcePath := filepath.Join(cwd, flags.Source)
			matchingFiles, err := getMatchingFiles(absSourcePath, flags.Pattern, true)
			if err != nil {
				return err
			}

			if len(matchingFiles) == 0 {
				return fmt.Errorf("no files found matching the pattern '%s'", flags.Pattern)
			}

			fmt.Printf("Source Data: %s\n", color.CyanString(absSourcePath))
			fmt.Printf("Storage Account: %s\n", color.CyanString(extensionConfig.Storage.Account))
			fmt.Printf("Storage Container: %s\n", color.CyanString(extensionConfig.Storage.Container))

			if !flags.Force {
				fmt.Println()
				fmt.Printf("Found %s matching files with pattern %s.\n", color.CyanString(fmt.Sprint(len(matchingFiles))), color.CyanString(flags.Pattern))

				continueConfirm := ux.NewConfirm(&ux.ConfirmOptions{
					DefaultValue: ux.Ptr(true),
					Message:      "Continue uploading documents?",
				})

				userConfirmed, err := continueConfirm.Ask()
				if err != nil {
					return err
				}

				if !*userConfirmed {
					return ux.ErrCancelled
				}
			}

			taskList := ux.NewTaskList(nil)

			docPrepService, err := docprep.NewDocumentPrepService(ctx, azdContext, extensionConfig)
			if err != nil {
				return err
			}

			// Walk through the directory and upload each file
			for _, file := range matchingFiles {
				relativePath, err := filepath.Rel(cwd, file)
				if err != nil {
					return err
				}

				taskList.AddTask(ux.TaskOptions{
					Title: fmt.Sprintf("Uploading document %s", color.CyanString(relativePath)),
					Async: true,
					Action: func(setProgress ux.SetProgressFunc) (ux.TaskState, error) {
						if err := docPrepService.Upload(ctx, file, relativePath); err != nil {
							return ux.Error, common.NewDetailedError("Failed to upload document", err)
						}

						return ux.Success, nil
					},
				})
			}

			if err := taskList.Run(); err != nil {
				return err
			}

			return nil
		},
	}

	// Define flags for `upload` command
	uploadCmd.Flags().StringVar(&flags.Source, "source", "", "Path to the local file or directory to upload (required)")
	uploadCmd.Flags().StringVar(&flags.AccountName, "account", "", "Azure Blob Storage account name to upload to")
	uploadCmd.Flags().StringVar(&flags.ContainerName, "container", "", "Azure Blob Storage container name to upload to")
	uploadCmd.Flags().StringVarP(&flags.Pattern, "pattern", "p", "", "Specify file type pattern to upload (e.g., '*.pdf', '*.txt')")
	uploadCmd.Flags().BoolVarP(&flags.Force, "force", "f", false, "Upload without confirmation")

	_ = uploadCmd.MarkFlagRequired("source")

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

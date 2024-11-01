package cmd

import (
	"github.com/spf13/cobra"
	"github.com/wbreza/azd-extensions/extensions/ai/internal"
	"github.com/wbreza/azd-extensions/sdk/ext"
	"github.com/wbreza/azd-extensions/sdk/ext/output"
)

// Flag structs for the azd ai index commands
type CreateIndexFlags struct {
	Name               string
	IndexType          string
	EmbeddingDimension int
	Metric             string
	StorageClass       string
	Replicas           int
	PartitionCount     int
}

// Command to initialize `azd ai index` command group
func newIndexCommand() *cobra.Command {
	// Main `index` command
	indexCmd := &cobra.Command{
		Use:   "index",
		Short: "Manage indexes for vector search, including creation, listing, deletion, and updating",
	}

	// Add subcommands to the `index` command
	indexCmd.AddCommand(newCreateIndexCommand())

	return indexCmd
}

// Subcommand `create` for creating a new index
func newCreateIndexCommand() *cobra.Command {
	flags := &CreateIndexFlags{}
	createCmd := &cobra.Command{
		Use:   "create",
		Short: "Define a new index structure for vector search",
		RunE: func(cmd *cobra.Command, args []string) error {
			header := output.CommandHeader{
				Title:       "Create an Azure search index (azd ai index create)",
				Description: "Creates an Azure search index for vector search",
			}
			header.Print()

			ctx := cmd.Context()

			azdContext, err := ext.CurrentContext(ctx)
			if err != nil {
				return err
			}

			aiConfig, err := internal.LoadOrPromptAiConfig(ctx, azdContext)
			if err != nil {
				return err
			}

			if aiConfig.Search.Service == "" {
				searchService, err := internal.PromptSearchService(ctx, azdContext, aiConfig)
				if err != nil {
					return err
				}

				aiConfig.Search.Service = *searchService.Name
			}

			if aiConfig.Search.Index == "" {
				searchIndex, err := internal.PromptSearchIndex(ctx, azdContext, aiConfig)
				if err != nil {
					return err
				}

				aiConfig.Search.Index = *searchIndex.Name
			}

			if err := internal.SaveAiConfig(ctx, azdContext, aiConfig); err != nil {
				return err
			}

			return nil
		},
	}

	// Define flags for `create` command
	createCmd.Flags().StringVar(&flags.Name, "name", "", "Name of the new index (required)")
	createCmd.Flags().IntVar(&flags.EmbeddingDimension, "embedding-dimension", 0, "Number of dimensions for embeddings")
	createCmd.Flags().StringVar(&flags.Metric, "metric", "", "Similarity metric (e.g., 'cosine', 'euclidean')")
	createCmd.Flags().StringVar(&flags.StorageClass, "storage-class", "", "Storage class based on access frequency (e.g., 'hot', 'cool', 'archive')")
	createCmd.Flags().IntVar(&flags.Replicas, "replicas", 1, "Number of replicas for high availability")
	createCmd.Flags().IntVar(&flags.PartitionCount, "partition-count", 1, "Set partition count for sharding")

	return createCmd
}
package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/fatih/color"
	"github.com/wbreza/azd-extensions/extensions/ai/internal/cmd"
	"github.com/wbreza/azd-extensions/sdk/ext"
)

func main() {
	// Execute the root command
	ctx := context.Background()
	rootCmd := cmd.NewRootCommand()

	if err := rootCmd.ExecuteContext(ctx); err != nil {
		var errWithSuggestion *ext.ErrorWithSuggestion

		if ok := errors.As(err, &errWithSuggestion); ok {
			color.Red("Error: %v", errWithSuggestion.Err)
			fmt.Printf("%s: %s\n", color.YellowString("Suggestion:"), errWithSuggestion.Suggestion)
		} else {
			color.Red("Error: %v", err)
		}

		os.Exit(1)
	}
}

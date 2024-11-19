package main

import (
	"context"
	"fmt"
	"os"

	"github.com/wbreza/azd-extensions/generator/internal/cmd"
)

func main() {
	ctx := context.Background()
	rootCmd := cmd.NewRootCommand()
	if err := rootCmd.ExecuteContext(ctx); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

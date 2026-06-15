// Package main is the Redoubt entry point.
// Foundation owns this file — it wires config, logging, and encryption then
// attaches the sub-commands registered by each stream.
package main

import (
	"fmt"
	"os"

	"github.com/kevinthelago/redoubt/internal/assets"
	"github.com/kevinthelago/redoubt/internal/cli"
	"github.com/spf13/cobra"
)

func main() {
	// TODO(Foundation): replace with proper config/logging/age setup.
	dataDir, _ := os.UserConfigDir()
	dataDir = dataDir + "/redoubt"

	store := assets.NewStore(dataDir, assets.NopLogger())
	if err := store.Load(); err != nil {
		fmt.Fprintf(os.Stderr, "error loading asset store: %v\n", err)
		os.Exit(1)
	}

	root := &cobra.Command{
		Use:   "redoubt",
		Short: "Personal encrypted backup tool",
	}
	root.AddCommand(cli.NewTrackCmd(store, nil, nil))

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

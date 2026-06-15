// Package main is the redoubt binary entry point.
// This stub is owned by the foundation stream (F1); it is here only so that
// internal/cli packages referencing cobra can be compiled and tested.
// Foundation should replace this with the full Cobra root command.
package main

import (
	"fmt"
	"os"

	"github.com/kevinthelago/redoubt/internal/cli"
	"github.com/spf13/cobra"
)

func main() {
	root := &cobra.Command{
		Use:   "redoubt",
		Short: "LAN-only backup & recovery for your development stack",
	}
	root.AddCommand(cli.NewStatusCmd())

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

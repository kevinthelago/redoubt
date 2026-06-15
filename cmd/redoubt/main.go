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
		CompletionOptions: cobra.CompletionOptions{
			DisableDefaultCmd: true,
		},
	}

	root.AddCommand(cli.NewDrillCmd())

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

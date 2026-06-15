package main

import (
	"os"

	"github.com/kevinthelago/redoubt/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}

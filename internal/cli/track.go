// Package cli assembles the Redoubt command-line interface.
package cli

import (
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/kevinthelago/redoubt/internal/assets"
	"github.com/spf13/cobra"
)

// NewTrackCmd returns the `redoubt track` command with its subcommands.
// store must already be loaded (call store.Load before passing it in).
// scanner and ex may be nil; defaults are used when nil.
func NewTrackCmd(store *assets.Store, scanner *assets.Scanner, ex *assets.Excluder) *cobra.Command {
	if scanner == nil {
		scanner = assets.DefaultScanner()
	}
	if ex == nil {
		ex = assets.NewExcluder(nil)
	}

	cmd := &cobra.Command{
		Use:   "track",
		Short: "Manage the set of tracked assets",
		Long: `Manage the set of filesystem paths that Redoubt backs up.

Each asset is tagged with a category:
  source    — source code and project files
  secrets   — credentials, keys, and sensitive configuration
  database  — databases (requires a validated dump command)
  config    — application and system configuration files`,
	}
	cmd.AddCommand(
		newTrackAddCmd(store),
		newTrackListCmd(store),
		newTrackRemoveCmd(store),
		newTrackScanCmd(store, scanner, ex),
	)
	return cmd
}

func newTrackAddCmd(store *assets.Store) *cobra.Command {
	var catStr string
	var dumpCmd string

	presetNames := make([]string, len(assets.DBPresets))
	for i, p := range assets.DBPresets {
		presetNames[i] = p.Name
	}

	cmd := &cobra.Command{
		Use:   "add <path>",
		Short: "Add a path to the tracked asset set",
		Args:  cobra.ExactArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			// Expand a preset name to its full command template before validation.
			if assets.Category(catStr) == assets.CategoryDatabase {
				if tmpl, ok := assets.PresetCommand(dumpCmd); ok {
					dumpCmd = tmpl
				}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			cat := assets.Category(catStr)
			if err := store.Add(args[0], cat, dumpCmd); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "added %s (%s)\n", args[0], cat)
			return nil
		},
	}

	cmd.Flags().StringVarP(&catStr, "type", "t", "",
		"asset category: source, secrets, database, config (required)")
	cmd.Flags().StringVarP(&dumpCmd, "dump-command", "d", "",
		fmt.Sprintf("database dump command or preset name (%s)", strings.Join(presetNames, ", ")))
	_ = cmd.MarkFlagRequired("type")

	return cmd
}

func newTrackListCmd(store *assets.Store) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all tracked assets",
		RunE: func(cmd *cobra.Command, _ []string) error {
			items := store.List()
			if len(items) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "no assets tracked — add one with: redoubt track add <path> --type <category>")
				return nil
			}

			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "CATEGORY\tPATH\tSIZE\tLAST BACKUP")
			for _, a := range items {
				size := assets.FormatSize(assets.DiskUsage(a.Path))
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
					a.Category, a.Path, size, "never")
			}
			return w.Flush()
		},
	}
}

func newTrackRemoveCmd(store *assets.Store) *cobra.Command {
	return &cobra.Command{
		Use:   "remove <path>",
		Short: "Remove a path from the tracked asset set",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := store.Remove(args[0]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "removed %s\n", args[0])
			return nil
		},
	}
}

func newTrackScanCmd(store *assets.Store, scanner *assets.Scanner, ex *assets.Excluder) *cobra.Command {
	return &cobra.Command{
		Use:   "scan",
		Short: "Scan non-secret assets for likely secrets and suggest re-tagging",
		Long: `Scan non-secret assets using a gitleaks-compatible ruleset.

Matched values are never printed. Only the file, line number, and rule
description are reported. Assets tagged "secrets" are skipped.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			items := store.List()

			results, err := scanner.ScanAssets(items, ex)
			if err != nil {
				return err
			}

			if len(results) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "no likely secrets found in non-secret assets")
				return nil
			}

			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "FILE\tLINE\tRULE\tDESCRIPTION")
			for _, r := range results {
				fmt.Fprintf(w, "%s\t%d\t%s\t%s\n",
					r.File, r.Line, r.RuleID, r.Description)
			}
			if err := w.Flush(); err != nil {
				return err
			}

			// Collect affected asset paths for the re-tagging suggestion.
			affected := make(map[string]bool)
			for _, r := range results {
				affected[r.AssetPath] = true
			}
			fmt.Fprintln(cmd.ErrOrStderr())
			fmt.Fprintf(cmd.ErrOrStderr(), "found likely secrets in %d asset(s). Consider re-tagging:\n", len(affected))
			for p := range affected {
				fmt.Fprintf(cmd.ErrOrStderr(), "  redoubt track remove %s\n", p)
				fmt.Fprintf(cmd.ErrOrStderr(), "  redoubt track add    %s --type secrets\n", p)
			}

			return nil
		},
	}
}

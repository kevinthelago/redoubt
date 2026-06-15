package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/kevinthelago/redoubt/internal/cold"
	"github.com/kevinthelago/redoubt/internal/config"
	"github.com/kevinthelago/redoubt/internal/restic"
	"github.com/kevinthelago/redoubt/internal/snapshots"
	"github.com/spf13/cobra"
)

// NewSnapshotsCmd returns the "redoubt snapshots" command tree.
// Register it in root.go: replace the stub snapshotsCmd with NewSnapshotsCmd().
//
// TODO(integration): wire config (F2) and keystore (K1) once those streams
// land; replace the factory body below with real credential resolution.
func NewSnapshotsCmd() *cobra.Command {
	newClient := buildClientFactory()
	return buildSnapshotsCmd(newClient, os.Stdout)
}

// buildClientFactory returns a function that constructs a snapshots.Client
// for the given source ("vault" or "cold").
//
// Password resolution: REDOUBT_REPO_PASSWORD env var is the current fallback.
// TODO(K1): replace os.Getenv with keystore.RepoPassword() once K1 lands.
func buildClientFactory() func(source string) (snapshots.Client, error) {
	return func(source string) (snapshots.Client, error) {
		cfg, err := config.Load()
		if err != nil {
			return nil, fmt.Errorf("load config: %w", err)
		}

		// TODO(K1): replace with keystore.RepoPassword() once K1 lands on develop.
		password := os.Getenv("REDOUBT_REPO_PASSWORD")
		if password == "" {
			return nil, fmt.Errorf("REDOUBT_REPO_PASSWORD is not set (keystore integration pending K1)")
		}

		backend, err := resticBackendForSource(source, cfg)
		if err != nil {
			return nil, err
		}
		return snapshots.NewResticAdapter(restic.New(backend, restic.WithPassword(password))), nil
	}
}

// resticBackendForSource maps the --source flag value to a restic.Backend.
// "vault" uses the REST backend from the user's config profile.
// "cold" finds the best-candidate mounted cold drive from the drive registry.
func resticBackendForSource(source string, cfg *config.Config) (restic.Backend, error) {
	switch source {
	case "vault":
		return restic.Backend{
			Kind:   restic.BackendREST,
			URL:    cfg.Vault.URL,
			CACert: cfg.Vault.CACert,
		}, nil
	case "cold":
		regPath := filepath.Join(config.DataDir(), "cold-registry.toml")
		reg, err := cold.LoadRegistry(regPath)
		if err != nil {
			return restic.Backend{}, fmt.Errorf("load cold drive registry: %w", err)
		}
		drive, ok := reg.BestCandidate()
		if !ok {
			return restic.Backend{}, fmt.Errorf("no cold drive is currently mounted")
		}
		return restic.Backend{
			Kind: restic.BackendLocal,
			Path: drive.RepoPath(),
		}, nil
	default:
		return restic.Backend{}, fmt.Errorf("unknown source %q: use \"vault\" or \"cold\"", source)
	}
}

// buildSnapshotsCmd is the testable core; callers inject the client factory and writer.
func buildSnapshotsCmd(newClient func(source string) (snapshots.Client, error), out io.Writer) *cobra.Command {
	var (
		source   string
		jsonMode bool
	)

	root := &cobra.Command{
		Use:   "snapshots",
		Short: "Browse restic snapshots (read-only)",
		Long: `Browse restic snapshots stored in the vault or on a cold drive.

All operations are strictly read-only.  Secret entries (age-sealed files)
are shown as sealed filenames only — their contents are never revealed here.`,
	}
	root.PersistentFlags().StringVar(&source, "source", "vault", `snapshot source: "vault" (default) or "cold"`)
	root.PersistentFlags().BoolVar(&jsonMode, "json", false, "output results as JSON")

	root.AddCommand(
		buildListCmd(&source, &jsonMode, newClient, out),
		buildLSCmd(&source, &jsonMode, newClient, out),
		buildFindCmd(&source, &jsonMode, newClient, out),
		buildDiffCmd(&source, &jsonMode, newClient, out),
	)

	return root
}

// --- list ---

func buildListCmd(source *string, jsonMode *bool, newClient func(string) (snapshots.Client, error), out io.Writer) *cobra.Command {
	var (
		tags     []string
		hostname string
		after    string
		before   string
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List snapshots",
		Long:  "List snapshots, optionally filtered by category tag, hostname, or date range.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			filter, err := buildFilter(tags, hostname, after, before)
			if err != nil {
				return err
			}
			client, err := newClient(*source)
			if err != nil {
				return err
			}
			browser := snapshots.New(client)
			snaps, err := browser.List(cmd.Context(), filter)
			if err != nil {
				return err
			}
			if *jsonMode {
				return writeJSON(out, snaps)
			}
			return printSnapshotList(out, snaps)
		},
	}
	cmd.Flags().StringArrayVar(&tags, "tag", nil, "filter by category tag (repeatable; AND-matched)")
	cmd.Flags().StringVar(&hostname, "host", "", "filter by hostname")
	cmd.Flags().StringVar(&after, "after", "", "show snapshots taken after this time (RFC3339 or YYYY-MM-DD)")
	cmd.Flags().StringVar(&before, "before", "", "show snapshots taken before this time (RFC3339 or YYYY-MM-DD)")
	return cmd
}

// --- ls ---

func buildLSCmd(source *string, jsonMode *bool, newClient func(string) (snapshots.Client, error), out io.Writer) *cobra.Command {
	var path string

	cmd := &cobra.Command{
		Use:   "ls <snapshot-id>",
		Short: "List files in a snapshot",
		Long: `List the file tree within a snapshot.

Secret entries (age-sealed files) are displayed as sealed filenames only.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			snapshotID := args[0]
			client, err := newClient(*source)
			if err != nil {
				return err
			}

			// Determine whether we need to look up the snapshot's tags to
			// identify secrets snapshots.  We do a quick List with ID filter
			// to get the metadata.
			browser := snapshots.New(client)
			all, err := browser.List(cmd.Context(), snapshots.SnapshotFilter{})
			if err != nil {
				return err
			}
			isSecretSnap := false
			for _, s := range all {
				if strings.HasPrefix(s.ID, snapshotID) || s.ShortID == snapshotID {
					isSecretSnap = s.IsSecretSnapshot()
					break
				}
			}

			var nodes []snapshots.TreeNode
			if isSecretSnap {
				nodes, err = browser.LSSealed(cmd.Context(), snapshotID, path)
			} else {
				nodes, err = browser.LS(cmd.Context(), snapshotID, path)
			}
			if err != nil {
				return err
			}
			if *jsonMode {
				return writeJSON(out, nodes)
			}
			return printFileTree(out, nodes)
		},
	}
	cmd.Flags().StringVar(&path, "path", "", "list only files under this path prefix")
	return cmd
}

// --- find ---

func buildFindCmd(source *string, jsonMode *bool, newClient func(string) (snapshots.Client, error), out io.Writer) *cobra.Command {
	var (
		tags     []string
		hostname string
		after    string
		before   string
	)

	cmd := &cobra.Command{
		Use:   "find <path>",
		Short: "Find a path across snapshots",
		Long: `Find files matching a path pattern across all snapshots.

The path may be an exact path or a glob pattern.  Filters restrict which
snapshots are searched.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pattern := args[0]
			filter, err := buildFilter(tags, hostname, after, before)
			if err != nil {
				return err
			}
			client, err := newClient(*source)
			if err != nil {
				return err
			}
			browser := snapshots.New(client)
			results, err := browser.Find(cmd.Context(), pattern, filter)
			if err != nil {
				return err
			}
			if *jsonMode {
				return writeJSON(out, results)
			}
			return printFindResults(out, results)
		},
	}
	cmd.Flags().StringArrayVar(&tags, "tag", nil, "restrict search to snapshots with this tag (repeatable; AND-matched)")
	cmd.Flags().StringVar(&hostname, "host", "", "restrict search to snapshots from this hostname")
	cmd.Flags().StringVar(&after, "after", "", "restrict search to snapshots taken after this time")
	cmd.Flags().StringVar(&before, "before", "", "restrict search to snapshots taken before this time")
	return cmd
}

// --- diff ---

func buildDiffCmd(source *string, jsonMode *bool, newClient func(string) (snapshots.Client, error), out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "diff <snapshot-a> <snapshot-b>",
		Short: "Show differences between two snapshots",
		Long: `Compare two snapshots and list the paths that changed.

Secret entries (age-sealed files) are shown as sealed filenames only.`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := newClient(*source)
			if err != nil {
				return err
			}
			browser := snapshots.New(client)
			entries, err := browser.Diff(cmd.Context(), args[0], args[1])
			if err != nil {
				return err
			}
			if *jsonMode {
				return writeJSON(out, entries)
			}
			return printDiff(out, entries)
		},
	}
	return cmd
}

// --- output helpers ---

func printSnapshotList(out io.Writer, snaps []snapshots.Snapshot) error {
	if len(snaps) == 0 {
		fmt.Fprintln(out, "No snapshots found. Run `redoubt backup` to create the first one.")
		return nil
	}
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tTIME\tHOST\tTAGS\tSIZE")
	for _, s := range snaps {
		tags := strings.Join(s.Tags, ",")
		if tags == "" {
			tags = "-"
		}
		size := formatBytes(s.BytesAdded)
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			s.ShortID,
			s.Time.Format(time.RFC3339),
			s.Hostname,
			tags,
			size,
		)
	}
	return w.Flush()
}

func printFileTree(out io.Writer, nodes []snapshots.TreeNode) error {
	if len(nodes) == 0 {
		fmt.Fprintln(out, "(empty)")
		return nil
	}
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	for _, n := range nodes {
		typeChar := "-"
		if n.Type == "dir" {
			typeChar = "d"
		} else if n.Type == "symlink" {
			typeChar = "l"
		}
		sealed := ""
		if n.IsSealed {
			sealed = " [sealed]"
		}
		fmt.Fprintf(w, "%s\t%s\t%s%s\n", typeChar, formatBytes(n.Size), n.Path, sealed)
	}
	return w.Flush()
}

func printFindResults(out io.Writer, results []snapshots.FindResult) error {
	if len(results) == 0 {
		fmt.Fprintln(out, "No matches found.")
		return nil
	}
	for _, r := range results {
		fmt.Fprintf(out, "snapshot %s (%s)\n", r.Snapshot.ShortID, r.Snapshot.Time.Format(time.RFC3339))
		for _, m := range r.Matches {
			sealed := ""
			if m.IsSealed {
				sealed = " [sealed]"
			}
			fmt.Fprintf(out, "  %s%s\n", m.Path, sealed)
		}
	}
	return nil
}

func printDiff(out io.Writer, entries []snapshots.DiffEntry) error {
	if len(entries) == 0 {
		fmt.Fprintln(out, "No differences.")
		return nil
	}
	for _, e := range entries {
		symbol := changeSymbol(e.ChangeType)
		sealed := ""
		if e.IsSealed {
			sealed = " [sealed]"
		}
		fmt.Fprintf(out, "%s %s%s\n", symbol, e.Path, sealed)
	}
	return nil
}

func changeSymbol(ct string) string {
	switch ct {
	case "added":
		return "+"
	case "removed":
		return "-"
	case "changed":
		return "~"
	default:
		return "?"
	}
}

func writeJSON(out io.Writer, v any) error {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// --- filter builder ---

func buildFilter(tags []string, hostname, after, before string) (snapshots.SnapshotFilter, error) {
	f := snapshots.SnapshotFilter{
		Tags:     tags,
		Hostname: hostname,
	}
	if after != "" {
		t, err := parseTime(after)
		if err != nil {
			return f, fmt.Errorf("--after: %w", err)
		}
		f.After = t
	}
	if before != "" {
		t, err := parseTime(before)
		if err != nil {
			return f, fmt.Errorf("--before: %w", err)
		}
		f.Before = t
	}
	return f, nil
}

// parseTime accepts RFC3339 or YYYY-MM-DD date strings.
func parseTime(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		// Treat a bare date as the start of that UTC day.
		return t, nil
	}
	return time.Time{}, fmt.Errorf("cannot parse %q as RFC3339 or YYYY-MM-DD", s)
}

func formatBytes(b int64) string {
	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
	)
	switch {
	case b >= GB:
		return fmt.Sprintf("%.1f GiB", float64(b)/float64(GB))
	case b >= MB:
		return fmt.Sprintf("%.1f MiB", float64(b)/float64(MB))
	case b >= KB:
		return fmt.Sprintf("%.1f KiB", float64(b)/float64(KB))
	default:
		return fmt.Sprintf("%d B", b)
	}
}

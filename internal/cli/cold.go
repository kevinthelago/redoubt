package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"text/tabwriter"
	"time"

	"github.com/kevinthelago/redoubt/internal/cold"
	"github.com/spf13/cobra"
)

// NewColdCopyCmd returns the "cold-copy" command and its sub-commands.
//
// Running `redoubt cold-copy` (with no subcommand) performs the copy cycle
// directly. Management subcommands (register, remove, list, status) are
// available for drive administration.
//
// engine must be non-nil; it is the single entry point for all cold-copy
// operations. Call this from the root command during CLI construction.
func NewColdCopyCmd(engine *cold.Engine) *cobra.Command {
	var (
		driveLabel    string
		skipRetention bool
		noWait        bool
	)

	cmd := &cobra.Command{
		Use:   "cold-copy",
		Short: "Copy vault snapshots to an offline removable drive",
		Long: `cold-copy keeps a disconnected restic repository on a removable drive,
forming the offline leg of a 3-2-1 backup scheme.

Running 'cold-copy' with no subcommand performs a full copy cycle:
  1. Detects a registered cold drive (or use --drive to target one).
  2. Initialises the cold repository on first use (same key as vault).
  3. Copies new snapshots from the vault via restic copy.
  4. Verifies the repository with restic check.
  5. Applies the cold-drive retention policy.
  6. Prompts to safely disconnect the drive.

Use subcommands to manage registered drives (register, remove, list, status).`,
		Example: `  # Run the copy flow (auto-select the best-candidate drive)
  redoubt cold-copy

  # Copy to a specific drive
  redoubt cold-copy --drive wd-blue-1

  # Register a new drive first
  redoubt cold-copy register --label wd-blue-1 --path E:\`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}

			opts := cold.RunOptions{
				DriveLabel:    driveLabel,
				SkipRetention: skipRetention,
				Progress: func(msg string) {
					fmt.Fprintln(cmd.OutOrStdout(), "  →", msg)
				},
			}

			result, err := engine.Run(ctx, opts)
			if err != nil {
				return formatColdError(err)
			}

			fmt.Fprintln(cmd.OutOrStdout())
			fmt.Fprintf(cmd.OutOrStdout(), "✓ Cold copy complete in %s\n", result.Duration.Round(time.Second))
			if result.Initialised {
				fmt.Fprintln(cmd.OutOrStdout(), "  (cold repository initialised for the first time)")
			}
			fmt.Fprintf(cmd.OutOrStdout(), "\n  Drive: %s (%s)\n\n",
				result.Drive.Label, result.Drive.MountPath)

			if !noWait {
				promptDisconnect(cmd.OutOrStdout(), result.Drive.Label)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&driveLabel, "drive", "", "use a specific registered drive (label); auto-selects if omitted")
	cmd.Flags().BoolVar(&skipRetention, "skip-retention", false, "skip the forget/prune step")
	cmd.Flags().BoolVar(&noWait, "no-wait", false, "return immediately without the disconnect prompt")

	cmd.AddCommand(
		newColdRegisterCmd(engine),
		newColdRemoveCmd(engine),
		newColdListCmd(engine),
		newColdStatusCmd(engine),
	)
	return cmd
}

// newColdRegisterCmd adds a new drive to the registry.
func newColdRegisterCmd(engine *cold.Engine) *cobra.Command {
	var (
		label      string
		mountPath  string
		repoSubdir string
	)

	cmd := &cobra.Command{
		Use:   "register",
		Short: "Register a removable drive for cold copies",
		Long: `Register a removable drive so Redoubt can copy vault snapshots to it.

The drive is identified by a user-chosen label and its expected mount path
when plugged in.  On Windows, mount paths are typically drive letters
(e.g. E:\); on Linux they are usually /media/user/<name>.

The restic repository is created inside mount-path/repo-subdir on first use.`,
		Example: `  redoubt cold-copy register --label wd-blue-1 --path E:\
  redoubt cold-copy register --label usb-offsite --path /media/user/BACKUP --repo-subdir redoubt`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if label == "" {
				return fmt.Errorf("--label is required")
			}
			if mountPath == "" {
				return fmt.Errorf("--path is required")
			}

			d := cold.DriveRecord{
				Label:      label,
				MountPath:  mountPath,
				RepoSubdir: repoSubdir,
			}
			if err := engine.Register(d); err != nil {
				return fmt.Errorf("register drive: %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Registered cold drive: %s (%s)\n", label, mountPath)
			return nil
		},
	}

	cmd.Flags().StringVar(&label, "label", "", "friendly name for this drive (required)")
	cmd.Flags().StringVar(&mountPath, "path", "", "expected mount path when connected (required)")
	cmd.Flags().StringVar(&repoSubdir, "repo-subdir", "redoubt", "subdirectory on the drive for the restic repository")
	_ = cmd.MarkFlagRequired("label")
	_ = cmd.MarkFlagRequired("path")

	return cmd
}

// newColdRemoveCmd removes a drive from the registry.
func newColdRemoveCmd(engine *cold.Engine) *cobra.Command {
	var label string

	cmd := &cobra.Command{
		Use:   "remove",
		Short: "Remove a registered cold drive",
		Long:  "Remove a drive from the registry.  The data on the drive is not affected.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if label == "" {
				return fmt.Errorf("--label is required")
			}
			if err := engine.Remove(label); err != nil {
				return fmt.Errorf("remove drive: %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Removed cold drive: %s\n", label)
			return nil
		},
	}

	cmd.Flags().StringVar(&label, "label", "", "label of the drive to remove (required)")
	_ = cmd.MarkFlagRequired("label")
	return cmd
}

// newColdListCmd lists all registered drives.
func newColdListCmd(engine *cold.Engine) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List registered cold drives",
		RunE: func(cmd *cobra.Command, _ []string) error {
			drives := engine.List()
			if len(drives) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No cold drives registered.")
				fmt.Fprintln(cmd.OutOrStdout(), "Run 'redoubt cold-copy register' to add one.")
				return nil
			}

			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "LABEL\tMOUNT PATH\tLAST COPY\tCOPIES\tACTIVE")
			for _, d := range drives {
				lastCopy := "never"
				if !d.LastCopyTime.IsZero() {
					lastCopy = d.LastCopyTime.Format("2006-01-02 15:04")
				}
				active := ""
				if d.Active {
					active = "✓"
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%s\n",
					d.Label, d.MountPath, lastCopy, d.TotalCopies, active)
			}
			return w.Flush()
		},
	}
}

// newColdStatusCmd shows staleness and mount state of all drives.
func newColdStatusCmd(engine *cold.Engine) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show cold-copy status and staleness warnings",
		RunE: func(cmd *cobra.Command, _ []string) error {
			statuses := engine.Status()
			if len(statuses) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No cold drives registered.")
				return nil
			}

			out := cmd.OutOrStdout()
			w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "LABEL\tMOUNTED\tLAST COPY\tDAYS AGO\tSTATUS")

			exitCode := 0
			for _, s := range statuses {
				mounted := "no"
				if s.Mounted {
					mounted = "yes"
				}
				lastCopy := "never"
				daysAgo := "—"
				if !s.Drive.LastCopyTime.IsZero() {
					lastCopy = s.Drive.LastCopyTime.Format("2006-01-02")
					if s.Staleness.DaysSinceLastCopy >= 0 {
						daysAgo = fmt.Sprintf("%d", s.Staleness.DaysSinceLastCopy)
					}
				}
				statusStr := formatStaleness(s.Staleness.Level)
				if s.Staleness.Level >= cold.StalenessWarning {
					exitCode = 1
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
					s.Drive.Label, mounted, lastCopy, daysAgo, statusStr)
			}
			if err := w.Flush(); err != nil {
				return err
			}

			if exitCode != 0 {
				return &exitError{code: exitCode}
			}
			return nil
		},
	}
}

// formatStaleness returns a human-readable status string for a staleness level.
func formatStaleness(l cold.StalenessLevel) string {
	switch l {
	case cold.StalenessOK:
		return "OK"
	case cold.StalenessWarning:
		return "WARNING – consider doing a cold copy soon"
	case cold.StalenessError:
		return "ERROR – cold copy overdue"
	default:
		return "unknown (never copied)"
	}
}

// formatColdError produces user-friendly error messages for cold-copy errors.
func formatColdError(err error) error {
	switch {
	case isType[*cold.ErrDriveNotMounted](err):
		return fmt.Errorf("%w\nPlug in the drive and retry, or omit --drive to auto-select a mounted drive", err)
	case isType[*cold.ErrNoDriveMounted](err):
		return fmt.Errorf("%w\nPlug in a registered drive and retry, or run 'redoubt cold-copy list' to see registered drives", err)
	case isType[*cold.ErrNoDriveRegistered](err):
		return fmt.Errorf("%w\nRun 'redoubt cold-copy register' to add a drive first", err)
	default:
		return err
	}
}

// promptDisconnect prints the safe-to-disconnect message and waits for the
// user to press Enter, giving them time to physically unplug the drive.
func promptDisconnect(out io.Writer, label string) {
	if out == nil {
		out = os.Stdout
	}
	fmt.Fprintf(out, "Safe to disconnect: %s\n", label)
	fmt.Fprint(out, "Press Enter after unplugging the drive… ")
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Scan()
	fmt.Fprintln(out, "Done.")
}

// isType reports whether err is (or wraps) an error of type T.
func isType[T error](err error) bool {
	var target T
	return errors.As(err, &target)
}

// exitError is a sentinel that carries an exit code but no message.
// The CLI framework should translate this to os.Exit(code).
type exitError struct {
	code int
}

func (e *exitError) Error() string { return "" }

// ExitCode returns the intended process exit code for status-check commands.
// Returns 0 when err is nil or not an exitError.
func ExitCode(err error) int {
	var e *exitError
	if errors.As(err, &e) {
		return e.code
	}
	return 0
}

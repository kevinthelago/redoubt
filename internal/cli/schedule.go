package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/kevinthelago/redoubt/internal/config"
	"github.com/kevinthelago/redoubt/internal/schedule"
)

func init() {
	scheduleCmd.Long = `Manage the automated daily backup schedule.

'schedule set' installs an OS-level task that runs 'redoubt backup' at the
configured time each day.  Missed runs are caught up automatically on the next
machine start-up.

  Windows: Task Scheduler entry (StartWhenAvailable=true, MultipleInstances=IgnoreNew)
  Linux:   systemd user timer (~/.config/systemd/user/redoubt-backup.timer, Persistent=true)

Retention (restic forget --prune) applies only on vault hosts
(REDOUBT_IS_VAULT=1) so developer machines never prune backup history.`
	scheduleCmd.RunE = nil
	scheduleCmd.AddCommand(
		newScheduleSetCmd(),
		newScheduleStatusCmd(),
		newScheduleRemoveCmd(),
		newScheduleRunCmd(),
	)
}

func newScheduleSetCmd() *cobra.Command {
	var runTime string
	var keepDaily, keepWeekly, keepMonthly int

	cmd := &cobra.Command{
		Use:   "set",
		Short: "Install the daily backup schedule",
		Long: `Install a daily backup task in the OS scheduler.

  Windows: creates a Task Scheduler entry with StartWhenAvailable=true
  Linux:   writes a systemd user timer with Persistent=true

Missed runs are caught up on the next machine boot.  Concurrent executions
are prevented by Task Scheduler / systemd and by the backup lock.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := schedule.Install(runTime); err != nil {
				printElevationHint(cmd, err)
				return err
			}

			if keepDaily > 0 || keepWeekly > 0 || keepMonthly > 0 {
				cfg, err := config.Load()
				if err != nil {
					return fmt.Errorf("load config: %w", err)
				}
				if keepDaily > 0 {
					cfg.Retention.Daily = keepDaily
				}
				if keepWeekly > 0 {
					cfg.Retention.Weekly = keepWeekly
				}
				if keepMonthly > 0 {
					cfg.Retention.Monthly = keepMonthly
				}
				if err := config.Save(cfg); err != nil {
					return fmt.Errorf("save retention config: %w", err)
				}
			}

			fmt.Fprintln(cmd.OutOrStdout(), "Backup schedule installed.")
			fmt.Fprintln(cmd.OutOrStdout(), "Run 'redoubt schedule status' to confirm the next run time.")
			return nil
		},
	}

	cmd.Flags().StringVarP(&runTime, "time", "t", "02:00", "daily run time in 24-hour HH:MM format")
	cmd.Flags().IntVar(&keepDaily, "keep-daily", 0, "daily snapshots to keep (vault-side; 0 keeps current setting)")
	cmd.Flags().IntVar(&keepWeekly, "keep-weekly", 0, "weekly snapshots to keep (vault-side; 0 keeps current setting)")
	cmd.Flags().IntVar(&keepMonthly, "keep-monthly", 0, "monthly snapshots to keep (vault-side; 0 keeps current setting)")
	return cmd
}

func newScheduleStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show the next scheduled run and the last run result",
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()

			nextRun, err := schedule.NextRun()
			if err != nil {
				fmt.Fprintf(out, "Next run:  (not installed — run 'redoubt schedule set')\n")
			} else {
				fmt.Fprintf(out, "Next run:  %s\n", nextRun.Local().Format("Mon Jan 2, 2006 at 3:04 PM"))
			}

			last, err := schedule.ReadStatus()
			if err != nil {
				return fmt.Errorf("read last-run status: %w", err)
			}
			if last.IsZero() {
				fmt.Fprintf(out, "Last run:  never\n")
				return nil
			}

			result := "ok"
			if last.ExitCode != 0 {
				result = fmt.Sprintf("FAILED (exit %d)", last.ExitCode)
				if last.Error != "" {
					result += ": " + last.Error
				}
			}
			duration := last.FinishedAt.Sub(last.StartedAt).Round(time.Second)
			fmt.Fprintf(out, "Last run:  %s — %s in %s\n",
				last.StartedAt.Local().Format("Mon Jan 2, 2006 at 3:04 PM"),
				result,
				formatDuration(duration),
			)
			return nil
		},
	}
}

func newScheduleRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove",
		Short: "Remove the scheduled backup task",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := schedule.Remove(); err != nil {
				printElevationHint(cmd, err)
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Scheduled backup removed.")
			return nil
		},
	}
}

// newScheduleRunCmd returns the hidden command invoked by the OS task scheduler.
// It is not intended for direct user use.
func newScheduleRunCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "run",
		Short:  "Execute a scheduled backup (invoked by the OS scheduler)",
		Hidden: true,
		RunE: func(_ *cobra.Command, _ []string) error {
			return schedule.Run(context.Background())
		},
	}
}

// printElevationHint writes platform-specific elevation advice when err
// indicates a missing OS permission.
func printElevationHint(cmd *cobra.Command, err error) {
	if !isElevationError(err) {
		return
	}
	fmt.Fprintf(cmd.ErrOrStderr(),
		"error: elevated permissions required\n"+
			"  Windows: re-run from an Administrator prompt\n"+
			"  Linux:   enable linger with: loginctl enable-linger\n\n")
}

// isElevationError reports whether err indicates a missing OS permission.
func isElevationError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "access is denied") ||
		strings.Contains(msg, "permission denied") ||
		strings.Contains(msg, "operation not permitted") ||
		strings.Contains(msg, "this command requires elevation")
}

// formatDuration formats d as a concise human-readable string.
func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh %dm", h, m)
	}
	if m > 0 {
		return fmt.Sprintf("%dm %ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

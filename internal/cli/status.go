// Package cli holds the Cobra sub-commands for the redoubt binary.
// This file implements the `redoubt status` command (issue H3).
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/kevinthelago/redoubt/internal/health"
	"github.com/spf13/cobra"
)

// NewStatusCmd creates the `redoubt status` Cobra command.
// Wire it into the root command with root.AddCommand(NewStatusCmd()).
func NewStatusCmd() *cobra.Command {
	var (
		asJSON   bool
		dataDir  string
		refresh  bool
		ntfyURL  string
	)

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show the current backup protection status",
		Long: `Display a dashboard of per-signal health grades and an overall
protection state. Works fully offline.

Exit codes:
  0  healthy
  1  warning
  2  critical
  3  unknown / not yet protected`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runStatus(cmd.Context(), dataDir, asJSON, refresh, ntfyURL)
		},
	}

	cmd.Flags().BoolVar(&asJSON, "json", false, "output machine-readable JSON to stdout")
	cmd.Flags().StringVar(&dataDir, "data-dir", "", "override the Redoubt data directory")
	// --refresh re-evaluates health live instead of reading the cached status.json.
	cmd.Flags().BoolVar(&refresh, "refresh", false, "re-evaluate health signals now (may check vault reachability)")
	cmd.Flags().StringVar(&ntfyURL, "ntfy-url", "", "optional LAN-only ntfy endpoint for push alerts")

	return cmd
}

func runStatus(ctx context.Context, dir string, asJSON, refresh bool, ntfyURL string) error {
	if dir == "" {
		var err error
		dir, err = health.DataDir()
		if err != nil {
			return fmt.Errorf("resolving data dir: %w", err)
		}
	}

	reader := health.NewStateReader(dir)

	var status health.Status
	var err error

	if refresh {
		// Re-evaluate signals live and update status.json + send notifications.
		status, err = health.CheckAndNotify(ctx, dir, health.DefaultThresholds(), ntfyURL)
	} else {
		// Fast path: read the last written status.json.
		status, err = reader.ReadStatus()
		if err != nil {
			return fmt.Errorf("reading status: %w", err)
		}
		// If status.json doesn't exist yet (EvaluatedAt zero), evaluate live.
		if status.EvaluatedAt.IsZero() {
			status, err = health.CheckAndNotify(ctx, dir, health.DefaultThresholds(), ntfyURL)
		}
	}
	if err != nil {
		return fmt.Errorf("evaluating health: %w", err)
	}

	if asJSON {
		return printJSON(status)
	}
	printDashboard(status)
	os.Exit(status.ExitCode())
	return nil
}

func printJSON(s health.Status) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(s); err != nil {
		return err
	}
	os.Exit(s.ExitCode())
	return nil
}

// Signal display order.
var signalOrder = []string{
	health.SignalLastBackup,
	health.SignalSchedule,
	health.SignalIntegrity,
	health.SignalDrill,
	health.SignalColdCopy,
	health.SignalVault,
	health.SignalDisk,
}

// signalLabel is the human-friendly name for each signal.
var signalLabel = map[string]string{
	health.SignalLastBackup: "Last Backup",
	health.SignalSchedule:   "Schedule",
	health.SignalIntegrity:  "Integrity",
	health.SignalDrill:      "Drill",
	health.SignalColdCopy:   "Cold Copy",
	health.SignalVault:      "Vault",
	health.SignalDisk:       "Disk Space",
}

func printDashboard(s health.Status) {
	fmt.Printf("\n  Redoubt Protection Status\n")
	fmt.Printf("  %s\n\n", time.Now().Format("Mon 2 Jan 2006 15:04 MST"))

	w := tabwriter.NewWriter(os.Stdout, 2, 0, 2, ' ', 0)
	for _, name := range signalOrder {
		sig, ok := s.Signals[name]
		if !ok {
			continue
		}
		label := signalLabel[name]
		icon := gradeIcon(sig.Grade)
		grade := sig.Grade.String()
		fmt.Fprintf(w, "  %s\t%-13s\t%-10s\t%s\n", icon, label, grade, sig.Message)
	}
	_ = w.Flush()

	fmt.Printf("\n  Overall:  %s %s\n\n", gradeIcon(s.Overall), overallLine(s.Overall))
}

func gradeIcon(g health.Grade) string {
	switch g {
	case health.GradeHealthy:
		return "✓"
	case health.GradeWarning:
		return "⚠"
	case health.GradeCritical:
		return "✗"
	default:
		return "?"
	}
}

func overallLine(g health.Grade) string {
	switch g {
	case health.GradeHealthy:
		return "PROTECTED"
	case health.GradeWarning:
		return "WARNING — action recommended"
	case health.GradeCritical:
		return "CRITICAL — action required"
	default:
		return "NOT YET PROTECTED"
	}
}

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"text/tabwriter"
	"time"

	"github.com/kevinthelago/redoubt/internal/config"
	"github.com/kevinthelago/redoubt/internal/drill"
	"github.com/spf13/cobra"
)

func init() {
	// Replace the stub drillCmd (defined in root.go) with the full implementation.
	drillCmd.Short = "Run and schedule restore drills"
	drillCmd.Long = `Restore drills prove that backups are actually recoverable.

A drill restores a rotating file sample plus a fixed canary from a recent
snapshot into an isolated scratch directory, verifies their SHA-256 hashes
against the snapshot manifest, records the result, and wipes the scratch dir.

The drill never modifies live data.`
	drillCmd.RunE = nil
	drillCmd.AddCommand(
		newDrillRunCmd(),
		newDrillHistoryCmd(),
		newDrillScheduleCmd(),
	)
}

// drillDataDir returns the directory used for drill state files.
func drillDataDir() string {
	return filepath.Join(config.DataDir(), "drill")
}

// drillHistoryPath is the JSON file that accumulates all drill results.
func drillHistoryPath() string {
	return filepath.Join(drillDataDir(), "history.json")
}

// drillStatusPath is the machine-readable summary consumed by health monitoring.
func drillStatusPath() string {
	return filepath.Join(drillDataDir(), "status.json")
}

// drillCadencePath stores the operator-configured cadence preferences.
func drillCadencePath() string {
	return filepath.Join(drillDataDir(), "cadence.json")
}

// newDrillRunCmd implements `redoubt drill run [--source vault|cold] [--force]`.
func newDrillRunCmd() *cobra.Command {
	var (
		sourceFlag string
		forceFlag  bool
		jsonFlag   bool
	)
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run a restore drill immediately",
		RunE: func(cmd *cobra.Command, _ []string) error {
			src, err := parseDrillSource(sourceFlag)
			if err != nil {
				return err
			}
			e := newDrillEngine()
			result, err := e.Run(cmd.Context(), drill.RunOptions{
				Source: src,
				Force:  forceFlag,
			})
			if err != nil {
				return fmt.Errorf("drill: %w", err)
			}
			if jsonFlag {
				return drillPrintJSON(cmd, result)
			}
			drillPrintResult(cmd, result)
			if result.Status == drill.StatusFail {
				os.Exit(1)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&sourceFlag, "source", "vault", "snapshot source to drill: vault or cold")
	cmd.Flags().BoolVar(&forceFlag, "force", false, "run immediately, bypassing cadence check")
	cmd.Flags().BoolVar(&jsonFlag, "json", false, "output result as JSON")
	return cmd
}

// newDrillHistoryCmd implements `redoubt drill history [--source] [-n N]`.
func newDrillHistoryCmd() *cobra.Command {
	var (
		sourceFlag string
		nFlag      int
		jsonFlag   bool
	)
	cmd := &cobra.Command{
		Use:   "history",
		Short: "Show drill result history",
		RunE: func(cmd *cobra.Command, _ []string) error {
			h, err := drill.LoadHistory(drillHistoryPath())
			if err != nil {
				return fmt.Errorf("load history: %w", err)
			}
			results := h.Results
			if sourceFlag != "" {
				src, err := parseDrillSource(sourceFlag)
				if err != nil {
					return err
				}
				var filtered []drill.Result
				for _, r := range results {
					if r.Source == src {
						filtered = append(filtered, r)
					}
				}
				results = filtered
			}
			if nFlag > 0 && nFlag < len(results) {
				results = results[:nFlag]
			}
			if jsonFlag {
				return drillPrintJSON(cmd, results)
			}
			if len(results) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No drill history found.")
				return nil
			}
			drillPrintHistory(cmd, results)
			return nil
		},
	}
	cmd.Flags().StringVar(&sourceFlag, "source", "", "filter by source: vault or cold (default: all)")
	cmd.Flags().IntVarP(&nFlag, "number", "n", 20, "max number of results to show")
	cmd.Flags().BoolVar(&jsonFlag, "json", false, "output as JSON")
	return cmd
}

// newDrillScheduleCmd provides `redoubt drill schedule {set|status|remove}`.
func newDrillScheduleCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "schedule",
		Short: "Manage the drill cadence",
	}
	cmd.AddCommand(
		newDrillScheduleSetCmd(),
		newDrillScheduleStatusCmd(),
		newDrillScheduleRemoveCmd(),
	)
	return cmd
}

func newDrillScheduleSetCmd() *cobra.Command {
	var (
		vaultCadence string
		coldCadence  string
	)
	cmd := &cobra.Command{
		Use:   "set",
		Short: "Set the drill cadence",
		Long:  "Configure how often vault and cold-drive drills run (daily, weekly, monthly).",
		RunE: func(cmd *cobra.Command, _ []string) error {
			vc, ok := drill.ParseCadence(vaultCadence)
			if !ok {
				return fmt.Errorf("invalid --vault-cadence %q: choose daily, weekly, or monthly", vaultCadence)
			}
			cc, ok := drill.ParseCadence(coldCadence)
			if !ok {
				return fmt.Errorf("invalid --cold-cadence %q: choose daily, weekly, or monthly", coldCadence)
			}
			cs := loadDrillCadence()
			cs.VaultCadence = vc
			cs.ColdCadence = cc
			if err := saveDrillCadence(cs); err != nil {
				return fmt.Errorf("save cadence: %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(),
				"Drill cadence set: vault=%s, cold=%s\n",
				drill.FormatCadence(vc), drill.FormatCadence(cc))
			fmt.Fprintln(cmd.OutOrStdout(),
				"Add `redoubt drill run` to your OS task scheduler to enforce this cadence.")
			return nil
		},
	}
	cmd.Flags().StringVar(&vaultCadence, "vault-cadence", "weekly", "how often to drill the vault (daily|weekly|monthly)")
	cmd.Flags().StringVar(&coldCadence, "cold-cadence", "monthly", "how often to drill the cold drive (daily|weekly|monthly)")
	return cmd
}

func newDrillScheduleStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show current cadence and when drills are next due",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cs := loadDrillCadence()
			h, _ := drill.LoadHistory(drillHistoryPath())

			tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "SOURCE\tCADENCE\tLAST RUN\tLAST STATUS\tNEXT DUE")
			for _, row := range []struct {
				src     drill.Source
				cadence time.Duration
			}{
				{drill.SourceVault, cs.VaultCadence},
				{drill.SourceCold, cs.ColdCadence},
			} {
				last, ok := h.Last(row.src)
				var lastStr, statusStr, nextStr string
				if ok {
					lastStr = last.Timestamp.Local().Format(time.RFC822)
					statusStr = string(last.Status)
					if drill.IsDue(last.Timestamp, row.cadence) {
						nextStr = "NOW"
					} else {
						nextStr = last.Timestamp.Add(row.cadence).Local().Format(time.RFC822)
					}
				} else {
					lastStr = "never"
					statusStr = "-"
					nextStr = "NOW"
				}
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
					row.src, drill.FormatCadence(row.cadence), lastStr, statusStr, nextStr)
			}
			return tw.Flush()
		},
	}
}

func newDrillScheduleRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove",
		Short: "Remove the saved cadence configuration",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := os.Remove(drillCadencePath()); err != nil && !os.IsNotExist(err) {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Drill cadence configuration removed.")
			return nil
		},
	}
}

// cadenceState persists the operator-configured drill cadences.
type cadenceState struct {
	VaultCadence time.Duration `json:"vault_cadence_ns"`
	ColdCadence  time.Duration `json:"cold_cadence_ns"`
}

func loadDrillCadence() cadenceState {
	data, err := os.ReadFile(drillCadencePath())
	if err != nil {
		return cadenceState{
			VaultCadence: drill.DefaultVaultCadence,
			ColdCadence:  drill.DefaultColdCadence,
		}
	}
	var cs cadenceState
	if json.Unmarshal(data, &cs) != nil {
		return cadenceState{
			VaultCadence: drill.DefaultVaultCadence,
			ColdCadence:  drill.DefaultColdCadence,
		}
	}
	return cs
}

func saveDrillCadence(cs cadenceState) error {
	path := drillCadencePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, _ := json.MarshalIndent(cs, "", "  ")
	return os.WriteFile(path, data, 0o600)
}

// newDrillEngine builds a drill.Engine using configured paths.
// Wires a stubRestorer until R1 provides the real restore backend.
func newDrillEngine() *drill.Engine {
	return drill.NewEngine(
		drill.Config{
			ScratchBaseDir:    filepath.Join(os.TempDir(), "redoubt-drill"),
			HistoryPath:       drillHistoryPath(),
			StatusPath:        drillStatusPath(),
			MaxSampleFiles:    10,
			MinFreeSpaceBytes: 500 * 1024 * 1024,
			HistoryMaxLen:     100,
		},
		&drillStubRestorer{},
		nil,
		slog.Default(),
	)
}

// drillStubRestorer satisfies drill.Restorer with clear "not yet integrated" errors.
// Replace with the real restore.Restorer once R1 is integrated.
type drillStubRestorer struct{}

func (s *drillStubRestorer) LatestSnapshotID(_ context.Context, _ drill.Source) (string, bool, error) {
	return "", false, fmt.Errorf("restore backend not yet integrated (R1 pending)")
}

func (s *drillStubRestorer) ListSnapshotFiles(_ context.Context, _ drill.Source, _, _ string) ([]string, error) {
	return nil, fmt.Errorf("restore backend not yet integrated (R1 pending)")
}

func (s *drillStubRestorer) RestoreFiles(_ context.Context, _ drill.Source, _ string, _ []string, _ string) error {
	return fmt.Errorf("restore backend not yet integrated (R1 pending)")
}

func (s *drillStubRestorer) FreeSpaceAt(path string) (uint64, error) {
	return freeSpaceAt(path)
}

// drillPrintResult writes a human-readable drill result to cmd's output.
func drillPrintResult(cmd *cobra.Command, r drill.Result) {
	out := cmd.OutOrStdout()
	icon := map[drill.Status]string{
		drill.StatusPass: "✓",
		drill.StatusFail: "✗",
		drill.StatusSkip: "○",
	}[r.Status]
	fmt.Fprintf(out, "%s Drill %s — %s\n", icon, r.Source, r.Status)
	fmt.Fprintf(out, "  Snapshot:  %s\n", nvlDrill(r.SnapshotID, "n/a"))
	fmt.Fprintf(out, "  Restored:  %d  Verified: %d  Failed: %d\n",
		r.Restored, r.Verified, r.Failed)
	fmt.Fprintf(out, "  Duration:  %dms\n", r.DurationMS)
	if r.SkipReason != "" {
		fmt.Fprintf(out, "  Reason:    %s\n", r.SkipReason)
	}
	if r.FailureMsg != "" {
		fmt.Fprintf(out, "  Failure:   %s\n", r.FailureMsg)
	}
}

// drillPrintHistory renders results as an ASCII table.
func drillPrintHistory(cmd *cobra.Command, results []drill.Result) {
	tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "TIME\tSOURCE\tSTATUS\tVERIFIED\tFAILED\tSNAPSHOT")
	for _, r := range results {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%d\t%s\n",
			r.Timestamp.Local().Format("2006-01-02 15:04"),
			r.Source, r.Status,
			r.Verified, r.Failed,
			nvlDrill(r.SnapshotID, "-"),
		)
	}
	_ = tw.Flush()
}

func drillPrintJSON(cmd *cobra.Command, v any) error {
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func parseDrillSource(s string) (drill.Source, error) {
	switch drill.Source(s) {
	case drill.SourceVault, drill.SourceCold:
		return drill.Source(s), nil
	}
	return "", fmt.Errorf("unknown source %q: choose vault or cold", s)
}

func nvlDrill(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

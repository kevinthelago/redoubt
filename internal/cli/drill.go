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

// NewDrillCmd returns the `redoubt drill` command tree.
func NewDrillCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "drill",
		Short: "Run and schedule restore drills",
		Long: `Restore drills prove that backups are actually recoverable.

A drill restores a rotating file sample plus a fixed canary from a recent
snapshot into an isolated scratch directory, verifies their SHA-256 hashes
against the snapshot manifest, records the result, and wipes the scratch dir.

The drill never modifies live data.`,
	}

	cmd.AddCommand(
		newDrillRunCmd(),
		newDrillHistoryCmd(),
		newDrillScheduleCmd(),
	)
	return cmd
}

// drillRunCmd implements `redoubt drill run [--source vault|cold] [--force]`.
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
			src, err := parseSource(sourceFlag)
			if err != nil {
				return err
			}
			cfg := config.DefaultDrillConfig()
			e := newEngine(cfg)
			result, err := e.Run(cmd.Context(), drill.RunOptions{
				Source: src,
				Force:  forceFlag,
			})
			if err != nil {
				return fmt.Errorf("drill: %w", err)
			}

			if jsonFlag {
				return printJSON(result)
			}
			printResult(result)
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

// drillHistoryCmd implements `redoubt drill history [--source vault|cold] [-n N]`.
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
			cfg := config.DefaultDrillConfig()
			h, err := drill.LoadHistory(cfg.HistoryPath)
			if err != nil {
				return fmt.Errorf("load history: %w", err)
			}

			results := h.Results
			if sourceFlag != "" {
				src, err := parseSource(sourceFlag)
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
				return printJSON(results)
			}
			if len(results) == 0 {
				fmt.Println("No drill history found.")
				return nil
			}
			printHistory(results)
			return nil
		},
	}
	cmd.Flags().StringVar(&sourceFlag, "source", "", "filter by source: vault or cold (default: all)")
	cmd.Flags().IntVarP(&nFlag, "number", "n", 20, "max number of results to show")
	cmd.Flags().BoolVar(&jsonFlag, "json", false, "output as JSON")
	return cmd
}

// drillScheduleCmd implements `redoubt drill schedule {set|status|remove}`.
func newDrillScheduleCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "schedule",
		Short: "Manage the drill cadence",
	}
	cmd.AddCommand(
		newScheduleSetCmd(),
		newScheduleStatusCmd(),
		newScheduleRemoveCmd(),
	)
	return cmd
}

func newScheduleSetCmd() *cobra.Command {
	var (
		vaultCadence string
		coldCadence  string
	)
	cmd := &cobra.Command{
		Use:   "set",
		Short: "Set the drill cadence",
		Long:  "Configure how often the vault and cold-drive drills run (daily, weekly, monthly).",
		RunE: func(cmd *cobra.Command, _ []string) error {
			vc, ok := drill.ParseCadence(vaultCadence)
			if !ok {
				return fmt.Errorf("invalid --vault-cadence %q: choose daily, weekly, or monthly", vaultCadence)
			}
			cc, ok := drill.ParseCadence(coldCadence)
			if !ok {
				return fmt.Errorf("invalid --cold-cadence %q: choose daily, weekly, or monthly", coldCadence)
			}
			cs := loadCadenceState()
			cs.VaultCadence = vc
			cs.ColdCadence = cc
			if err := saveCadenceState(cs); err != nil {
				return fmt.Errorf("save cadence: %w", err)
			}
			fmt.Printf("Drill cadence set: vault=%s, cold=%s\n",
				drill.FormatCadence(vc), drill.FormatCadence(cc))
			fmt.Println("Add `redoubt drill run` to your OS task scheduler to enforce this cadence.")
			return nil
		},
	}
	cmd.Flags().StringVar(&vaultCadence, "vault-cadence", "weekly", "how often to drill the vault (daily|weekly|monthly)")
	cmd.Flags().StringVar(&coldCadence, "cold-cadence", "monthly", "how often to drill the cold drive (daily|weekly|monthly)")
	return cmd
}

func newScheduleStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show current cadence and when drills are next due",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg := config.DefaultDrillConfig()
			cs := loadCadenceState()
			h, _ := drill.LoadHistory(cfg.HistoryPath)

			tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
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

func newScheduleRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove",
		Short: "Remove the saved cadence configuration",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := removeCadenceState(); err != nil {
				return err
			}
			fmt.Println("Drill cadence configuration removed.")
			return nil
		},
	}
}

// cadenceState persists the configured drill cadences between CLI invocations.
type cadenceState struct {
	VaultCadence time.Duration `json:"vault_cadence_ns"`
	ColdCadence  time.Duration `json:"cold_cadence_ns"`
}

func cadenceStatePath() string {
	cfg := config.DefaultDrillConfig()
	return filepath.Join(filepath.Dir(cfg.HistoryPath), "drill-cadence.json")
}

func loadCadenceState() cadenceState {
	path := cadenceStatePath()
	data, err := os.ReadFile(path)
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

func saveCadenceState(cs cadenceState) error {
	path := cadenceStatePath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, _ := json.MarshalIndent(cs, "", "  ")
	return os.WriteFile(path, data, 0600)
}

func removeCadenceState() error {
	return os.Remove(cadenceStatePath())
}

// newEngine builds a drill.Engine wired to config defaults.
// In production the restorer will be the real restore.Restorer from R1.
// For now this returns a placeholder that surfaces a clear "not configured" error.
func newEngine(cfg config.DrillConfig) *drill.Engine {
	return drill.NewEngine(
		drill.Config{
			ScratchBaseDir:    cfg.ScratchBaseDir,
			HistoryPath:       cfg.HistoryPath,
			StatusPath:        cfg.StatusPath,
			MaxSampleFiles:    cfg.MaxSampleFiles,
			MinFreeSpaceBytes: cfg.MinFreeSpaceBytes,
			HistoryMaxLen:     100,
		},
		&stubRestorer{},
		nil,
		slog.Default(),
	)
}

// stubRestorer satisfies drill.Restorer with clear "not implemented" errors.
// Replace with the real restore.Restorer once R1 is integrated.
type stubRestorer struct{}

func (s *stubRestorer) LatestSnapshotID(_ context.Context, src drill.Source) (string, bool, error) {
	return "", false, fmt.Errorf("restore backend not configured (R1 integration pending)")
}
func (s *stubRestorer) ListSnapshotFiles(_ context.Context, _ drill.Source, _, _ string) ([]string, error) {
	return nil, fmt.Errorf("restore backend not configured (R1 integration pending)")
}
func (s *stubRestorer) RestoreFiles(_ context.Context, _ drill.Source, _ string, _ []string, _ string) error {
	return fmt.Errorf("restore backend not configured (R1 integration pending)")
}
func (s *stubRestorer) FreeSpaceAt(path string) (uint64, error) {
	return freeSpaceAt(path)
}

// printResult writes a human-readable summary of one drill result.
func printResult(r drill.Result) {
	icon := map[drill.Status]string{
		drill.StatusPass: "✓",
		drill.StatusFail: "✗",
		drill.StatusSkip: "○",
	}[r.Status]

	fmt.Printf("%s Drill %s — %s\n", icon, r.Source, r.Status)
	fmt.Printf("  Snapshot:  %s\n", nvl(r.SnapshotID, "n/a"))
	fmt.Printf("  Restored:  %d  Verified: %d  Failed: %d\n", r.Restored, r.Verified, r.Failed)
	fmt.Printf("  Duration:  %dms\n", r.DurationMS)
	if r.SkipReason != "" {
		fmt.Printf("  Reason:    %s\n", r.SkipReason)
	}
	if r.FailureMsg != "" {
		fmt.Printf("  Failure:   %s\n", r.FailureMsg)
	}
}

// printHistory renders the result list as an ASCII table.
func printHistory(results []drill.Result) {
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "TIME\tSOURCE\tSTATUS\tVERIFIED\tFAILED\tSNAPSHOT")
	for _, r := range results {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%d\t%s\n",
			r.Timestamp.Local().Format("2006-01-02 15:04"),
			r.Source,
			r.Status,
			r.Verified,
			r.Failed,
			nvl(r.SnapshotID, "-"),
		)
	}
	_ = tw.Flush()
}

func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func parseSource(s string) (drill.Source, error) {
	switch drill.Source(s) {
	case drill.SourceVault, drill.SourceCold:
		return drill.Source(s), nil
	}
	return "", fmt.Errorf("unknown source %q: choose vault or cold", s)
}

func nvl(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

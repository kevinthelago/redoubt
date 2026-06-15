// Package cli provides the Cobra command implementations for the redoubt CLI.
// Each file in this package defines one command group.
//
// This file defines the `redoubt backup` command.
//
// DEPENDENCY NOTE: the full wiring of this command (connecting the Pipeline to
// the concrete foundation/assets/vault packages) will be completed once the
// following PRs are merged:
//   - foundation  → internal/config, internal/restic, internal/age, internal/logging
//   - choose-what-to-back-up → internal/assets
//   - set-up-vault → internal/vault
//
// The command structure, flag parsing, and output formatting are complete.
// The runBackup function currently returns a placeholder error; search for
// "TODO(wire)" to find the injection points.
package cli

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/kevinthelago/redoubt/internal/backup"
)

// NewBackupCmd returns the `redoubt backup` Cobra command.
func NewBackupCmd() *cobra.Command {
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "backup",
		Short: "Back up all tracked assets to the vault",
		Long: `Resolves the tracked asset set, dumps databases, age-seals secrets,
and pushes a single category-tagged snapshot to the append-only vault over LAN TLS.

Fail-soft: if one asset fails, the others still capture. The run exits non-zero
if any asset failed or if the backup step itself encountered an error.

The vault must be reachable on the LAN. The command fails immediately with a
clear error if the vault cannot be contacted — it never hangs waiting.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBackup(cmd, dryRun)
		},
		SilenceUsage: true,
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false,
		"preview the captured asset set without writing to the vault")

	return cmd
}

// runBackup executes the backup and prints the result.
func runBackup(cmd *cobra.Command, dryRun bool) error {
	ctx := cmd.Context()

	// TODO(wire): construct the pipeline from concrete foundation/assets/vault packages.
	//
	// When all dependency PRs are merged, replace the placeholder below with:
	//
	//   cfg, err := config.Load()
	//   if err != nil {
	//       return fmt.Errorf("load config: %w", err)
	//   }
	//   log := logging.New()
	//
	//   vaultProfile, err := vault.LoadProfile(cfg)
	//   if err != nil {
	//       return fmt.Errorf("load vault profile: %w", err)
	//   }
	//
	//   ks, err := keystore.Load(cfg)
	//   if err != nil {
	//       return fmt.Errorf("load keystore: %w", err)
	//   }
	//
	//   pipe := backup.New(backup.Config{
	//       Resolver: assets.NewResolver(cfg),
	//       Sealer:   age.NewSealer(ks),
	//       Runner:   restic.NewClient(vaultProfile),
	//       Log:      log,
	//       LockPath: config.LockPath(cfg),
	//   })
	//
	//   result, err := pipe.Run(ctx, dryRun)
	//   ...

	_ = ctx
	_ = dryRun
	return errors.New(
		"backup command not yet wired: waiting for foundation, choose-what-to-back-up, " +
			"and set-up-vault PRs to merge",
	)
}

// printBackupResult writes the backup summary to stdout (and failures to stderr).
// Called by runBackup once the wire-up is complete.
func printBackupResult(result *backup.RunResult) {
	if result.DryRun {
		printDryRunResult(result)
		return
	}

	if result.Stats != nil {
		fmt.Printf("Snapshot:   %s\n", result.Stats.SnapshotID)
		fmt.Printf("Files:      %d new / %d total\n", result.Stats.FilesNew, result.Stats.FilesTotal)
		fmt.Printf("Size:       %s added / %s total  (%.1fx dedup)\n",
			formatBytes(result.Stats.BytesAdded),
			formatBytes(result.Stats.BytesTotal),
			effectiveDedupRatio(result.Stats),
		)
		fmt.Printf("Duration:   %s\n", result.Duration.Round(time.Millisecond))
	}

	if result.HasErrors {
		printAssetFailures(result)
	}
}

func printDryRunResult(result *backup.RunResult) {
	fmt.Println("DRY RUN — the following would be backed up (no data written):")
	if len(result.PreviewPaths) == 0 {
		fmt.Println("  (no paths resolved)")
		return
	}
	for _, p := range result.PreviewPaths {
		fmt.Printf("  %s\n", p)
	}
	fmt.Printf("\n%d asset(s) tracked\n", len(result.Assets))
}

func printAssetFailures(result *backup.RunResult) {
	fmt.Fprintln(os.Stderr, "\nAsset failures:")
	for _, s := range result.Assets {
		if s.Err != nil {
			fmt.Fprintf(os.Stderr, "  [%s] %s: %s stage: %v\n",
				s.Asset.Category, s.Asset.ID, s.Stage, s.Err)
		}
	}
}

// backupExitCode returns 1 if there were any errors, 0 on full success.
// Used by runBackup to set the process exit code.
func backupExitCode(result *backup.RunResult) int {
	if result.HasErrors {
		return 1
	}
	return 0
}

func formatBytes(b int64) string {
	const (
		gib = 1 << 30
		mib = 1 << 20
		kib = 1 << 10
	)
	switch {
	case b >= gib:
		return fmt.Sprintf("%.1f GiB", float64(b)/gib)
	case b >= mib:
		return fmt.Sprintf("%.1f MiB", float64(b)/mib)
	case b >= kib:
		return fmt.Sprintf("%.1f KiB", float64(b)/kib)
	default:
		return fmt.Sprintf("%d B", b)
	}
}

func effectiveDedupRatio(stats *backup.BackupStats) float64 {
	if stats.BytesAdded == 0 {
		return 1.0
	}
	return float64(stats.BytesTotal) / float64(stats.BytesAdded)
}

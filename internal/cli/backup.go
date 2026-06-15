// Package cli provides the Cobra command implementations for the redoubt CLI.
// Each file in this package defines one command group.
//
// This file defines the `redoubt backup` command.
package cli

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/kevinthelago/redoubt/internal/assets"
	"github.com/kevinthelago/redoubt/internal/backup"
	"github.com/kevinthelago/redoubt/internal/config"
	"github.com/kevinthelago/redoubt/internal/logging"
	"github.com/kevinthelago/redoubt/internal/restic"
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

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if cfg.Vault.URL == "" {
		return errors.New("vault not configured: run 'redoubt vault set' to add a vault profile")
	}

	log := logging.New(os.Stderr, slog.LevelInfo)

	dataDir := config.DataDir()
	store := assets.NewStore(dataDir, nil)
	if err := store.Load(); err != nil {
		return fmt.Errorf("load tracked assets: %w", err)
	}

	backend := restic.Backend{
		Kind:   restic.BackendREST,
		URL:    cfg.Vault.URL,
		CACert: cfg.Vault.CACert,
	}
	runner := restic.New(backend)

	pipe := backup.New(backup.Config{
		Resolver: &backup.StoreResolver{Store: store},
		// TODO(vault): replace with a real age.Sealer once the vault stream
		// lands recipients. Until then, secrets assets fail-soft with a clear
		// error rather than being backed up unsealed.
		Sealer:   backup.ErrSealer{},
		Runner:   &backup.ResticAdapter{R: runner},
		Log:      log,
		LockPath: filepath.Join(dataDir, "backup.lock"),
		TempBase: os.TempDir(),
	})

	result, err := pipe.Run(ctx, dryRun)
	if err != nil {
		return err
	}

	printBackupResult(result)
	if result.HasErrors {
		os.Exit(backupExitCode(result))
	}
	return nil
}

// printBackupResult writes the backup summary to stdout (and failures to stderr).
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
func backupExitCode(result *backup.RunResult) int {
	if result.HasErrors {
		return 1
	}
	return 0
}

func effectiveDedupRatio(stats *backup.BackupStats) float64 {
	if stats.BytesAdded == 0 {
		return 1.0
	}
	return float64(stats.BytesTotal) / float64(stats.BytesAdded)
}

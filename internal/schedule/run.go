package schedule

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/kevinthelago/redoubt/internal/config"
	"github.com/kevinthelago/redoubt/internal/restic"
)

// IsVaultHost reports whether this machine is the designated vault host.
// Retention operations (restic forget --prune) are only permitted on vault
// hosts to prevent dev boxes from pruning backup history.
func IsVaultHost() bool {
	return os.Getenv("REDOUBT_IS_VAULT") == "1"
}

// backupRunner is the function used to execute a backup. Replaced in tests.
var backupRunner = runBackup

// Run executes a scheduled backup cycle: runs the backup, records the result,
// then applies retention on vault hosts.  The error returned is the backup
// exit error so the OS task scheduler marks the run as failed.
func Run(ctx context.Context) error {
	s := Status{StartedAt: time.Now().UTC()}

	backupErr := backupRunner(ctx)

	s.FinishedAt = time.Now().UTC()
	if backupErr != nil {
		s.ExitCode = 1
		s.Error = backupErr.Error()
	}

	if err := WriteStatus(s); err != nil {
		fmt.Fprintf(os.Stderr, "redoubt schedule: write status: %v\n", err)
	}

	if backupErr != nil {
		return backupErr
	}

	applyRetention(ctx)
	return nil
}

// runBackup invokes the backup subcommand of the current executable.
// Concurrent runs are prevented by the backup command's own file lock.
func runBackup(ctx context.Context) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable: %w", err)
	}
	cmd := exec.CommandContext(ctx, exe, "backup")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// applyRetention runs restic forget --prune using the saved retention config.
// Skipped silently on non-vault hosts so dev boxes cannot prune history.
func applyRetention(ctx context.Context) {
	if !IsVaultHost() {
		return
	}
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "redoubt schedule: retention: load config: %v\n", err)
		return
	}
	// Password read from env until keystore integration lands (TODO).
	password := os.Getenv("RESTIC_PASSWORD")
	if password == "" {
		fmt.Fprintln(os.Stderr, "redoubt schedule: retention skipped: RESTIC_PASSWORD not set")
		return
	}
	backend := restic.Backend{
		Kind:   restic.BackendREST,
		URL:    cfg.Vault.URL,
		CACert: cfg.Vault.CACert,
	}
	r := restic.New(backend, restic.WithPassword(password))
	policy := restic.ForgetPolicy{
		KeepHourly:  cfg.Retention.Hourly,
		KeepDaily:   cfg.Retention.Daily,
		KeepWeekly:  cfg.Retention.Weekly,
		KeepMonthly: cfg.Retention.Monthly,
	}
	if err := r.Forget(ctx, policy, true); err != nil {
		fmt.Fprintf(os.Stderr, "redoubt schedule: retention: %v\n", err)
	}
}

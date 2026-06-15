package health

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// ResticChecker can run an integrity check against a restic repository.
// This interface will be satisfied by the internal/restic package (F3) when it
// lands; until then the ExecChecker implementation below is used directly.
type ResticChecker interface {
	Check(ctx context.Context) error
}

// ExecChecker runs `restic check` by invoking the restic binary via exec.
// repo and password are passed via environment variables to avoid leaking them
// in the process command line.
type ExecChecker struct {
	// RepoURL is the restic repository URL (REST or local path).
	RepoURL string
	// Password is the restic repository password.
	Password string
	// CACert is the path to the CA cert for TLS verification (optional).
	CACert string
}

// Check runs `restic check` and returns an error if the repository has issues.
func (c *ExecChecker) Check(ctx context.Context) error {
	args := []string{"check"}
	if c.CACert != "" {
		args = append(args, "--cacert", c.CACert)
	}
	cmd := exec.CommandContext(ctx, "restic", args...)
	cmd.Env = append(cmd.Environ(),
		"RESTIC_REPOSITORY="+c.RepoURL,
		"RESTIC_PASSWORD="+c.Password,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("restic check: %w\n%s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// RunIntegrityCheck runs a restic check via checker, records the result in the
// state dir, and returns the IntegrityState. The data dir must be writable.
func RunIntegrityCheck(ctx context.Context, checker ResticChecker, reader *StateReader) (IntegrityState, error) {
	now := time.Now().UTC()
	state := IntegrityState{
		LastCheckAt: now,
		HasHistory:  true,
	}

	if err := checker.Check(ctx); err != nil {
		state.Errors = 1
		state.Message = err.Error()
		// Persist even on failure so we know the check ran.
		_ = reader.WriteIntegrity(state)
		return state, nil
	}

	if err := reader.WriteIntegrity(state); err != nil {
		return state, fmt.Errorf("writing integrity state: %w", err)
	}
	return state, nil
}

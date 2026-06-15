package cold

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// ErrDriveNotMounted is returned when a named drive is registered but its
// mount path is not accessible. No repository mutation is performed.
type ErrDriveNotMounted struct {
	Label     string
	MountPath string
}

func (e *ErrDriveNotMounted) Error() string {
	return fmt.Sprintf("cold copy: drive %q not mounted (expected at %s)", e.Label, e.MountPath)
}

// ErrNoDriveMounted is returned when auto-detection finds no registered drive
// is currently mounted.
type ErrNoDriveMounted struct{}

func (e *ErrNoDriveMounted) Error() string {
	return "cold copy: no registered cold drive is currently mounted"
}

// ErrNoDriveRegistered is returned when Run is called but no drives have been
// registered yet.
type ErrNoDriveRegistered struct{}

func (e *ErrNoDriveRegistered) Error() string {
	return "cold copy: no drives registered — run 'redoubt cold-copy register' first"
}

// RunOptions controls a single cold-copy run.
type RunOptions struct {
	// DriveLabel selects a specific registered drive. If empty, the engine
	// auto-selects the mounted drive with the oldest last-copy time
	// (promoting rotation across multiple drives).
	DriveLabel string

	// Progress receives human-readable status messages as the run proceeds.
	// It may be called from any goroutine but is called sequentially.
	// A nil Progress silently discards messages.
	Progress func(string)

	// SkipRetention disables the forget/prune step after a successful copy.
	// Useful when disk space on the cold drive is not a concern or when
	// testing.
	SkipRetention bool

	// RetentionOverride, if non-nil, replaces the policy in ColdConfig.
	RetentionOverride *RetentionPolicy
}

// RunResult describes the outcome of a successful cold-copy run.
type RunResult struct {
	// Drive is the registry record of the drive that was used.
	Drive DriveRecord

	// Duration is the wall-clock time from drive-detect to verify-complete.
	Duration time.Duration

	// Initialised is true when the cold repository was created during this
	// run (first-time use of this drive).
	Initialised bool
}

// Engine orchestrates cold-copy operations.
// Construct one with NewEngine; do not copy after first use.
type Engine struct {
	backend  Backend
	config   ConfigProvider
	registry *Registry
	mu       sync.Mutex // one cold copy at a time
}

// NewEngine constructs an Engine, loading (or creating) the drive registry
// at the path returned by cfg.DriveRegistryPath().
func NewEngine(backend Backend, cfg ConfigProvider) (*Engine, error) {
	reg, err := LoadRegistry(cfg.DriveRegistryPath())
	if err != nil {
		return nil, fmt.Errorf("cold engine: load registry: %w", err)
	}
	return &Engine{
		backend:  backend,
		config:   cfg,
		registry: reg,
	}, nil
}

func (e *Engine) progress(opts RunOptions, msg string) {
	if opts.Progress != nil {
		opts.Progress(msg)
	}
}

// Run performs a complete cold-copy cycle:
//  1. Selects the target drive (by label or auto-rotation).
//  2. Initialises the cold repository on first use.
//  3. Copies new snapshots from the vault (idempotent).
//  4. Verifies repository integrity.
//  5. Optionally applies the cold retention policy.
//  6. Updates the drive registry.
//
// If the target drive is not mounted, Run returns [*ErrDriveNotMounted] or
// [*ErrNoDriveMounted] without modifying any repository state.
func (e *Engine) Run(ctx context.Context, opts RunOptions) (*RunResult, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Always reload from disk so we see external registry updates.
	if err := e.registry.Reload(); err != nil {
		return nil, fmt.Errorf("cold engine: reload registry: %w", err)
	}

	// Ensure at least one drive is registered before doing anything.
	if len(e.registry.List()) == 0 {
		return nil, &ErrNoDriveRegistered{}
	}

	start := time.Now()

	// 1. Select the target drive.
	var drive DriveRecord
	if opts.DriveLabel != "" {
		d, ok := e.registry.Get(opts.DriveLabel)
		if !ok {
			return nil, fmt.Errorf("cold engine: drive %q not registered", opts.DriveLabel)
		}
		if !d.IsMounted() {
			return nil, &ErrDriveNotMounted{Label: d.Label, MountPath: d.MountPath}
		}
		drive = d
	} else {
		d, ok := e.registry.BestCandidate()
		if !ok {
			return nil, &ErrNoDriveMounted{}
		}
		drive = d
	}

	repoPath := drive.RepoPath()
	e.progress(opts, fmt.Sprintf("Using cold drive: %s (%s)", drive.Label, repoPath))

	// 2. Initialise the repository on first use.
	initialised := false
	ready, err := e.backend.IsLocalRepoReady(ctx, repoPath)
	if err != nil {
		return nil, fmt.Errorf("cold engine: check repo %s: %w", repoPath, err)
	}
	if !ready {
		e.progress(opts, "Initialising cold repository (first use)…")
		if err := e.backend.InitLocalRepo(ctx, repoPath); err != nil {
			return nil, fmt.Errorf("cold engine: init repo %s: %w", repoPath, err)
		}
		initialised = true
		e.progress(opts, "Cold repository initialised.")
	}

	// 3. Copy new snapshots from vault → cold drive.
	e.progress(opts, "Copying snapshots from vault to cold drive…")
	if err := e.backend.CopySnapshotsTo(ctx, repoPath); err != nil {
		return nil, fmt.Errorf("cold engine: copy snapshots to %s: %w", repoPath, err)
	}
	e.progress(opts, "Snapshot copy complete.")

	// 4. Verify repository integrity.
	e.progress(opts, "Verifying cold repository integrity…")
	if err := e.backend.CheckLocalRepo(ctx, repoPath); err != nil {
		return nil, fmt.Errorf("cold engine: check failed for %s: %w", repoPath, err)
	}
	e.progress(opts, "Integrity check passed.")

	// 5. Apply retention policy (unless skipped).
	if !opts.SkipRetention {
		policy := e.config.ColdConfig().Retention
		if opts.RetentionOverride != nil {
			policy = *opts.RetentionOverride
		}
		if retentionActive(policy) {
			e.progress(opts, "Applying cold-drive retention policy…")
			if err := e.backend.ForgetInLocalRepo(ctx, repoPath, policy); err != nil {
				// Non-fatal: the copy and verify succeeded. Log and continue.
				e.progress(opts, fmt.Sprintf("Warning: retention policy failed: %v", err))
			}
		}
	}

	// 6. Update registry: record copy and verify times.
	now := time.Now()
	if err := e.registry.RecordCopy(drive.Label, now); err != nil {
		return nil, fmt.Errorf("cold engine: update registry: %w", err)
	}
	if err := e.registry.RecordVerify(drive.Label, now); err != nil {
		// Non-fatal; copy succeeded.
		e.progress(opts, fmt.Sprintf("Warning: could not record verify time: %v", err))
	}

	result := &RunResult{
		Drive:       drive,
		Duration:    time.Since(start),
		Initialised: initialised,
	}
	e.progress(opts, fmt.Sprintf(
		"Cold copy complete in %s. Safe to disconnect: %s",
		result.Duration.Round(time.Second),
		drive.Label,
	))
	return result, nil
}

// Register adds a new drive to the registry. Returns an error if a drive with
// the same label is already registered.
func (e *Engine) Register(d DriveRecord) error {
	if d.Label == "" {
		return fmt.Errorf("cold engine: drive label must not be empty")
	}
	if d.MountPath == "" {
		return fmt.Errorf("cold engine: mount path must not be empty for drive %q", d.Label)
	}
	return e.registry.Add(d)
}

// Remove deletes the drive with the given label from the registry.
func (e *Engine) Remove(label string) error {
	return e.registry.Remove(label)
}

// Status returns the current health status of every registered drive.
func (e *Engine) Status() []DriveStatus {
	if err := e.registry.Reload(); err != nil {
		// Best-effort: return what we have.
		_ = err
	}
	cfg := e.config.ColdConfig()
	drives := e.registry.List()
	out := make([]DriveStatus, len(drives))
	for i, d := range drives {
		out[i] = DriveStatus{
			Drive:     d,
			Mounted:   d.IsMounted(),
			Staleness: computeStaleness(d.LastCopyTime, cfg),
		}
	}
	return out
}

// List returns all registered drives, sorted by label.
func (e *Engine) List() []DriveRecord {
	if err := e.registry.Reload(); err != nil {
		_ = err
	}
	return e.registry.List()
}

// retentionActive returns true if any field in p is non-zero (i.e. some
// retention is actually configured).
func retentionActive(p RetentionPolicy) bool {
	return p.KeepLast > 0 || p.KeepDaily > 0 || p.KeepWeekly > 0 ||
		p.KeepMonthly > 0 || p.KeepYearly > 0
}

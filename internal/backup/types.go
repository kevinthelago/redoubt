// Package backup implements the core backup pipeline: tracked-asset resolution,
// DB dumps, age-sealing of secrets, and a single category-tagged restic snapshot
// pushed to the append-only vault over LAN TLS.
//
// All dependencies are injected via interfaces so the pipeline is independently
// testable without the concrete foundation/assets/vault packages.
package backup

import (
	"context"
	"errors"
	"time"
)

// Sentinel errors returned by [Pipeline.Run].
var (
	// ErrEmptyTrackedSet is returned when the tracked set has no assets.
	ErrEmptyTrackedSet = errors.New("tracked set is empty: run 'redoubt track add' to declare assets")

	// ErrVaultUnreachable is returned when the vault cannot be contacted before
	// the backup starts. It wraps the underlying network error.
	ErrVaultUnreachable = errors.New("vault unreachable")

	// ErrLocked is returned when another backup process is already running.
	ErrLocked = errors.New("backup already running")
)

// Category identifies how an asset is captured.
type Category string

const (
	CategorySource   Category = "source"
	CategorySecrets  Category = "secrets"
	CategoryDatabase Category = "database"
	CategoryConfig   Category = "config"
)

// TrackedAsset is a resolved entry from the tracked-set, ready for the pipeline.
// The [AssetResolver] implementation (in internal/assets) converts config entries
// into this form before handing them to the pipeline.
type TrackedAsset struct {
	// ID is the stable identifier used in logs and the run result.
	ID string
	// Category determines how the asset is processed.
	Category Category
	// Paths are the concrete file or directory paths to include.
	// For CategoryDatabase this is informational; DumpCmd produces the actual artifact.
	Paths []string
	// Excludes are gitignore-style patterns applied when walking Paths.
	Excludes []string
	// DumpCmd is the command used to produce a consistent snapshot of a database asset.
	// Element 0 is the executable; the rest are arguments.
	// Non-nil only for CategoryDatabase.
	DumpCmd []string
}

// AssetResolver resolves the current tracked set into pipeline-ready assets.
// Satisfied by internal/assets.Resolver.
type AssetResolver interface {
	Resolve(ctx context.Context) ([]TrackedAsset, error)
}

// Sealer age-seals a single file from src to dst.
// The caller is responsible for creating the dst directory.
// Satisfied by internal/age.Sealer.
type Sealer interface {
	SealFile(ctx context.Context, src, dst string) error
}

// Runner abstracts the restic backup engine and vault connectivity check.
// Satisfied by an adapter in internal/restic.
type Runner interface {
	// Check verifies the vault is reachable. Returns a typed error on failure.
	// Must return quickly (short timeout) to satisfy the "fail fast, no hang" requirement.
	Check(ctx context.Context) error

	// Backup runs a restic backup with the given options and returns the snapshot result.
	Backup(ctx context.Context, opts RunOptions) (*BackupStats, error)

	// Preview returns the paths that would be included, without writing anything.
	// Used for --dry-run.
	Preview(ctx context.Context, opts RunOptions) ([]string, error)
}

// RunOptions specifies what to back up in a single restic invocation.
type RunOptions struct {
	// Sources are the file/directory paths to include in the snapshot.
	Sources []string
	// Excludes are gitignore-style patterns to exclude from Sources.
	Excludes []string
	// Tags are attached to the resulting snapshot for filtering.
	Tags []string
}

// BackupStats summarises the completed restic backup.
type BackupStats struct {
	SnapshotID string
	FilesNew   int64
	FilesTotal int64
	BytesAdded int64
	BytesTotal int64
	// DedupRatio is BytesTotal / BytesAdded, or 1.0 if BytesAdded == 0.
	DedupRatio float64
}

// Logger is the structured logging interface satisfied by internal/logging.
// All calls use slog-style key-value pairs in args.
type Logger interface {
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
	Debug(msg string, args ...any)
}

// AssetStatus records the outcome of processing one tracked asset.
type AssetStatus struct {
	Asset TrackedAsset
	// Err is nil on success.
	Err error
	// Stage is where the error occurred: "dump", "seal", or "backup".
	Stage string
}

// RunResult summarises a completed pipeline run.
type RunResult struct {
	// Assets holds per-asset outcomes; present even when some assets failed.
	Assets []AssetStatus
	// Stats is the restic summary for a successful, non-dry-run backup.
	// Nil on dry-run or when the backup step itself fails.
	Stats *BackupStats
	// DryRun is true when the run was a preview with no side effects.
	DryRun bool
	// PreviewPaths is populated on a dry-run with the paths that would be backed up.
	PreviewPaths []string
	// Duration is the wall-clock time for the run.
	Duration time.Duration
	// HasErrors is true when at least one asset failed.
	HasErrors bool
}

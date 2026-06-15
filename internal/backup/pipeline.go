package backup

import (
	"context"
	"fmt"
	"os"
	"time"
)

// Pipeline orchestrates the dump → seal → push backup flow.
//
// Concurrency contract: a single pipeline may only run one backup at a time;
// the file lock at [Config.LockPath] prevents concurrent runs across processes.
type Pipeline struct {
	resolver AssetResolver
	sealer   Sealer
	runner   Runner
	log      Logger
	lockPath string
	tempBase string
}

// Config holds the injectable dependencies for a [Pipeline].
type Config struct {
	Resolver AssetResolver
	Sealer   Sealer
	Runner   Runner
	Log      Logger
	// LockPath is the exclusive file lock used to prevent concurrent backups.
	// Should live in the platform config dir.
	LockPath string
	// TempBase is the parent directory for temporary artifacts (DB dumps, sealed
	// secrets). Defaults to os.TempDir() when empty.
	TempBase string
}

// New creates a Pipeline from the supplied dependencies.
func New(cfg Config) *Pipeline {
	tb := cfg.TempBase
	if tb == "" {
		tb = os.TempDir()
	}
	return &Pipeline{
		resolver: cfg.Resolver,
		sealer:   cfg.Sealer,
		runner:   cfg.Runner,
		log:      cfg.Log,
		lockPath: cfg.LockPath,
		tempBase: tb,
	}
}

// Run executes the backup pipeline.
//
// When dryRun is true the pipeline resolves the tracked set and previews
// what would be captured without acquiring the lock, dumping, sealing, or
// writing to the vault.
//
// Fail-soft: asset-level errors (dump failures, seal failures) are collected
// in RunResult.Assets and do not abort the backup for the remaining assets.
// A non-nil top-level error from Run indicates a fatal pre-flight failure
// (vault unreachable, lock contention, empty tracked set, etc.).
// RunResult.HasErrors is true when at least one asset failed.
func (p *Pipeline) Run(ctx context.Context, dryRun bool) (*RunResult, error) {
	start := time.Now()

	// ── Pre-flight: vault connectivity ─────────────────────────────────────
	// Checked before the lock so the caller gets a fast, clear failure without
	// holding the lock or blocking a real backup.
	if !dryRun {
		p.log.Info("checking vault connectivity")
		if err := p.runner.Check(ctx); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrVaultUnreachable, err)
		}
	}

	// ── Pre-flight: exclusive run lock ────────────────────────────────────
	if !dryRun {
		release, err := acquireLock(p.lockPath)
		if err != nil {
			return nil, err // already ErrLocked or a wrapped fs error
		}
		defer release()
	}

	// ── Resolve tracked set ───────────────────────────────────────────────
	p.log.Info("resolving tracked asset set")
	assets, err := p.resolver.Resolve(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolve tracked set: %w", err)
	}
	if len(assets) == 0 {
		return nil, ErrEmptyTrackedSet
	}

	if dryRun {
		return p.runDryRun(ctx, assets, start)
	}

	return p.runReal(ctx, assets, start)
}

// runReal executes the full dump → seal → restic-backup flow.
func (p *Pipeline) runReal(ctx context.Context, assets []TrackedAsset, start time.Time) (*RunResult, error) {
	// ── Temp dir for dumps and sealed secrets ─────────────────────────────
	tmpDir, err := os.MkdirTemp(p.tempBase, "redoubt-backup-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}
	defer func() {
		p.log.Debug("wiping temp artifacts", "dir", tmpDir)
		if werr := wipeTempDir(tmpDir); werr != nil {
			p.log.Warn("temp wipe incomplete; manual cleanup may be needed",
				"dir", tmpDir, "err", werr)
		}
	}()

	// ── Prepare each asset: dump DBs, seal secrets ────────────────────────
	sources, excludes, statuses := p.prepareAssets(ctx, assets, tmpDir)

	// ── Build category tags ───────────────────────────────────────────────
	tags := buildTags(assets)

	// ── Run restic backup ─────────────────────────────────────────────────
	p.log.Info("starting restic backup", "sources", len(sources), "tags", tags)
	stats, backupErr := p.runner.Backup(ctx, RunOptions{
		Sources:  sources,
		Excludes: excludes,
		Tags:     tags,
	})

	result := &RunResult{
		Assets:   statuses,
		Duration: time.Since(start),
	}
	for _, s := range statuses {
		if s.Err != nil {
			result.HasErrors = true
			break
		}
	}

	if backupErr != nil {
		// The restic step itself failed; wrap and return.
		// The deferred wipe still runs.
		return result, fmt.Errorf("restic backup: %w", backupErr)
	}

	result.Stats = stats
	return result, nil
}

// prepareAssets processes each asset for backup:
//   - database: runs the dump command, writing to a temp file
//   - secrets: age-seals all files into a mirrored temp directory
//   - source/config: added directly with their exclude patterns
//
// Failures are collected per-asset (fail-soft); the function always returns.
func (p *Pipeline) prepareAssets(
	ctx context.Context,
	assets []TrackedAsset,
	tmpDir string,
) (sources, excludes []string, statuses []AssetStatus) {
	statuses = make([]AssetStatus, 0, len(assets))

	for _, a := range assets {
		s := AssetStatus{Asset: a}

		switch a.Category {
		case CategoryDatabase:
			path, err := dumpDB(ctx, a, tmpDir, p.log)
			if err != nil {
				p.log.Error("DB dump failed; skipping asset", "asset", a.ID, "err", err)
				s.Err = err
				s.Stage = "dump"
				statuses = append(statuses, s)
				continue // fail-soft: other assets still proceed
			}
			sources = append(sources, path)

		case CategorySecrets:
			sealedDir, err := sealSecretsAsset(ctx, a, tmpDir, p.sealer, p.log)
			if err != nil {
				p.log.Error("seal failed; skipping asset", "asset", a.ID, "err", err)
				s.Err = err
				s.Stage = "seal"
				statuses = append(statuses, s)
				continue // fail-soft
			}
			// Back up the sealed dir, not the original paths.
			sources = append(sources, sealedDir)

		case CategorySource, CategoryConfig:
			sources = append(sources, a.Paths...)
			excludes = append(excludes, a.Excludes...)
		}

		statuses = append(statuses, s)
	}

	return sources, excludes, statuses
}

// runDryRun previews the backup without acquiring the lock or performing any I/O.
func (p *Pipeline) runDryRun(ctx context.Context, assets []TrackedAsset, start time.Time) (*RunResult, error) {
	var previewSources, previewExcludes []string
	statuses := make([]AssetStatus, 0, len(assets))

	for _, a := range assets {
		statuses = append(statuses, AssetStatus{Asset: a})
		switch a.Category {
		case CategoryDatabase:
			previewSources = append(previewSources, fmt.Sprintf("<db-dump:%s cmd=%v>", a.ID, a.DumpCmd))
		case CategorySecrets:
			for _, path := range a.Paths {
				previewSources = append(previewSources, fmt.Sprintf("<sealed:%s>", path))
			}
		case CategorySource, CategoryConfig:
			previewSources = append(previewSources, a.Paths...)
			previewExcludes = append(previewExcludes, a.Excludes...)
		}
	}

	// Ask the runner to expand the source list (e.g. walk directories, apply excludes).
	// Fall back to the raw source list if preview is not supported.
	preview, err := p.runner.Preview(ctx, RunOptions{
		Sources:  previewSources,
		Excludes: previewExcludes,
	})
	if err != nil || len(preview) == 0 {
		preview = previewSources
	}

	return &RunResult{
		Assets:       statuses,
		DryRun:       true,
		PreviewPaths: preview,
		Duration:     time.Since(start),
	}, nil
}

// buildTags returns the restic snapshot tags for this run.
// Tags always include "redoubt" plus one "category=X" tag per unique category present.
func buildTags(assets []TrackedAsset) []string {
	seen := map[Category]bool{}
	tags := []string{"redoubt"}
	for _, a := range assets {
		if !seen[a.Category] {
			seen[a.Category] = true
			tags = append(tags, "category="+string(a.Category))
		}
	}
	return tags
}

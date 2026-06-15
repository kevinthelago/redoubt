// Package drill implements the restore-drill engine.
//
// A drill restores a rotating file sample plus a fixed canary from a restic
// snapshot into an isolated scratch directory, verifies their SHA-256 hashes
// against a manifest stored in the snapshot, records the result, alerts on
// failure, and then securely wipes the scratch directory.
//
// The drill never touches live data and always exits non-zero on failure.
package drill

import (
	"context"
	"encoding/binary"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

const (
	// DefaultMaxSampleFiles is the default number of files in the rotating sample.
	DefaultMaxSampleFiles = 10
	// DefaultMinFreeSpaceBytes requires at least 500 MiB before starting.
	DefaultMinFreeSpaceBytes uint64 = 500 * 1024 * 1024
	// DefaultHistoryMaxLen is the maximum number of results kept in history.
	DefaultHistoryMaxLen = 100
)

// Restorer is the interface the drill engine uses to access snapshots.
// The restore stream (R1) provides the concrete implementation.
type Restorer interface {
	// LatestSnapshotID returns the most recent snapshot ID for src.
	// Returns ("", false, nil) when no snapshots exist.
	LatestSnapshotID(ctx context.Context, src Source) (string, bool, error)

	// ListSnapshotFiles returns paths of files in snapshotID whose path starts
	// with prefix. Pass an empty prefix to list all files.
	ListSnapshotFiles(ctx context.Context, src Source, snapshotID, prefix string) ([]string, error)

	// RestoreFiles copies the named paths from snapshotID into targetDir.
	// targetDir is an empty, isolated directory created by the engine.
	RestoreFiles(ctx context.Context, src Source, snapshotID string, paths []string, targetDir string) error

	// FreeSpaceAt returns available bytes on the volume at path.
	FreeSpaceAt(path string) (uint64, error)
}

// Alerter is notified when a drill fails so the health stream can react.
type Alerter interface {
	DrillFailed(result Result)
}

// Config holds all tunable parameters for the drill engine.
type Config struct {
	ScratchBaseDir    string
	HistoryPath       string
	StatusPath        string // written after each run for health monitoring
	MaxSampleFiles    int
	MinFreeSpaceBytes uint64
	HistoryMaxLen     int
}

func (c *Config) applyDefaults() {
	if c.MaxSampleFiles <= 0 {
		c.MaxSampleFiles = DefaultMaxSampleFiles
	}
	if c.MinFreeSpaceBytes == 0 {
		c.MinFreeSpaceBytes = DefaultMinFreeSpaceBytes
	}
	if c.HistoryMaxLen <= 0 {
		c.HistoryMaxLen = DefaultHistoryMaxLen
	}
	if c.ScratchBaseDir == "" {
		c.ScratchBaseDir = filepath.Join(os.TempDir(), "redoubt-drill")
	}
}

// Engine executes restore drills.
type Engine struct {
	cfg      Config
	restorer Restorer
	alerter  Alerter
	log      *slog.Logger
}

// NewEngine returns a configured Engine.
func NewEngine(cfg Config, restorer Restorer, alerter Alerter, log *slog.Logger) *Engine {
	cfg.applyDefaults()
	if log == nil {
		log = slog.Default()
	}
	return &Engine{cfg: cfg, restorer: restorer, alerter: alerter, log: log}
}

// RunOptions controls a single drill execution.
type RunOptions struct {
	Source Source
	// Force bypasses any cadence check (used by "run-now").
	Force bool
}

// Run executes one restore drill.
//
// The result is always written to history and the health summary file.
// On StatusFail the Alerter is called and the returned error is nil — the
// failure is captured in result.Status and result.FailureMsg. A non-nil
// returned error means the drill could not run at all (infrastructure fault).
func (e *Engine) Run(ctx context.Context, opts RunOptions) (Result, error) {
	start := time.Now()
	result := Result{Timestamp: start, Source: opts.Source}

	// Resolve the latest snapshot.
	snapshotID, found, err := e.restorer.LatestSnapshotID(ctx, opts.Source)
	if err != nil {
		return result, fmt.Errorf("resolve latest snapshot for %q: %w", opts.Source, err)
	}
	if !found {
		result.Status = StatusSkip
		result.SkipReason = fmt.Sprintf("no snapshots found for source %q", opts.Source)
		result.DurationMS = time.Since(start).Milliseconds()
		e.log.Info("drill skipped: no snapshots", "source", opts.Source)
		e.persistResult(result)
		return result, nil
	}
	result.SnapshotID = snapshotID

	// Prepare the scratch directory, checking free space first.
	scratch, skipped, err := e.prepareScratch()
	if err != nil {
		if skipped {
			result.Status = StatusSkip
			result.SkipReason = err.Error()
			result.DurationMS = time.Since(start).Milliseconds()
			e.persistResult(result)
			return result, nil
		}
		return result, fmt.Errorf("prepare scratch: %w", err)
	}
	// Always wipe on exit, even if we return early due to an error below.
	defer func() {
		if wipeErr := SecureWipe(scratch); wipeErr != nil {
			e.log.Warn("scratch wipe error (contents may remain)",
				"path", scratch, "err", wipeErr)
		}
	}()

	// Select: canary + rotating sample.
	paths, err := e.selectPaths(ctx, opts.Source, snapshotID)
	if err != nil {
		return result, fmt.Errorf("select files from snapshot %q: %w", snapshotID, err)
	}
	if len(paths) == 0 {
		result.Status = StatusSkip
		result.SkipReason = "snapshot contains no drillable files (no canary, no regular files)"
		result.DurationMS = time.Since(start).Milliseconds()
		e.persistResult(result)
		return result, nil
	}

	// Restore the selected files.
	e.log.Info("restoring drill sample",
		"source", opts.Source, "snapshot", snapshotID, "files", len(paths))
	if err := e.restorer.RestoreFiles(ctx, opts.Source, snapshotID, paths, scratch); err != nil {
		result.Status = StatusFail
		result.FailureMsg = fmt.Sprintf("restore failed: %v", err)
		result.DurationMS = time.Since(start).Milliseconds()
		e.log.Error("drill restore failed", "err", err)
		e.persistResult(result)
		if e.alerter != nil {
			e.alerter.DrillFailed(result)
		}
		return result, nil
	}
	result.Restored = len(paths)

	// Load the manifest from the restored snapshot and verify only restored paths.
	verified, failures, err := e.verify(scratch, paths)
	if err != nil {
		return result, fmt.Errorf("manifest verification setup: %w", err)
	}
	result.Verified = verified
	result.Failed = len(failures)

	if len(failures) > 0 {
		result.Status = StatusFail
		result.FailureMsg = fmt.Sprintf("%d SHA-256 mismatch(es): %v", len(failures), failures)
		result.DurationMS = time.Since(start).Milliseconds()
		e.log.Error("drill verification failed", "failures", failures)
		e.persistResult(result)
		if e.alerter != nil {
			e.alerter.DrillFailed(result)
		}
		return result, nil
	}

	result.Status = StatusPass
	result.DurationMS = time.Since(start).Milliseconds()
	e.log.Info("drill passed",
		"source", opts.Source, "verified", verified, "duration_ms", result.DurationMS)
	e.persistResult(result)
	return result, nil
}

// prepareScratch checks free space and creates the scratch directory.
// Returns (dir, skipped=false, nil) on success.
// Returns ("", skipped=true, err) when low space triggers a skip.
// Returns ("", skipped=false, err) on other infrastructure errors.
func (e *Engine) prepareScratch() (dir string, skipped bool, err error) {
	if mkErr := os.MkdirAll(e.cfg.ScratchBaseDir, 0700); mkErr != nil {
		return "", false, fmt.Errorf("create scratch base %q: %w", e.cfg.ScratchBaseDir, mkErr)
	}
	free, spaceErr := e.restorer.FreeSpaceAt(e.cfg.ScratchBaseDir)
	if spaceErr != nil {
		return "", false, fmt.Errorf("check free space at %q: %w", e.cfg.ScratchBaseDir, spaceErr)
	}
	if free < e.cfg.MinFreeSpaceBytes {
		return "", true, fmt.Errorf(
			"insufficient free space at %q: %s available, need %s",
			e.cfg.ScratchBaseDir, formatBytes(free), formatBytes(e.cfg.MinFreeSpaceBytes),
		)
	}
	dir, err = CreateScratch(e.cfg.ScratchBaseDir)
	return dir, false, err
}

// selectPaths picks the canary plus a rotating sample from the snapshot.
func (e *Engine) selectPaths(ctx context.Context, src Source, snapshotID string) ([]string, error) {
	all, err := e.restorer.ListSnapshotFiles(ctx, src, snapshotID, "")
	if err != nil {
		return nil, err
	}

	var canaryPaths []string
	var rest []string
	for _, p := range all {
		if filepath.Base(p) == CanaryFileName || filepath.Base(p) == ManifestFileName {
			canaryPaths = append(canaryPaths, p)
		} else {
			rest = append(rest, p)
		}
	}

	selected := append([]string{}, canaryPaths...)

	sampleQuota := e.cfg.MaxSampleFiles - len(selected)
	if sampleQuota > 0 && len(rest) > 0 {
		// Rotate using the snapshot ID as a deterministic seed so the sample
		// changes each drill without being random.
		offset := snapshotOffset(snapshotID) % len(rest)
		for i := 0; i < sampleQuota && i < len(rest); i++ {
			selected = append(selected, rest[(offset+i)%len(rest)])
		}
	}

	return selected, nil
}

// verify loads the snapshot manifest from scratch and checks only the restored paths.
func (e *Engine) verify(scratch string, restoredPaths []string) (verified int, failures []string, err error) {
	m, err := LoadManifestFromDir(scratch)
	if err != nil {
		return 0, nil, fmt.Errorf("load snapshot manifest: %w", err)
	}
	verified, failures = m.VerifyFiles(scratch, restoredPaths)
	return verified, failures, nil
}

func (e *Engine) persistResult(r Result) {
	if e.cfg.HistoryPath != "" {
		h, loadErr := LoadHistory(e.cfg.HistoryPath)
		if loadErr != nil {
			e.log.Warn("load drill history", "err", loadErr)
			h = &History{}
		}
		h.Append(r, e.cfg.HistoryMaxLen)
		if saveErr := h.Save(e.cfg.HistoryPath); saveErr != nil {
			e.log.Warn("save drill history", "err", saveErr)
		}
		if e.cfg.StatusPath != "" {
			if sumErr := h.WriteSummary(e.cfg.StatusPath); sumErr != nil {
				e.log.Warn("write drill status", "err", sumErr)
			}
		}
	}
}

// snapshotOffset maps a snapshot ID string to a non-negative integer for
// rotating the sample selection.
func snapshotOffset(id string) int {
	b := make([]byte, 8)
	copy(b, []byte(id))
	v := int64(binary.LittleEndian.Uint64(b))
	if v < 0 {
		v = -v
	}
	return int(v)
}

func formatBytes(b uint64) string {
	const (
		MiB = 1024 * 1024
		GiB = 1024 * MiB
	)
	switch {
	case b >= GiB:
		return fmt.Sprintf("%.1f GiB", float64(b)/float64(GiB))
	case b >= MiB:
		return fmt.Sprintf("%.1f MiB", float64(b)/float64(MiB))
	default:
		return fmt.Sprintf("%d B", b)
	}
}

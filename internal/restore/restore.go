package restore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	goage "filippo.io/age"
)

// Pipeline is the core restore engine. Constructed with dependency-injected
// implementations of ResticBackend, FileUnsealer, KeyProvider, EscrowProvider,
// and SnapshotBrowser — so every layer is testable in isolation.
type Pipeline struct {
	backend  ResticBackend
	unsealer FileUnsealer
	keys     KeyProvider
	escrow   EscrowProvider
	browser  SnapshotBrowser
	log      *slog.Logger
	in       io.Reader // interactive input (share entry, confirmations)
	out      io.Writer // user-facing output
}

// NewPipeline constructs a Pipeline. in and out default to os.Stdin/os.Stdout
// if nil.
func NewPipeline(
	backend ResticBackend,
	unsealer FileUnsealer,
	keys KeyProvider,
	escrow EscrowProvider,
	browser SnapshotBrowser,
	log *slog.Logger,
	in io.Reader,
	out io.Writer,
) *Pipeline {
	if in == nil {
		in = os.Stdin
	}
	if out == nil {
		out = os.Stdout
	}
	return &Pipeline{
		backend:  backend,
		unsealer: unsealer,
		keys:     keys,
		escrow:   escrow,
		browser:  browser,
		log:      log,
		in:       in,
		out:      out,
	}
}

// Run executes a restore with the given options. It is safe to call multiple
// times (each call is independent). The restore is idempotent: restic skips
// files it has already restored correctly.
func (p *Pipeline) Run(ctx context.Context, opts Options) (*Result, error) {
	log := p.log.With("snapshot", opts.SnapshotID, "target", opts.TargetDir)

	// ── 1. Resolve snapshot ──────────────────────────────────────────────────
	snap, err := p.resolveSnapshot(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("resolve snapshot: %w", err)
	}
	log = log.With("snapshot_id", snap.ID, "snapshot_time", snap.Time)
	log.Info("resolved snapshot")

	// ── 2. Resolve age identity (keystore or break-glass) ───────────────────
	identity, usedBG, err := p.resolveIdentity(ctx, opts)
	if err != nil {
		return nil, err
	}
	if usedBG {
		log.Info("identity reconstructed via Shamir shares (break-glass)")
		fmt.Fprintln(p.out, "Key reconstructed from shares. Proceeding with restore.")
	}

	// ── 3. Build file list for pre-flight ───────────────────────────────────
	fileList, err := p.backend.ListFiles(ctx, snap.ID, opts.Includes)
	if err != nil {
		// Non-fatal: proceed without space estimate if listing fails.
		log.Warn("could not list snapshot files; skipping pre-flight space check", "err", err)
	}

	// ── 4. Pre-flight ───────────────────────────────────────────────────────
	if err := preflight(opts.TargetDir, opts.Overwrite, fileList); err != nil {
		return nil, err
	}

	// ── 5. Dry-run: report and exit ─────────────────────────────────────────
	if opts.DryRun {
		return dryRunResult(snap, fileList, usedBG), nil
	}

	// ── 6. Restore ──────────────────────────────────────────────────────────
	log.Info("starting restore", "files", len(fileList), "target", opts.TargetDir)
	fmt.Fprintf(p.out, "Restoring snapshot %s → %s\n", snap.ShortID, opts.TargetDir)
	// NoVerify=false → restic does its own --verify after restore.
	if err := p.backend.Restore(ctx, snap.ID, opts.TargetDir, opts.Includes, !opts.NoVerify); err != nil {
		return nil, fmt.Errorf("restic restore: %w", err)
	}
	log.Info("restic restore complete")

	// ── 7. Age-unseal secrets ────────────────────────────────────────────────
	sealed, err := p.unsealTree(ctx, opts.TargetDir, identity)
	if err != nil {
		return nil, fmt.Errorf("unseal secrets: %w", err)
	}
	log.Info("unsealed secrets", "count", sealed)

	// ── 8. Post-restore verification ────────────────────────────────────────
	var verifyErrs []VerifyError
	verified := false
	if !opts.NoVerify {
		verifyErrs, err = p.verify(opts.TargetDir, fileList, identity)
		if err != nil {
			return nil, fmt.Errorf("verify: %w", err)
		}
		verified = len(verifyErrs) == 0
		if verified {
			fmt.Fprintln(p.out, "✓ Post-restore verification: all files match snapshot hashes.")
		} else {
			fmt.Fprintf(p.out, "✗ VERIFICATION MISMATCH: %d file(s) did not match.\n", len(verifyErrs))
			for _, ve := range verifyErrs {
				fmt.Fprintf(p.out, "  - %s: %s\n", ve.Path, ve.Message)
			}
		}
	}

	// ── 9. Detect DB dumps for rehydrate offer ───────────────────────────────
	dumps := detectDBDumps(opts.TargetDir, fileList)

	result := &Result{
		SnapshotID:     snap.ID,
		SnapshotTime:   snap.Time,
		FilesRestored:  len(fileList),
		BytesRestored:  totalBytes(fileList),
		SealedFiles:    sealed,
		Verified:       verified,
		VerifyErrors:   verifyErrs,
		DBDumps:        dumps,
		UsedBreakGlass: usedBG,
	}
	return result, nil
}

// resolveSnapshot maps "latest" to the actual most-recent snapshot ID, or
// fetches metadata for a specific ID.
func (p *Pipeline) resolveSnapshot(ctx context.Context, opts Options) (*SnapshotMeta, error) {
	if opts.SnapshotID == "" || opts.SnapshotID == "latest" {
		return p.browser.Latest(ctx, opts.Source)
	}
	return p.browser.Get(ctx, opts.Source, opts.SnapshotID)
}

// resolveIdentity returns the age.Identity for unsealing secrets. It tries the
// keystore first; if unavailable (or ForceBreakGlass is set), it falls back to
// Shamir reconstruction.
func (p *Pipeline) resolveIdentity(ctx context.Context, opts Options) (goage.Identity, bool, error) {
	if !opts.ForceBreakGlass {
		id, err := p.keys.MasterIdentity(ctx)
		if err == nil {
			return id, false, nil
		}
		if !errors.Is(err, ErrKeystoreUnavailable) {
			return nil, false, fmt.Errorf("keystore: %w", err)
		}
		// Keystore unavailable on this machine → break-glass.
		fmt.Fprintln(p.out, "Keystore is unavailable on this machine.")
	} else {
		fmt.Fprintln(p.out, "Break-glass mode forced via --break-glass flag.")
	}
	fmt.Fprintln(p.out, "Entering break-glass recovery mode.")

	id, err := p.collectShares(ctx)
	if err != nil {
		return nil, false, err
	}
	return id, true, nil
}

// unsealTree walks targetDir, finds all .age files, and unseals them in place.
// Returns the number of files unsealed.
func (p *Pipeline) unsealTree(_ context.Context, dir string, identity goage.Identity) (int, error) {
	var count int
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, werr error) error {
		if werr != nil {
			return werr
		}
		if d.IsDir() || !p.unsealer.IsSealed(path) {
			return nil
		}
		plain := p.unsealer.PlainPath(path)
		p.log.Debug("unsealing secret", "sealed", path, "plain", plain)
		if err := p.unsealer.UnsealFile(identity, path, plain); err != nil {
			return fmt.Errorf("unseal %q: %w", path, err)
		}
		count++
		return nil
	})
	return count, err
}


func dryRunResult(snap *SnapshotMeta, files []FileEntry, usedBG bool) *Result {
	return &Result{
		SnapshotID:     snap.ID,
		SnapshotTime:   snap.Time,
		FilesRestored:  len(files),
		BytesRestored:  totalBytes(files),
		DryRun:         true,
		UsedBreakGlass: usedBG,
	}
}

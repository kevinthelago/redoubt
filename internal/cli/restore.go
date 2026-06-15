package cli

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	goage "filippo.io/age"
	"github.com/spf13/cobra"

	internalage "github.com/kevinthelago/redoubt/internal/age"
	"github.com/kevinthelago/redoubt/internal/config"
	"github.com/kevinthelago/redoubt/internal/escrow"
	"github.com/kevinthelago/redoubt/internal/keystore"
	"github.com/kevinthelago/redoubt/internal/logging"
	"github.com/kevinthelago/redoubt/internal/restic"
	"github.com/kevinthelago/redoubt/internal/restore"
)

var (
	restoreTarget     string
	restoreSnapshotID string
	restoreSource     string
	restoreIncludes   []string
	restoreCategories []string
	restoreOverwrite  bool
	restoreDryRun     bool
	restoreNoVerify   bool
	restoreBreakGlass bool
)

func init() {
	// Wire the real restore implementation into the stub command from root.go.
	restoreCmd.Long = `Restore a snapshot from the vault (default) or cold drive.

By default, the target directory must be empty. Use --overwrite to restore into
an existing directory (a typed confirmation is required).

Sealed secrets are automatically unsealed at the target. A post-restore
verification pass confirms every file matches the snapshot hashes.

If the keystore is unavailable (e.g. on a new machine), break-glass mode is
entered automatically: you will be prompted to enter your Shamir shares.`

	restoreCmd.Args = cobra.MaximumNArgs(1)
	restoreCmd.RunE = runRestore

	f := restoreCmd.Flags()
	f.StringVarP(&restoreTarget, "target", "t", "", "Directory to restore into (required)")
	f.StringVar(&restoreSnapshotID, "snapshot", "latest", "Snapshot ID to restore (default: latest)")
	f.StringVar(&restoreSource, "source", "vault", "Backend source: vault or cold")
	f.StringArrayVar(&restoreIncludes, "include", nil, "Restore only this path (repeatable)")
	f.StringArrayVar(&restoreCategories, "category", nil,
		"Restore only this category: source, secrets, database, config (repeatable)")
	f.BoolVar(&restoreOverwrite, "overwrite", false,
		"Allow restoring into a non-empty directory (requires typed confirmation)")
	f.BoolVar(&restoreDryRun, "dry-run", false, "Preview restore without writing any files")
	f.BoolVar(&restoreNoVerify, "no-verify", false, "Skip post-restore verification (not recommended)")
	f.BoolVar(&restoreBreakGlass, "break-glass", false,
		"Force Shamir share reconstruction even if keystore is available")
}

func runRestore(cmd *cobra.Command, args []string) error {
	snapshotID := restoreSnapshotID
	if len(args) > 0 {
		snapshotID = args[0]
	}
	if snapshotID == "" {
		snapshotID = "latest"
	}
	if restoreTarget == "" {
		return fmt.Errorf("--target is required")
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Overwrite requires typed confirmation before anything else.
	if restoreOverwrite && !restoreDryRun {
		if err := requireOverwriteConfirmation(cmd.OutOrStdout(), os.Stdin, restoreTarget); err != nil {
			return err
		}
	}

	// Resolve key material first — gives us both the age identity and the
	// restic password (HMAC of master key).
	km, usedBG, err := resolveKeyMaterial(restoreBreakGlass, os.Stdin, os.Stderr)
	if err != nil {
		return err
	}

	repoPass, err := km.ResticPassword()
	if err != nil {
		return fmt.Errorf("derive restic password: %w", err)
	}

	backend, err := resticBackendForSource(restoreSource, cfg)
	if err != nil {
		return fmt.Errorf("resolve backend: %w", err)
	}
	runner := restic.New(backend, restic.WithPassword(repoPass))

	log := logging.New(os.Stderr, slog.LevelInfo, repoPass)

	p := restore.NewPipeline(
		&resticRunnerAdapter{runner},
		&ageFileAdapter{},
		&preResolvedKeys{id: km.AgeIdentity, usedBG: usedBG},
		&noopEscrow{},
		&snapshotRunnerAdapter{runner},
		log,
		os.Stdin,
		cmd.OutOrStdout(),
	)

	ctx := context.Background()
	opts := restore.Options{
		SnapshotID: snapshotID,
		TargetDir:  restoreTarget,
		Overwrite:  restoreOverwrite,
		DryRun:     restoreDryRun,
		Includes:   restoreIncludes,
		Categories: restoreCategories,
		NoVerify:   restoreNoVerify,
	}

	result, err := p.Run(ctx, opts)
	if err != nil {
		return err
	}

	printRestoreResult(result, cmd.OutOrStdout())

	if len(result.DBDumps) > 0 && !restoreDryRun {
		if err := p.OfferRehydrate(ctx, result.DBDumps); err != nil {
			return err
		}
	}

	if !result.Verified && !result.DryRun && !restoreNoVerify {
		return fmt.Errorf("restore completed but verification FAILED (%d mismatch(es)); "+
			"review the output above", len(result.VerifyErrors))
	}
	return nil
}

// ── Key resolution ────────────────────────────────────────────────────────────

// resolveKeyMaterial opens and unlocks the OS keystore (prompting for a
// passphrase), or falls back to Shamir break-glass reconstruction.
// Returns the unlocked *KeyMaterial and whether break-glass was used.
func resolveKeyMaterial(forceBreakGlass bool, in *os.File, errOut *os.File) (*keystore.KeyMaterial, bool, error) {
	if !forceBreakGlass {
		km, err := keystore.Open()
		if err == nil {
			// Prompt for passphrase to unlock.
			fmt.Fprint(errOut, "Keystore passphrase: ")
			scanner := bufio.NewScanner(in)
			if !scanner.Scan() {
				return nil, false, fmt.Errorf("keystore: unexpected end of input")
			}
			passphrase := strings.TrimRight(scanner.Text(), "\r\n")
			if unlockErr := km.Unlock(passphrase); unlockErr != nil {
				if errors.Is(unlockErr, keystore.ErrWrongPassphrase) {
					return nil, false, fmt.Errorf("wrong passphrase — cannot decrypt")
				}
				return nil, false, fmt.Errorf("unlock keystore: %w", unlockErr)
			}
			return km, false, nil
		}
		if !errors.Is(err, keystore.ErrNotInitialized) {
			return nil, false, fmt.Errorf("open keystore: %w", err)
		}
		fmt.Fprintln(errOut, "Keystore not initialized on this machine.")
		fmt.Fprintln(errOut, "Entering break-glass recovery mode.")
	} else {
		fmt.Fprintln(errOut, "Break-glass mode forced via --break-glass flag.")
		fmt.Fprintln(errOut, "Entering break-glass recovery mode.")
	}

	km, err := collectBreakGlassShares(in, errOut)
	if err != nil {
		return nil, false, err
	}
	return km, true, nil
}

// collectBreakGlassShares prompts for Shamir shares and reconstructs KeyMaterial.
func collectBreakGlassShares(in *os.File, errOut *os.File) (*keystore.KeyMaterial, error) {
	const threshold = 2 // default 2-of-3; real value from escrow metadata
	fmt.Fprintf(errOut, "Enter %d escrow shares (base64, one per line):\n", threshold)

	scanner := bufio.NewScanner(in)
	shares := make([]escrow.Share, 0, threshold)
	for i := 0; i < threshold; i++ {
		fmt.Fprintf(errOut, "Share %d/%d: ", i+1, threshold)
		if !scanner.Scan() {
			return nil, errors.New("cannot decrypt: unexpected end of input")
		}
		raw, decErr := base64.StdEncoding.DecodeString(strings.TrimSpace(scanner.Text()))
		if decErr != nil || len(raw) < 2 {
			return nil, fmt.Errorf("cannot decrypt: invalid share format")
		}
		data := raw[1:]
		h := sha256.Sum256(data)
		shares = append(shares, escrow.Share{
			Index:    int(raw[0]),
			Data:     data,
			Checksum: hex.EncodeToString(h[:]),
		})
	}

	payload, err := escrow.Reconstruct(shares)
	if err != nil {
		return nil, fmt.Errorf("cannot decrypt: %w", err)
	}
	km, err := keystore.RestoreFromEscrowPayload(payload)
	if err != nil {
		return nil, errors.New("cannot decrypt: share material does not reconstruct a valid key")
	}
	fmt.Fprintln(errOut, "Key reconstructed from shares.")
	return km, nil
}

// ── Adapters ──────────────────────────────────────────────────────────────────

// preResolvedKeys is a KeyProvider that returns a pre-unlocked age identity.
// When usedBG is true the pipeline flags UsedBreakGlass in the result.
type preResolvedKeys struct {
	id     *goage.X25519Identity
	usedBG bool
}

func (p *preResolvedKeys) MasterIdentity(_ context.Context) (goage.Identity, error) {
	if p.id == nil {
		return nil, restore.ErrKeystoreUnavailable
	}
	return p.id, nil
}

// noopEscrow satisfies EscrowProvider when the CLI already reconstructed the key.
type noopEscrow struct{}

func (e *noopEscrow) Info() (int, int) { return 2, 3 }
func (e *noopEscrow) Reconstruct(_ context.Context, _ [][]byte) (goage.Identity, error) {
	return nil, errors.New("cannot decrypt: break-glass already resolved at CLI level")
}

// resticRunnerAdapter wraps restic.Runner to satisfy restore.ResticBackend.
type resticRunnerAdapter struct{ r *restic.Runner }

func (a *resticRunnerAdapter) Restore(ctx context.Context, snapshotID, targetDir string, includes []string, _ bool) error {
	return a.r.Restore(ctx, snapshotID, targetDir, includes)
}

func (a *resticRunnerAdapter) ListFiles(ctx context.Context, snapshotID string, includes []string) ([]restore.FileEntry, error) {
	entries, err := a.r.Ls(ctx, snapshotID, "")
	if err != nil {
		return nil, err
	}
	var out []restore.FileEntry
	for _, e := range entries {
		if e.Type == "dir" {
			continue
		}
		if len(includes) > 0 && !matchesAnyInclude(e.Path, includes) {
			continue
		}
		out = append(out, restore.FileEntry{Path: e.Path, Type: e.Type, Size: e.Size})
	}
	return out, nil
}

func (a *resticRunnerAdapter) LatestSnapshot(ctx context.Context) (string, error) {
	snaps, err := a.r.Snapshots(ctx, nil)
	if err != nil {
		return "", err
	}
	if len(snaps) == 0 {
		return "", fmt.Errorf("no snapshots found in repository")
	}
	return snaps[len(snaps)-1].ID, nil
}

// ageFileAdapter implements restore.FileUnsealer using the age streaming API.
type ageFileAdapter struct{}

func (a *ageFileAdapter) UnsealFile(identity goage.Identity, src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("age: open sealed file: %w", err)
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("age: create plain file: %w", err)
	}

	if err := internalage.Unseal(out, in, identity); err != nil {
		out.Close()
		os.Remove(dst)
		return fmt.Errorf("age: %w", err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("age: close plain file: %w", err)
	}
	in.Close()
	if err := os.Remove(src); err != nil {
		return fmt.Errorf("age: remove sealed file: %w", err)
	}
	return nil
}

func (a *ageFileAdapter) IsSealed(path string) bool {
	return strings.HasSuffix(path, ".age")
}

func (a *ageFileAdapter) PlainPath(sealed string) string {
	return strings.TrimSuffix(sealed, ".age")
}

// snapshotRunnerAdapter provides restore.SnapshotBrowser using restic.Runner.
// The source parameter is ignored because the runner is already configured
// for a specific backend; source selection happens at pipeline construction.
type snapshotRunnerAdapter struct{ r *restic.Runner }

func (s *snapshotRunnerAdapter) Latest(ctx context.Context, _ restore.Source) (*restore.SnapshotMeta, error) {
	snaps, err := s.r.Snapshots(ctx, nil)
	if err != nil {
		return nil, err
	}
	if len(snaps) == 0 {
		return nil, fmt.Errorf("no snapshots found in repository")
	}
	latest := snaps[len(snaps)-1]
	return snapshotToMeta(latest), nil
}

func (s *snapshotRunnerAdapter) Get(ctx context.Context, _ restore.Source, id string) (*restore.SnapshotMeta, error) {
	snaps, err := s.r.Snapshots(ctx, nil)
	if err != nil {
		return nil, err
	}
	for _, snap := range snaps {
		if snap.ID == id || strings.HasPrefix(snap.ID, id) || snap.ShortID == id {
			return snapshotToMeta(snap), nil
		}
	}
	return nil, fmt.Errorf("snapshot %q not found", id)
}

func snapshotToMeta(s restic.Snapshot) *restore.SnapshotMeta {
	return &restore.SnapshotMeta{
		ID:       s.ID,
		ShortID:  s.ShortID,
		Time:     s.Time,
		Hostname: s.Hostname,
		Tags:     s.Tags,
	}
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func matchesAnyInclude(path string, includes []string) bool {
	for _, inc := range includes {
		if strings.HasPrefix(path, inc) || path == inc {
			return true
		}
	}
	return false
}

func requireOverwriteConfirmation(out io.Writer, in *os.File, target string) error {
	fmt.Fprintf(out, "WARNING: --overwrite will restore into an existing directory.\n")
	fmt.Fprintf(out, "Existing files at conflicting paths will be overwritten.\n")
	fmt.Fprintf(out, "Type the target path to confirm: ")

	scanner := bufio.NewScanner(in)
	if !scanner.Scan() {
		return fmt.Errorf("overwrite confirmation: unexpected end of input")
	}
	answer := strings.TrimSpace(scanner.Text())
	if answer != target {
		return fmt.Errorf("overwrite confirmation: expected %q, got %q — aborting", target, answer)
	}
	return nil
}

func printRestoreResult(r *restore.Result, out io.Writer) {
	if r.DryRun {
		fmt.Fprintf(out, "Dry run — snapshot %s (%s)\n", r.SnapshotID, r.SnapshotTime.Format(time.RFC3339))
		fmt.Fprintf(out, "  Files: %d  Size: %s\n", r.FilesRestored, humanBytes(r.BytesRestored))
		fmt.Fprintln(out, "  (no files written)")
		return
	}
	fmt.Fprintf(out, "Restore complete — snapshot %s (%s)\n", r.SnapshotID, r.SnapshotTime.Format(time.RFC3339))
	fmt.Fprintf(out, "  Files restored:  %d\n", r.FilesRestored)
	fmt.Fprintf(out, "  Bytes restored:  %s\n", humanBytes(r.BytesRestored))
	fmt.Fprintf(out, "  Secrets unsealed:%d\n", r.SealedFiles)
	if r.UsedBreakGlass {
		fmt.Fprintln(out, "  Key source:      Shamir reconstruction (break-glass)")
	}
	if r.Verified {
		fmt.Fprintln(out, "  Verification:    PASSED")
	} else if len(r.VerifyErrors) > 0 {
		fmt.Fprintf(out, "  Verification:    FAILED (%d mismatch(es))\n", len(r.VerifyErrors))
	}
}

func humanBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}

// Package restore implements the full restore pipeline: pre-flight checks,
// restic restore, age-unseal of secrets, post-restore verification, and
// break-glass key reconstruction. It is the core of what the whole system
// exists to do.
package restore

import (
	"context"
	"errors"
	"time"

	goage "filippo.io/age"
)

// ErrKeystoreUnavailable is returned by KeyProvider.MasterIdentity when the
// keystore is not initialized or locked on this machine, triggering break-glass.
var ErrKeystoreUnavailable = errors.New("keystore: unavailable")

// Source selects the restic backend for a restore.
type Source int

const (
	// SourceVault restores from the append-only LAN vault (default).
	SourceVault Source = iota
	// SourceColdDrive restores from the offline cold-copy drive.
	SourceColdDrive
)

// Options controls a restore operation.
type Options struct {
	// SnapshotID is the restic snapshot to restore. "latest" selects the
	// most recent snapshot.
	SnapshotID string
	// Source selects the backend (vault or cold drive).
	Source Source
	// TargetDir is the directory to restore into. Must exist.
	TargetDir string
	// Overwrite allows restoring into a non-empty directory. Requires
	// explicit typed confirmation from the operator.
	Overwrite bool
	// DryRun previews the restore (file list + sizes) without writing.
	DryRun bool
	// Includes restricts the restore to specific paths or globs. Empty = all.
	Includes []string
	// Categories restricts the restore to specific snapshot tag categories
	// (source, secrets, database, config). Empty = all.
	Categories []string
	// NoVerify skips the post-restore byte verification step. Not recommended.
	NoVerify bool
	// ForceBreakGlass bypasses the keystore and forces Shamir reconstruction
	// even if the keystore is reachable. Used for testing escrow shares.
	ForceBreakGlass bool
}

// VerifyError describes a single post-restore verification mismatch.
type VerifyError struct {
	Path    string
	Message string
}

// DBDump describes a database dump file that was restored and may be rehydrated.
type DBDump struct {
	Path        string // local path of the restored dump file
	Category    string // e.g. "database"
	RehydratCmd string // preset import command (empty if none known)
}

// Result is the outcome of a successful restore operation.
type Result struct {
	SnapshotID    string
	SnapshotTime  time.Time
	FilesRestored int
	BytesRestored int64
	SealedFiles   int       // age-sealed files that were unsealed
	Verified      bool      // true if post-restore verification passed
	VerifyErrors  []VerifyError
	DBDumps       []DBDump
	DryRun        bool
	UsedBreakGlass bool
}

// ResticBackend performs restic restore operations.
type ResticBackend interface {
	// Restore extracts snapshotID to targetDir.
	// includes filters to specific paths (nil = all).
	// verify enables restic's own --verify pass after restore.
	Restore(ctx context.Context, snapshotID, targetDir string, includes []string, verify bool) error
	// ListFiles returns the set of file entries in snapshotID.
	// Used for pre-flight space estimation.
	ListFiles(ctx context.Context, snapshotID string, includes []string) ([]FileEntry, error)
	// LatestSnapshot returns the ID of the most recent snapshot.
	LatestSnapshot(ctx context.Context) (string, error)
}

// FileEntry is one entry from the snapshot file list.
type FileEntry struct {
	Path string
	Type string // "file", "dir", "symlink"
	Size int64
	Tags []string
}

// FileUnsealer age-unseals a file in the restored tree.
type FileUnsealer interface {
	// UnsealFile decrypts src (.age file) to dst (plain path) using identity.
	// The .age source is removed after a successful unseal.
	UnsealFile(identity goage.Identity, src, dst string) error
	// IsSealed reports whether path ends in ".age".
	IsSealed(path string) bool
	// PlainPath strips the ".age" suffix from a sealed path.
	PlainPath(sealed string) string
}

// KeyProvider returns the master age identity from the OS keystore.
type KeyProvider interface {
	// MasterIdentity returns the identity, or ErrKeystoreUnavailable if the
	// keystore is not initialized or locked on this machine.
	MasterIdentity(ctx context.Context) (goage.Identity, error)
}

// EscrowProvider reconstructs the master identity from Shamir shares.
type EscrowProvider interface {
	// Info returns the share threshold and total count stored in the escrow.
	Info() (threshold, total int)
	// Reconstruct rebuilds the age.Identity from a threshold of shares.
	// Each share is [index byte || value bytes] as produced by SplitIdentity.
	Reconstruct(ctx context.Context, shares [][]byte) (goage.Identity, error)
}

// SnapshotBrowser provides snapshot metadata needed for restore.
type SnapshotBrowser interface {
	// Latest returns the most recent snapshot from the given source.
	Latest(ctx context.Context, source Source) (*SnapshotMeta, error)
	// Get returns the snapshot with the given ID.
	Get(ctx context.Context, source Source, snapshotID string) (*SnapshotMeta, error)
}

// SnapshotMeta holds the metadata the restore pipeline needs about a snapshot.
type SnapshotMeta struct {
	ID       string
	ShortID  string
	Time     time.Time
	Hostname string
	Tags     []string
}

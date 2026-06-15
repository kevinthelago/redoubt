// Package restore exposes the Restorer interface consumed by the drill engine.
// STUB — restore stream (R1) owns the full implementation.
package restore

import "context"

// Source identifies which snapshot repository to use.
type Source string

const (
	SourceVault Source = "vault"
	SourceCold  Source = "cold"
)

// Restorer provides snapshot access needed by the drill engine.
type Restorer interface {
	// LatestSnapshotID returns the most recent snapshot ID for src.
	// Returns ("", false, nil) when the repository has no snapshots.
	LatestSnapshotID(ctx context.Context, src Source) (string, bool, error)

	// ListSnapshotFiles returns the paths of all files in snapshotID whose path
	// starts with prefix. Pass an empty prefix to list everything.
	ListSnapshotFiles(ctx context.Context, src Source, snapshotID, prefix string) ([]string, error)

	// RestoreFiles restores the named paths from snapshotID into targetDir.
	// targetDir must be an empty, isolated directory that the caller controls.
	RestoreFiles(ctx context.Context, src Source, snapshotID string, paths []string, targetDir string) error

	// FreeSpaceAt returns the available bytes on the volume containing path.
	FreeSpaceAt(path string) (uint64, error)
}

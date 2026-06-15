package snapshots

import "context"

// Client is the interface for restic operations consumed by the browsing engine.
//
// The real implementation lives in internal/restic (Foundation stream F3).
// This interface is deliberately narrow — it expresses only what snapshot
// browsing needs so that the Foundation stream can evolve its full API freely.
type Client interface {
	// ListSnapshots returns snapshots from the repository.  The implementation
	// applies host and tag filters at the restic layer when possible.
	ListSnapshots(ctx context.Context, filter SnapshotFilter) ([]Snapshot, error)

	// ListFiles returns the file tree rooted at path inside snapshotID.
	// Pass an empty path to list the root.
	ListFiles(ctx context.Context, snapshotID, path string) ([]TreeNode, error)

	// FindPath returns, for each snapshot that contains path, the matching
	// nodes grouped by snapshot.  path may be an exact path or a glob pattern.
	FindPath(ctx context.Context, path string, filter SnapshotFilter) ([]FindResult, error)

	// DiffSnapshots returns the set of paths that differ between snapshotA
	// and snapshotB.
	DiffSnapshots(ctx context.Context, snapshotA, snapshotB string) ([]DiffEntry, error)
}

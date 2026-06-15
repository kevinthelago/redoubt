// Package snapshots provides a read-only browsing engine over restic snapshots.
//
// It exposes four operations — List, LS, Find, Diff — and enforces two
// invariants that callers must not bypass:
//
//  1. The engine is strictly read-only; it never issues a restic write command.
//  2. Age-sealed secret entries (files ending in .age, or any file in a snapshot
//     tagged "secrets") are surfaced as sealed filenames only.  The engine
//     never decrypts or reveals their contents.
package snapshots

import (
	"context"
	"fmt"
)

// Browser is the read-only snapshot browsing engine.
// Construct one with New; all methods are safe for concurrent use if the
// underlying Client is.
type Browser struct {
	client Client
}

// New creates a Browser backed by client.  client must already be
// configured for the desired source (vault or cold drive) — the browser
// itself is source-agnostic.
func New(client Client) *Browser {
	return &Browser{client: client}
}

// List returns all snapshots that satisfy filter, sorted by the client's
// natural order (typically descending time).  An empty result is not an
// error; the caller should present a friendly empty state.  The returned
// slice is always non-nil, so JSON encoding produces [] rather than null.
func (b *Browser) List(ctx context.Context, filter SnapshotFilter) ([]Snapshot, error) {
	snaps, err := b.client.ListSnapshots(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("snapshots list: %w", err)
	}
	// Client-side filter pass for any fields the backend didn't handle.
	result := make([]Snapshot, 0, len(snaps))
	for _, s := range snaps {
		if s.matchesFilter(filter) {
			result = append(result, s)
		}
	}
	return result, nil
}

// LS returns the file tree at path inside snapshotID.  Pass an empty path to
// list the root.  Files in a "secrets"-tagged snapshot or with the .age
// extension are returned with IsSealed=true; their names are visible but the
// engine never reads their contents.  The returned slice is always non-nil.
func (b *Browser) LS(ctx context.Context, snapshotID, path string) ([]TreeNode, error) {
	nodes, err := b.client.ListFiles(ctx, snapshotID, path)
	if err != nil {
		return nil, fmt.Errorf("snapshots ls %s:%s: %w", snapshotID, path, err)
	}
	if nodes == nil {
		nodes = []TreeNode{}
	}
	// We don't know the snapshot tags at this point, so sealed detection is
	// purely path-based here; the CLI layer can pass the snapshot's tag state
	// down to further annotate if needed.
	for i := range nodes {
		if isSealed(nodes[i].Path) {
			nodes[i].IsSealed = true
		}
	}
	return nodes, nil
}

// LSSealed is like LS but marks every node as sealed because the snapshot
// itself is a secrets-tagged snapshot.  Called by the CLI when it has
// already determined the snapshot's category.
func (b *Browser) LSSealed(ctx context.Context, snapshotID, path string) ([]TreeNode, error) {
	nodes, err := b.LS(ctx, snapshotID, path)
	if err != nil {
		return nil, err
	}
	for i := range nodes {
		nodes[i].IsSealed = true
	}
	return nodes, nil
}

// Find finds nodes matching path (exact path or glob) across all snapshots
// that satisfy filter.  Snapshots that contain no match are omitted from the
// result.  An empty result is not an error.
func (b *Browser) Find(ctx context.Context, path string, filter SnapshotFilter) ([]FindResult, error) {
	if path == "" {
		return nil, fmt.Errorf("snapshots find: path must not be empty")
	}
	results, err := b.client.FindPath(ctx, path, filter)
	if err != nil {
		return nil, fmt.Errorf("snapshots find %q: %w", path, err)
	}
	if results == nil {
		return []FindResult{}, nil
	}
	for i := range results {
		sealed := results[i].Snapshot.IsSecretSnapshot()
		for j := range results[i].Matches {
			if sealed || isSealed(results[i].Matches[j].Path) {
				results[i].Matches[j].IsSealed = true
			}
		}
	}
	return results, nil
}

// Diff returns the paths that differ between snapshotA and snapshotB.  An
// empty slice means the snapshots are identical.  The caller is responsible
// for validating that both IDs exist before calling Diff.
func (b *Browser) Diff(ctx context.Context, snapshotA, snapshotB string) ([]DiffEntry, error) {
	if snapshotA == "" || snapshotB == "" {
		return nil, fmt.Errorf("snapshots diff: both snapshot IDs must be non-empty")
	}
	entries, err := b.client.DiffSnapshots(ctx, snapshotA, snapshotB)
	if err != nil {
		return nil, fmt.Errorf("snapshots diff %s..%s: %w", snapshotA, snapshotB, err)
	}
	if entries == nil {
		return []DiffEntry{}, nil
	}
	for i := range entries {
		if isSealed(entries[i].Path) {
			entries[i].IsSealed = true
		}
	}
	return entries, nil
}

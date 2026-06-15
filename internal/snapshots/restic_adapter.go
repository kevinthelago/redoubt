package snapshots

import (
	"context"

	"github.com/kevinthelago/redoubt/internal/restic"
)

// ResticAdapter implements Client using Foundation's restic.Runner.
// Construct one with NewResticAdapter; wire it into the CLI via
// snapshots.New(NewResticAdapter(runner)).
type ResticAdapter struct {
	runner *restic.Runner
}

// NewResticAdapter wraps a restic.Runner as a snapshots.Client.
func NewResticAdapter(runner *restic.Runner) Client {
	return &ResticAdapter{runner: runner}
}

// ListSnapshots lists snapshots, pushing tag filtering to restic and leaving
// hostname/date filtering to the browser's client-side pass.
func (a *ResticAdapter) ListSnapshots(ctx context.Context, filter SnapshotFilter) ([]Snapshot, error) {
	raw, err := a.runner.Snapshots(ctx, filter.Tags)
	if err != nil {
		return nil, err
	}
	snaps := make([]Snapshot, len(raw))
	for i, r := range raw {
		snaps[i] = Snapshot{
			ID:       r.ID,
			ShortID:  r.ShortID,
			Time:     r.Time,
			Hostname: r.Hostname,
			Tags:     r.Tags,
			Paths:    r.Paths,
			// BytesAdded: not in Foundation's Snapshot type — stays 0.
			// Foundation's BackupSummary carries DataAdded, but Snapshots()
			// doesn't return it.  Noted via bsc-note for Foundation to add.
		}
	}
	return snaps, nil
}

// ListFiles lists entries in a snapshot, optionally under a path prefix.
func (a *ResticAdapter) ListFiles(ctx context.Context, snapshotID, path string) ([]TreeNode, error) {
	raw, err := a.runner.Ls(ctx, snapshotID, path)
	if err != nil {
		return nil, err
	}
	nodes := make([]TreeNode, len(raw))
	for i, r := range raw {
		nodes[i] = TreeNode{
			Name:    r.Name,
			Path:    r.Path,
			Type:    r.Type,
			Size:    r.Size,
			ModTime: r.Mtime,
			// Mode: not in Foundation's LsEntry — stays 0.
		}
	}
	return nodes, nil
}

// FindPath finds nodes matching pattern across all snapshots, then applies
// filter (hostname/date/additional tags) on the fetched snapshot metadata.
// This requires two restic invocations: Find and Snapshots.
func (a *ResticAdapter) FindPath(ctx context.Context, pattern string, filter SnapshotFilter) ([]FindResult, error) {
	// Find matches across all snapshots; restic handles the glob expansion.
	hits, err := a.runner.Find(ctx, pattern, "")
	if err != nil {
		return nil, err
	}
	if len(hits) == 0 {
		return []FindResult{}, nil
	}

	// Fetch snapshot metadata so we can apply hostname/date/tag filtering and
	// build the full Snapshot value for each FindResult.
	allRaw, err := a.runner.Snapshots(ctx, nil)
	if err != nil {
		return nil, err
	}
	snapByID := make(map[string]Snapshot, len(allRaw))
	for _, r := range allRaw {
		snapByID[r.ID] = Snapshot{
			ID:       r.ID,
			ShortID:  r.ShortID,
			Time:     r.Time,
			Hostname: r.Hostname,
			Tags:     r.Tags,
			Paths:    r.Paths,
		}
	}

	// Group hits by snapshot in insertion order, applying filter.
	type snapGroup struct {
		snap    Snapshot
		matches []TreeNode
	}
	seen := make(map[string]int) // snapshotID → index in groups
	var groups []snapGroup

	for _, h := range hits {
		snap, ok := snapByID[h.SnapshotID]
		if !ok {
			continue // pruned/unknown snapshot
		}
		if !snap.matchesFilter(filter) {
			continue
		}
		node := TreeNode{
			Path:    h.Path,
			Type:    h.Type,
			ModTime: h.Mtime,
			Name:    nodeNameFromPath(h.Path),
		}
		if idx, already := seen[h.SnapshotID]; already {
			groups[idx].matches = append(groups[idx].matches, node)
		} else {
			seen[h.SnapshotID] = len(groups)
			groups = append(groups, snapGroup{snap: snap, matches: []TreeNode{node}})
		}
	}

	results := make([]FindResult, len(groups))
	for i, g := range groups {
		results[i] = FindResult{Snapshot: g.snap, Matches: g.matches}
	}
	return results, nil
}

// DiffSnapshots returns the changed paths between two snapshots, translating
// restic's single-char modifier into a human-readable change type.
func (a *ResticAdapter) DiffSnapshots(ctx context.Context, snapshotA, snapshotB string) ([]DiffEntry, error) {
	raw, err := a.runner.Diff(ctx, snapshotA, snapshotB)
	if err != nil {
		return nil, err
	}
	entries := make([]DiffEntry, len(raw))
	for i, r := range raw {
		entries[i] = DiffEntry{
			Path:       r.Path,
			ChangeType: modifierToChangeType(r.Modifier),
		}
	}
	return entries, nil
}

// modifierToChangeType maps restic's single-char diff modifier to the
// canonical change type string used by DiffEntry.
func modifierToChangeType(m string) string {
	switch m {
	case "+":
		return "added"
	case "-":
		return "removed"
	default:
		// "M" (modified), "T" (type changed), "U" (unchanged but visited)
		return "changed"
	}
}

// nodeNameFromPath extracts the filename from a full path.
func nodeNameFromPath(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' || path[i] == '\\' {
			return path[i+1:]
		}
	}
	return path
}

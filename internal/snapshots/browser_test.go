package snapshots_test

import (
	"context"
	"io/fs"
	"testing"
	"time"

	"github.com/kevinthelago/redoubt/internal/snapshots"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- mock client ---

type mockClient struct {
	snapshots   []snapshots.Snapshot
	files       map[string][]snapshots.TreeNode // snapshotID -> nodes
	findResults []snapshots.FindResult
	diffEntries []snapshots.DiffEntry
	listErr     error
	filesErr    error
	findErr     error
	diffErr     error
}

func (m *mockClient) ListSnapshots(_ context.Context, filter snapshots.SnapshotFilter) ([]snapshots.Snapshot, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.snapshots, nil
}

func (m *mockClient) ListFiles(_ context.Context, snapshotID, _ string) ([]snapshots.TreeNode, error) {
	if m.filesErr != nil {
		return nil, m.filesErr
	}
	return m.files[snapshotID], nil
}

func (m *mockClient) FindPath(_ context.Context, _ string, _ snapshots.SnapshotFilter) ([]snapshots.FindResult, error) {
	if m.findErr != nil {
		return nil, m.findErr
	}
	return m.findResults, nil
}

func (m *mockClient) DiffSnapshots(_ context.Context, _, _ string) ([]snapshots.DiffEntry, error) {
	if m.diffErr != nil {
		return nil, m.diffErr
	}
	return m.diffEntries, nil
}

// --- helpers ---

func now() time.Time { return time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC) }

func snap(id, host string, tags ...string) snapshots.Snapshot {
	short := id
	if len(id) > 8 {
		short = id[:8]
	}
	return snapshots.Snapshot{
		ID:       id,
		ShortID:  short,
		Time:     now(),
		Hostname: host,
		Tags:     tags,
		Paths:    []string{"/home/user"},
	}
}

func node(name, typ string, sealed bool) snapshots.TreeNode {
	return snapshots.TreeNode{
		Name:     name,
		Path:     "/" + name,
		Type:     typ,
		Size:     1024,
		Mode:     fs.FileMode(0o644),
		ModTime:  now(),
		IsSealed: sealed,
	}
}

// --- List tests ---

func TestList_EmptyRepository(t *testing.T) {
	b := snapshots.New(&mockClient{})
	result, err := b.List(context.Background(), snapshots.SnapshotFilter{})
	require.NoError(t, err)
	assert.Empty(t, result, "empty repo must return empty slice, not an error")
}

func TestList_NoFilter(t *testing.T) {
	snaps := []snapshots.Snapshot{
		snap("aaaa1111", "host-a", "source"),
		snap("bbbb2222", "host-b", "database"),
	}
	b := snapshots.New(&mockClient{snapshots: snaps})
	result, err := b.List(context.Background(), snapshots.SnapshotFilter{})
	require.NoError(t, err)
	assert.Len(t, result, 2)
}

func TestList_FilterByTag(t *testing.T) {
	snaps := []snapshots.Snapshot{
		snap("aaaa1111", "host-a", "source"),
		snap("bbbb2222", "host-b", "database"),
		snap("cccc3333", "host-a", "source", "database"),
	}
	b := snapshots.New(&mockClient{snapshots: snaps})

	result, err := b.List(context.Background(), snapshots.SnapshotFilter{Tags: []string{"source"}})
	require.NoError(t, err)
	assert.Len(t, result, 2, "should match aaaa and cccc which both have 'source'")
}

func TestList_FilterByMultipleTags(t *testing.T) {
	snaps := []snapshots.Snapshot{
		snap("aaaa1111", "host-a", "source"),
		snap("bbbb2222", "host-b", "source", "database"),
	}
	b := snapshots.New(&mockClient{snapshots: snaps})

	result, err := b.List(context.Background(), snapshots.SnapshotFilter{Tags: []string{"source", "database"}})
	require.NoError(t, err)
	assert.Len(t, result, 1)
	assert.Equal(t, "bbbb2222", result[0].ID)
}

func TestList_FilterByHostname(t *testing.T) {
	snaps := []snapshots.Snapshot{
		snap("aaaa1111", "host-a", "source"),
		snap("bbbb2222", "host-b", "source"),
	}
	b := snapshots.New(&mockClient{snapshots: snaps})

	result, err := b.List(context.Background(), snapshots.SnapshotFilter{Hostname: "host-a"})
	require.NoError(t, err)
	assert.Len(t, result, 1)
	assert.Equal(t, "host-a", result[0].Hostname)
}

func TestList_FilterByHostnameIsCaseInsensitive(t *testing.T) {
	snaps := []snapshots.Snapshot{snap("aaaa1111", "HOST-A", "source")}
	b := snapshots.New(&mockClient{snapshots: snaps})

	result, err := b.List(context.Background(), snapshots.SnapshotFilter{Hostname: "host-a"})
	require.NoError(t, err)
	assert.Len(t, result, 1)
}

func TestList_FilterByDateRange(t *testing.T) {
	base := now()
	snaps := []snapshots.Snapshot{
		{ID: "old00001", Time: base.Add(-48 * time.Hour), Hostname: "h", Tags: []string{"source"}},
		{ID: "mid00001", Time: base.Add(-24 * time.Hour), Hostname: "h", Tags: []string{"source"}},
		{ID: "new00001", Time: base, Hostname: "h", Tags: []string{"source"}},
	}
	b := snapshots.New(&mockClient{snapshots: snaps})

	after := base.Add(-36 * time.Hour)
	result, err := b.List(context.Background(), snapshots.SnapshotFilter{After: after})
	require.NoError(t, err)
	assert.Len(t, result, 2, "mid and new should be returned")
}

func TestList_PropagatesClientError(t *testing.T) {
	b := snapshots.New(&mockClient{listErr: assert.AnError})
	_, err := b.List(context.Background(), snapshots.SnapshotFilter{})
	require.Error(t, err)
}

// --- LS tests ---

func TestLS_FileTreeReturned(t *testing.T) {
	files := map[string][]snapshots.TreeNode{
		"snap0001": {
			node("code", "dir", false),
			node("main.go", "file", false),
		},
	}
	b := snapshots.New(&mockClient{files: files})

	nodes, err := b.LS(context.Background(), "snap0001", "")
	require.NoError(t, err)
	assert.Len(t, nodes, 2)
}

func TestLS_SealedFileMarkedByExtension(t *testing.T) {
	files := map[string][]snapshots.TreeNode{
		"snap0001": {
			{Name: "secrets.tar.age", Path: "/secrets.tar.age", Type: "file"},
			{Name: "readme.txt", Path: "/readme.txt", Type: "file"},
		},
	}
	b := snapshots.New(&mockClient{files: files})

	nodes, err := b.LS(context.Background(), "snap0001", "")
	require.NoError(t, err)
	require.Len(t, nodes, 2)
	assert.True(t, nodes[0].IsSealed, ".age file must be sealed")
	assert.False(t, nodes[1].IsSealed, "plain file must not be sealed")
}

func TestLS_AllSealedWhenSnapshotIsSecretCategory(t *testing.T) {
	files := map[string][]snapshots.TreeNode{
		"snap0001": {
			{Name: "env.age", Path: "/env.age", Type: "file"},
			{Name: "key.age", Path: "/key.age", Type: "file"},
		},
	}
	b := snapshots.New(&mockClient{files: files})

	nodes, err := b.LSSealed(context.Background(), "snap0001", "")
	require.NoError(t, err)
	for _, n := range nodes {
		assert.True(t, n.IsSealed, "all nodes in a secrets snapshot must be sealed")
	}
}

func TestLS_EmptySnapshot(t *testing.T) {
	b := snapshots.New(&mockClient{files: map[string][]snapshots.TreeNode{"snap0001": nil}})
	nodes, err := b.LS(context.Background(), "snap0001", "")
	require.NoError(t, err)
	assert.Empty(t, nodes)
}

func TestLS_PropagatesClientError(t *testing.T) {
	b := snapshots.New(&mockClient{filesErr: assert.AnError})
	_, err := b.LS(context.Background(), "snap0001", "")
	require.Error(t, err)
}

// --- Find tests ---

func TestFind_ReturnsMatchesAcrossSnapshots(t *testing.T) {
	results := []snapshots.FindResult{
		{
			Snapshot: snap("aaaa1111", "host-a", "source"),
			Matches:  []snapshots.TreeNode{node("main.go", "file", false)},
		},
		{
			Snapshot: snap("bbbb2222", "host-a", "source"),
			Matches:  []snapshots.TreeNode{node("main.go", "file", false)},
		},
	}
	b := snapshots.New(&mockClient{findResults: results})

	found, err := b.Find(context.Background(), "main.go", snapshots.SnapshotFilter{})
	require.NoError(t, err)
	assert.Len(t, found, 2)
}

func TestFind_EmptyResult(t *testing.T) {
	b := snapshots.New(&mockClient{})
	found, err := b.Find(context.Background(), "nonexistent.txt", snapshots.SnapshotFilter{})
	require.NoError(t, err)
	assert.Empty(t, found, "no matches is not an error")
}

func TestFind_EmptyPathReturnsError(t *testing.T) {
	b := snapshots.New(&mockClient{})
	_, err := b.Find(context.Background(), "", snapshots.SnapshotFilter{})
	require.Error(t, err)
}

func TestFind_SealedByExtension(t *testing.T) {
	results := []snapshots.FindResult{
		{
			Snapshot: snap("aaaa1111", "host-a", "source"),
			Matches: []snapshots.TreeNode{
				{Name: "creds.age", Path: "/creds.age", Type: "file"},
			},
		},
	}
	b := snapshots.New(&mockClient{findResults: results})

	found, err := b.Find(context.Background(), "creds.age", snapshots.SnapshotFilter{})
	require.NoError(t, err)
	require.Len(t, found, 1)
	assert.True(t, found[0].Matches[0].IsSealed)
}

func TestFind_SealedBySnapshotCategory(t *testing.T) {
	results := []snapshots.FindResult{
		{
			Snapshot: snap("aaaa1111", "host-a", "secrets"),
			Matches: []snapshots.TreeNode{
				{Name: "env.age", Path: "/env.age", Type: "file"},
				{Name: "apikey.age", Path: "/apikey.age", Type: "file"},
			},
		},
	}
	b := snapshots.New(&mockClient{findResults: results})

	found, err := b.Find(context.Background(), "*.age", snapshots.SnapshotFilter{})
	require.NoError(t, err)
	for _, match := range found[0].Matches {
		assert.True(t, match.IsSealed, "every match in a secrets snapshot must be sealed")
	}
}

func TestFind_PropagatesClientError(t *testing.T) {
	b := snapshots.New(&mockClient{findErr: assert.AnError})
	_, err := b.Find(context.Background(), "file.txt", snapshots.SnapshotFilter{})
	require.Error(t, err)
}

// --- Diff tests ---

func TestDiff_ReturnsDifferences(t *testing.T) {
	entries := []snapshots.DiffEntry{
		{Path: "/added.txt", ChangeType: "added"},
		{Path: "/removed.txt", ChangeType: "removed"},
		{Path: "/changed.txt", ChangeType: "changed"},
	}
	b := snapshots.New(&mockClient{diffEntries: entries})

	diff, err := b.Diff(context.Background(), "aaaa1111", "bbbb2222")
	require.NoError(t, err)
	assert.Len(t, diff, 3)
}

func TestDiff_EmptyDiff(t *testing.T) {
	b := snapshots.New(&mockClient{})
	diff, err := b.Diff(context.Background(), "aaaa1111", "bbbb2222")
	require.NoError(t, err)
	assert.Empty(t, diff, "identical snapshots produce no diff entries")
}

func TestDiff_SealedFileMarked(t *testing.T) {
	entries := []snapshots.DiffEntry{
		{Path: "/secrets.tar.age", ChangeType: "changed"},
		{Path: "/readme.txt", ChangeType: "changed"},
	}
	b := snapshots.New(&mockClient{diffEntries: entries})

	diff, err := b.Diff(context.Background(), "aaaa1111", "bbbb2222")
	require.NoError(t, err)
	require.Len(t, diff, 2)
	assert.True(t, diff[0].IsSealed, ".age diff entry must be sealed")
	assert.False(t, diff[1].IsSealed)
}

func TestDiff_EmptySnapshotIDReturnsError(t *testing.T) {
	b := snapshots.New(&mockClient{})
	_, err := b.Diff(context.Background(), "", "bbbb2222")
	require.Error(t, err)

	_, err = b.Diff(context.Background(), "aaaa1111", "")
	require.Error(t, err)
}

func TestDiff_PropagatesClientError(t *testing.T) {
	b := snapshots.New(&mockClient{diffErr: assert.AnError})
	_, err := b.Diff(context.Background(), "aaaa1111", "bbbb2222")
	require.Error(t, err)
}

// --- Snapshot type tests ---

func TestSnapshotHasTag_CaseInsensitive(t *testing.T) {
	s := snap("aaaa1111", "h", "Source", "DATABASE")
	assert.True(t, s.HasTag("source"))
	assert.True(t, s.HasTag("database"))
	assert.False(t, s.HasTag("config"))
}

func TestSnapshotIsSecretSnapshot(t *testing.T) {
	assert.True(t, snap("a", "h", "secrets").IsSecretSnapshot())
	assert.False(t, snap("a", "h", "source").IsSecretSnapshot())
	assert.False(t, snap("a", "h").IsSecretSnapshot())
}

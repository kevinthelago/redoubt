package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/kevinthelago/redoubt/internal/cli"
	"github.com/kevinthelago/redoubt/internal/snapshots"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- stub client ---

type stubClient struct {
	snaps   []snapshots.Snapshot
	files   map[string][]snapshots.TreeNode
	findRes []snapshots.FindResult
	diffRes []snapshots.DiffEntry
}

func (s *stubClient) ListSnapshots(_ context.Context, _ snapshots.SnapshotFilter) ([]snapshots.Snapshot, error) {
	return s.snaps, nil
}
func (s *stubClient) ListFiles(_ context.Context, id, _ string) ([]snapshots.TreeNode, error) {
	return s.files[id], nil
}
func (s *stubClient) FindPath(_ context.Context, _ string, _ snapshots.SnapshotFilter) ([]snapshots.FindResult, error) {
	return s.findRes, nil
}
func (s *stubClient) DiffSnapshots(_ context.Context, _, _ string) ([]snapshots.DiffEntry, error) {
	return s.diffRes, nil
}

func newFactory(c snapshots.Client) func(string) (snapshots.Client, error) {
	return func(_ string) (snapshots.Client, error) { return c, nil }
}

func runCmd(args []string, client snapshots.Client) (string, error) {
	var buf bytes.Buffer
	cmd := cli.ExportedBuildSnapshotsCmd(newFactory(client), &buf)
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs(args)
	err := cmd.ExecuteContext(context.Background())
	return buf.String(), err
}

// --- list tests ---

func TestCLI_ListEmpty_ShowsFriendlyMessage(t *testing.T) {
	out, err := runCmd([]string{"list"}, &stubClient{})
	require.NoError(t, err)
	assert.Contains(t, out, "No snapshots found")
	assert.Contains(t, out, "redoubt backup")
}

func TestCLI_ListEmpty_JSONReturnsEmptyArray(t *testing.T) {
	out, err := runCmd([]string{"list", "--json"}, &stubClient{})
	require.NoError(t, err)
	var result []snapshots.Snapshot
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	assert.Empty(t, result)
}

func TestCLI_ListSnapshots_TableHasExpectedColumns(t *testing.T) {
	c := &stubClient{
		snaps: []snapshots.Snapshot{
			{
				ID: "aaaa1111bbbb2222", ShortID: "aaaa1111",
				Time:     time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC),
				Hostname: "myhost",
				Tags:     []string{"source"},
				BytesAdded: 1024 * 1024,
			},
		},
	}
	out, err := runCmd([]string{"list"}, c)
	require.NoError(t, err)
	assert.Contains(t, out, "aaaa1111")
	assert.Contains(t, out, "myhost")
	assert.Contains(t, out, "source")
	assert.Contains(t, out, "MiB")
}

func TestCLI_ListSnapshots_JSONContainsSnapshotID(t *testing.T) {
	c := &stubClient{
		snaps: []snapshots.Snapshot{
			{ID: "aaaa1111bbbb2222", ShortID: "aaaa1111", Hostname: "h", Tags: []string{"source"}},
		},
	}
	out, err := runCmd([]string{"list", "--json"}, c)
	require.NoError(t, err)
	assert.Contains(t, out, "aaaa1111bbbb2222")
}

func TestCLI_ListFilterByTag(t *testing.T) {
	c := &stubClient{
		snaps: []snapshots.Snapshot{
			{ID: "a", ShortID: "a1234567", Hostname: "h", Tags: []string{"source"}},
			{ID: "b", ShortID: "b1234567", Hostname: "h", Tags: []string{"database"}},
		},
	}
	out, err := runCmd([]string{"list", "--tag", "source"}, c)
	require.NoError(t, err)
	assert.Contains(t, out, "a1234567")
	assert.NotContains(t, out, "b1234567")
}

func TestCLI_ListFilterByHost(t *testing.T) {
	c := &stubClient{
		snaps: []snapshots.Snapshot{
			{ID: "a", ShortID: "a1234567", Hostname: "host-a", Tags: []string{"source"}},
			{ID: "b", ShortID: "b1234567", Hostname: "host-b", Tags: []string{"source"}},
		},
	}
	out, err := runCmd([]string{"list", "--host", "host-a"}, c)
	require.NoError(t, err)
	assert.Contains(t, out, "host-a")
	assert.NotContains(t, out, "host-b")
}

func TestCLI_ListFilterByDateRange(t *testing.T) {
	base := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	c := &stubClient{
		snaps: []snapshots.Snapshot{
			{ID: "old", ShortID: "old12345", Hostname: "h", Time: base.Add(-48 * time.Hour), Tags: []string{"source"}},
			{ID: "new", ShortID: "new12345", Hostname: "h", Time: base, Tags: []string{"source"}},
		},
	}
	out, err := runCmd([]string{"list", "--after", "2025-06-14"}, c)
	require.NoError(t, err)
	assert.Contains(t, out, "new12345")
	assert.NotContains(t, out, "old12345")
}

func TestCLI_ListInvalidAfterDate(t *testing.T) {
	_, err := runCmd([]string{"list", "--after", "not-a-date"}, &stubClient{})
	require.Error(t, err)
}

// --- ls tests ---

func TestCLI_LSEmpty(t *testing.T) {
	c := &stubClient{
		files: map[string][]snapshots.TreeNode{"snap0001": nil},
	}
	out, err := runCmd([]string{"ls", "snap0001"}, c)
	require.NoError(t, err)
	assert.Contains(t, out, "(empty)")
}

func TestCLI_LSShowsSealedLabel(t *testing.T) {
	c := &stubClient{
		files: map[string][]snapshots.TreeNode{
			"snap0001": {
				{Name: "creds.age", Path: "/creds.age", Type: "file"},
			},
		},
	}
	out, err := runCmd([]string{"ls", "snap0001"}, c)
	require.NoError(t, err)
	assert.Contains(t, out, "[sealed]")
}

func TestCLI_LSJson(t *testing.T) {
	c := &stubClient{
		files: map[string][]snapshots.TreeNode{
			"snap0001": {
				{Name: "main.go", Path: "/main.go", Type: "file", Size: 512},
			},
		},
	}
	out, err := runCmd([]string{"ls", "snap0001", "--json"}, c)
	require.NoError(t, err)
	var nodes []snapshots.TreeNode
	require.NoError(t, json.Unmarshal([]byte(out), &nodes))
	assert.Len(t, nodes, 1)
	assert.Equal(t, "main.go", nodes[0].Name)
}

// --- find tests ---

func TestCLI_FindEmpty(t *testing.T) {
	out, err := runCmd([]string{"find", "*.go"}, &stubClient{})
	require.NoError(t, err)
	assert.Contains(t, out, "No matches found")
}

func TestCLI_FindShowsResults(t *testing.T) {
	c := &stubClient{
		findRes: []snapshots.FindResult{
			{
				Snapshot: snapshots.Snapshot{ShortID: "snap0001", Time: time.Now(), Hostname: "h"},
				Matches: []snapshots.TreeNode{
					{Path: "/code/main.go", Type: "file"},
				},
			},
		},
	}
	out, err := runCmd([]string{"find", "main.go"}, c)
	require.NoError(t, err)
	assert.Contains(t, out, "snap0001")
	assert.Contains(t, out, "/code/main.go")
}

func TestCLI_FindSealedShowsLabel(t *testing.T) {
	c := &stubClient{
		findRes: []snapshots.FindResult{
			{
				Snapshot: snapshots.Snapshot{ShortID: "snap0001", Time: time.Now(), Hostname: "h"},
				Matches: []snapshots.TreeNode{
					{Path: "/creds.age", Type: "file", IsSealed: true},
				},
			},
		},
	}
	out, err := runCmd([]string{"find", "creds.age"}, c)
	require.NoError(t, err)
	assert.Contains(t, out, "[sealed]")
}

func TestCLI_FindJSON(t *testing.T) {
	c := &stubClient{
		findRes: []snapshots.FindResult{
			{
				Snapshot: snapshots.Snapshot{ShortID: "snap0001"},
				Matches:  []snapshots.TreeNode{{Path: "/a.go"}},
			},
		},
	}
	out, err := runCmd([]string{"find", "*.go", "--json"}, c)
	require.NoError(t, err)
	var results []snapshots.FindResult
	require.NoError(t, json.Unmarshal([]byte(out), &results))
	assert.Len(t, results, 1)
}

// --- diff tests ---

func TestCLI_DiffEmpty(t *testing.T) {
	out, err := runCmd([]string{"diff", "aaaa1111", "bbbb2222"}, &stubClient{})
	require.NoError(t, err)
	assert.Contains(t, out, "No differences")
}

func TestCLI_DiffShowsChanges(t *testing.T) {
	c := &stubClient{
		diffRes: []snapshots.DiffEntry{
			{Path: "/added.txt", ChangeType: "added"},
			{Path: "/removed.txt", ChangeType: "removed"},
			{Path: "/changed.txt", ChangeType: "changed"},
		},
	}
	out, err := runCmd([]string{"diff", "aaaa1111", "bbbb2222"}, c)
	require.NoError(t, err)
	assert.True(t, strings.Contains(out, "+") || strings.Contains(out, "added.txt"), "added file symbol")
	assert.Contains(t, out, "/removed.txt")
	assert.Contains(t, out, "/changed.txt")
}

func TestCLI_DiffSealedShowsLabel(t *testing.T) {
	c := &stubClient{
		diffRes: []snapshots.DiffEntry{
			{Path: "/secrets.tar.age", ChangeType: "changed", IsSealed: true},
		},
	}
	out, err := runCmd([]string{"diff", "aaaa1111", "bbbb2222"}, c)
	require.NoError(t, err)
	assert.Contains(t, out, "[sealed]")
}

func TestCLI_DiffJSON(t *testing.T) {
	c := &stubClient{
		diffRes: []snapshots.DiffEntry{
			{Path: "/a.txt", ChangeType: "added"},
		},
	}
	out, err := runCmd([]string{"diff", "aaaa1111", "bbbb2222", "--json"}, c)
	require.NoError(t, err)
	var entries []snapshots.DiffEntry
	require.NoError(t, json.Unmarshal([]byte(out), &entries))
	assert.Len(t, entries, 1)
	assert.Equal(t, "added", entries[0].ChangeType)
}

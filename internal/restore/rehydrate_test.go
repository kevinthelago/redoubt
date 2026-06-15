package restore_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kevinthelago/redoubt/internal/restore"
)

// TestRehydrateNotAutomatic verifies that database dumps are DETECTED but NOT
// automatically imported — the operator must explicitly type "yes".
func TestRehydrateDetection(t *testing.T) {
	dir := t.TempDir()

	dumpPath := filepath.Join(dir, "backup.sql")
	require.NoError(t, os.WriteFile(dumpPath, []byte("-- SQL dump"), 0600))

	id := freshIdentity(t)
	entries := []restore.FileEntry{
		{Path: "backup.sql", Type: "file", Size: 11},
	}
	backend := &mockBackend{
		listFilesFn: func(_ context.Context, _ string, _ []string) ([]restore.FileEntry, error) {
			return entries, nil
		},
		restoreFn: func(_ context.Context, _, _ string, _ []string, _ bool) error {
			return nil
		},
	}
	out := &bytes.Buffer{}
	p := newTestPipeline(t, backend, &mockUnsealer{}, &mockKeys{identity: id}, &mockEscrow{}, &mockBrowser{}, &bytes.Buffer{}, out)

	result, err := p.Run(context.Background(), restore.Options{
		SnapshotID: "latest",
		TargetDir:  dir,
		Overwrite:  true, // dir has backup.sql pre-written
		NoVerify:   true,
	})
	require.NoError(t, err)

	// Dumps should be detected and returned in the result — not auto-imported.
	assert.NotEmpty(t, result.DBDumps, "backup.sql should be detected as a DB dump")
	dump := result.DBDumps[0]
	assert.Contains(t, dump.Path, "backup.sql")
	assert.Equal(t, "database", dump.Category)
	assert.NotEmpty(t, dump.RehydratCmd, "should have a preset import command for .sql files")
}

// TestRehydrateSkippedWithoutYes confirms that answering anything other than
// "yes" (including "y", "no", "YES") skips the import.
func TestRehydrateSkippedWithoutYes(t *testing.T) {
	dir := t.TempDir()
	dumps := []restore.DBDump{
		{
			Path:        filepath.Join(dir, "db.sql"),
			Category:    "database",
			RehydratCmd: "echo should-not-run",
		},
	}

	for _, answer := range []string{"y", "no", "YES", "n", ""} {
		out := &bytes.Buffer{}
		in := bytes.NewBufferString(answer + "\n")
		p := newTestPipeline(t, &mockBackend{}, &mockUnsealer{}, &mockKeys{identity: freshIdentity(t)}, &mockEscrow{}, &mockBrowser{}, in, out)

		err := p.OfferRehydrate(context.Background(), dumps)
		require.NoError(t, err, "answer=%q should not error", answer)
		assert.Contains(t, out.String(), "skipped", "answer=%q should skip import", answer)
	}
}

// TestRehydrateRunsOnYes verifies that typing exactly "yes" triggers the
// import command. We use "echo" as the command so no real DB is needed.
func TestRehydrateRunsOnYes(t *testing.T) {
	dir := t.TempDir()
	dumps := []restore.DBDump{
		{
			Path:        filepath.Join(dir, "db.sql"),
			Category:    "database",
			RehydratCmd: "echo rehydrate-ran",
		},
	}

	out := &bytes.Buffer{}
	in := bytes.NewBufferString("yes\n")
	p := newTestPipeline(t, &mockBackend{}, &mockUnsealer{}, &mockKeys{identity: freshIdentity(t)}, &mockEscrow{}, &mockBrowser{}, in, out)

	err := p.OfferRehydrate(context.Background(), dumps)
	require.NoError(t, err)
	assert.Contains(t, out.String(), "rehydrate-ran")
}

// TestRehydrateNoCommand verifies that dumps without a preset command give a
// "import manually" message instead of erroring.
func TestRehydrateNoPreset(t *testing.T) {
	dir := t.TempDir()
	dumps := []restore.DBDump{
		{
			Path:        filepath.Join(dir, "app.sqlite3"),
			Category:    "database",
			RehydratCmd: "", // SQLite — no import needed
		},
	}

	out := &bytes.Buffer{}
	p := newTestPipeline(t, &mockBackend{}, &mockUnsealer{}, &mockKeys{identity: freshIdentity(t)}, &mockEscrow{}, &mockBrowser{}, &bytes.Buffer{}, out)

	err := p.OfferRehydrate(context.Background(), dumps)
	require.NoError(t, err)
	assert.Contains(t, out.String(), "manually")
}

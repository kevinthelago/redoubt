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

func TestVerifyPass(t *testing.T) {
	id := freshIdentity(t)
	dir := t.TempDir()

	// Write files that the "restore" will have placed.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("hello world"), 0600))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "src"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src", "main.go"), []byte("package main"), 0600))

	entries := []restore.FileEntry{
		{Path: "readme.txt", Type: "file", Size: 11},
		{Path: "src/main.go", Type: "file", Size: 12},
	}
	backend := &mockBackend{
		listFilesFn: func(_ context.Context, _ string, _ []string) ([]restore.FileEntry, error) {
			return entries, nil
		},
		restoreFn: func(_ context.Context, _, _ string, _ []string, _ bool) error {
			return nil // files already written above
		},
	}

	out := &bytes.Buffer{}
	p := newTestPipeline(t, backend, &mockUnsealer{}, &mockKeys{identity: id}, &mockEscrow{}, &mockBrowser{}, &bytes.Buffer{}, out)

	result, err := p.Run(context.Background(), restore.Options{
		SnapshotID: "latest",
		TargetDir:  dir,
		Overwrite:  true, // dir is pre-populated (files written before restore call)
	})
	require.NoError(t, err)
	assert.True(t, result.Verified, "verification should pass when all files present")
	assert.Empty(t, result.VerifyErrors)
	assert.Contains(t, out.String(), "verification")
}

func TestVerifyMissingFile(t *testing.T) {
	id := freshIdentity(t)
	dir := t.TempDir()

	// Snapshot says two files but restore only delivers one.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "present.txt"), []byte("here"), 0600))

	entries := []restore.FileEntry{
		{Path: "present.txt", Type: "file", Size: 4},
		{Path: "missing.txt", Type: "file", Size: 100},
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
		Overwrite:  true, // dir has present.txt pre-written
	})
	require.NoError(t, err)
	assert.False(t, result.Verified)

	found := false
	for _, ve := range result.VerifyErrors {
		if ve.Path == "missing.txt" {
			found = true
		}
	}
	assert.True(t, found, "missing.txt should appear in VerifyErrors")
	assert.Contains(t, out.String(), "VERIFICATION MISMATCH")
}

func TestVerifySkippedWithNoVerify(t *testing.T) {
	id := freshIdentity(t)
	dir := t.TempDir()

	// Don't write the expected file; but with NoVerify the missing file won't
	// be flagged.
	entries := []restore.FileEntry{
		{Path: "secret.txt", Type: "file", Size: 50},
	}
	backend := &mockBackend{
		listFilesFn: func(_ context.Context, _ string, _ []string) ([]restore.FileEntry, error) {
			return entries, nil
		},
	}

	p := newTestPipeline(t, backend, &mockUnsealer{}, &mockKeys{identity: id}, &mockEscrow{}, &mockBrowser{}, &bytes.Buffer{}, &bytes.Buffer{})

	result, err := p.Run(context.Background(), restore.Options{
		SnapshotID: "latest",
		TargetDir:  dir,
		NoVerify:   true,
	})
	require.NoError(t, err)
	assert.False(t, result.Verified, "Verified should be false when skipped")
	assert.Empty(t, result.VerifyErrors, "no verify errors when NoVerify is set")
}

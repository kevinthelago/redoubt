package restore_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	goage "filippo.io/age"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kevinthelago/redoubt/internal/restore"
)

// ── Mocks ─────────────────────────────────────────────────────────────────────

type mockBackend struct {
	restoreFn   func(ctx context.Context, snapshotID, targetDir string, includes []string, verify bool) error
	listFilesFn func(ctx context.Context, snapshotID string, includes []string) ([]restore.FileEntry, error)
	latestFn    func(ctx context.Context) (string, error)
}

func (m *mockBackend) Restore(ctx context.Context, sid, dir string, inc []string, v bool) error {
	if m.restoreFn != nil {
		return m.restoreFn(ctx, sid, dir, inc, v)
	}
	return nil
}
func (m *mockBackend) ListFiles(ctx context.Context, sid string, inc []string) ([]restore.FileEntry, error) {
	if m.listFilesFn != nil {
		return m.listFilesFn(ctx, sid, inc)
	}
	return nil, nil
}
func (m *mockBackend) LatestSnapshot(ctx context.Context) (string, error) {
	if m.latestFn != nil {
		return m.latestFn(ctx)
	}
	return "abc123", nil
}

type mockUnsealer struct {
	unsealFn func(identity goage.Identity, src, dst string) error
	sealed   map[string]bool
}

func (m *mockUnsealer) UnsealFile(identity goage.Identity, src, dst string) error {
	if m.unsealFn != nil {
		return m.unsealFn(identity, src, dst)
	}
	return nil
}
func (m *mockUnsealer) IsSealed(path string) bool {
	if m.sealed != nil {
		return m.sealed[path]
	}
	return len(path) > 4 && path[len(path)-4:] == ".age"
}
func (m *mockUnsealer) PlainPath(sealed string) string {
	if len(sealed) > 4 && sealed[len(sealed)-4:] == ".age" {
		return sealed[:len(sealed)-4]
	}
	return sealed
}

type mockKeys struct {
	identity goage.Identity
	err      error
}

func (m *mockKeys) MasterIdentity(_ context.Context) (goage.Identity, error) {
	return m.identity, m.err
}

type mockEscrow struct {
	threshold int
	total     int
	identity  goage.Identity
	err       error
}

func (m *mockEscrow) Info() (int, int) { return m.threshold, m.total }
func (m *mockEscrow) Reconstruct(_ context.Context, _ [][]byte) (goage.Identity, error) {
	return m.identity, m.err
}

type mockBrowser struct {
	latestFn func(ctx context.Context, source restore.Source) (*restore.SnapshotMeta, error)
	getFn    func(ctx context.Context, source restore.Source, id string) (*restore.SnapshotMeta, error)
}

func (m *mockBrowser) Latest(ctx context.Context, src restore.Source) (*restore.SnapshotMeta, error) {
	if m.latestFn != nil {
		return m.latestFn(ctx, src)
	}
	return &restore.SnapshotMeta{ID: "abc123", ShortID: "abc123", Time: time.Now()}, nil
}
func (m *mockBrowser) Get(ctx context.Context, src restore.Source, id string) (*restore.SnapshotMeta, error) {
	if m.getFn != nil {
		return m.getFn(ctx, src, id)
	}
	return &restore.SnapshotMeta{ID: id, ShortID: id[:8], Time: time.Now()}, nil
}

// ── Test helpers ──────────────────────────────────────────────────────────────

func freshIdentity(t *testing.T) *goage.X25519Identity {
	t.Helper()
	id, err := goage.GenerateX25519Identity()
	require.NoError(t, err)
	return id
}

func newTestPipeline(
	t *testing.T,
	backend restore.ResticBackend,
	unsealer restore.FileUnsealer,
	keys restore.KeyProvider,
	escrow restore.EscrowProvider,
	browser restore.SnapshotBrowser,
	in *bytes.Buffer,
	out *bytes.Buffer,
) *restore.Pipeline {
	t.Helper()
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	return restore.NewPipeline(backend, unsealer, keys, escrow, browser, log, in, out)
}

// ── Tests ─────────────────────────────────────────────────────────────────────

func TestRestorePipeline_DryRun(t *testing.T) {
	id := freshIdentity(t)
	backend := &mockBackend{
		listFilesFn: func(_ context.Context, _ string, _ []string) ([]restore.FileEntry, error) {
			return []restore.FileEntry{
				{Path: "docs/readme.txt", Type: "file", Size: 1024},
				{Path: "src/main.go", Type: "file", Size: 4096},
			}, nil
		},
	}
	keys := &mockKeys{identity: id}
	browser := &mockBrowser{}
	out := &bytes.Buffer{}
	in := &bytes.Buffer{}

	p := newTestPipeline(t, backend, &mockUnsealer{}, keys, &mockEscrow{}, browser, in, out)

	dir := t.TempDir()
	result, err := p.Run(context.Background(), restore.Options{
		SnapshotID: "latest",
		TargetDir:  dir,
		DryRun:     true,
	})
	require.NoError(t, err)
	assert.True(t, result.DryRun)
	assert.Equal(t, 2, result.FilesRestored)
	assert.Equal(t, int64(5120), result.BytesRestored)
}

func TestRestorePipeline_NonDestructiveDefault(t *testing.T) {
	// A non-empty target directory without --overwrite must fail pre-flight.
	id := freshIdentity(t)
	dir := t.TempDir()

	// Put a file in it.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "existing.txt"), []byte("data"), 0600))

	p := newTestPipeline(t,
		&mockBackend{},
		&mockUnsealer{},
		&mockKeys{identity: id},
		&mockEscrow{},
		&mockBrowser{},
		&bytes.Buffer{},
		&bytes.Buffer{},
	)

	_, err := p.Run(context.Background(), restore.Options{
		SnapshotID: "latest",
		TargetDir:  dir,
		Overwrite:  false,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not empty")
}

func TestRestorePipeline_OverwriteAllowed(t *testing.T) {
	id := freshIdentity(t)
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "existing.txt"), []byte("data"), 0600))

	restored := false
	backend := &mockBackend{
		restoreFn: func(_ context.Context, _, _ string, _ []string, _ bool) error {
			restored = true
			return nil
		},
	}
	out := &bytes.Buffer{}
	p := newTestPipeline(t, backend, &mockUnsealer{}, &mockKeys{identity: id}, &mockEscrow{}, &mockBrowser{}, &bytes.Buffer{}, out)

	_, err := p.Run(context.Background(), restore.Options{
		SnapshotID: "latest",
		TargetDir:  dir,
		Overwrite:  true,
		NoVerify:   true,
	})
	require.NoError(t, err)
	assert.True(t, restored)
}

func TestRestorePipeline_RestoreError(t *testing.T) {
	id := freshIdentity(t)
	resticErr := errors.New("restic: repository not found")
	backend := &mockBackend{
		restoreFn: func(_ context.Context, _, _ string, _ []string, _ bool) error {
			return resticErr
		},
	}
	dir := t.TempDir()
	p := newTestPipeline(t, backend, &mockUnsealer{}, &mockKeys{identity: id}, &mockEscrow{}, &mockBrowser{}, &bytes.Buffer{}, &bytes.Buffer{})

	_, err := p.Run(context.Background(), restore.Options{
		SnapshotID: "latest",
		TargetDir:  dir,
		NoVerify:   true,
	})
	require.Error(t, err)
	assert.ErrorContains(t, err, "restic restore")
}

func TestRestorePipeline_UnsealsSecrets(t *testing.T) {
	id := freshIdentity(t)
	dir := t.TempDir()

	var unsealedSrc, unsealedDst string
	unsealer := &mockUnsealer{
		// No explicit sealed map — IsSealed uses the .age extension check.
		unsealFn: func(_ goage.Identity, src, dst string) error {
			unsealedSrc = src
			unsealedDst = dst
			// Simulate unsealing: write the "plain" file and remove the .age file.
			os.WriteFile(dst, []byte("plaintext"), 0600)
			os.Remove(src)
			return nil
		},
	}

	backend := &mockBackend{
		restoreFn: func(_ context.Context, _, targetDir string, _ []string, _ bool) error {
			// Simulate restic creating the sealed secret in the target.
			subDir := filepath.Join(targetDir, "secrets")
			os.MkdirAll(subDir, 0755)
			return os.WriteFile(filepath.Join(subDir, "key.txt.age"), []byte("sealed"), 0600)
		},
	}

	p := newTestPipeline(t, backend, unsealer, &mockKeys{identity: id}, &mockEscrow{}, &mockBrowser{}, &bytes.Buffer{}, &bytes.Buffer{})

	result, err := p.Run(context.Background(), restore.Options{
		SnapshotID: "latest",
		TargetDir:  dir,
		NoVerify:   true,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, result.SealedFiles, "one file should be unsealed")
	assert.True(t, strings.HasSuffix(unsealedSrc, ".age"), "unsealed src must end in .age")
	assert.Equal(t, strings.TrimSuffix(unsealedSrc, ".age"), unsealedDst)
}

func TestRestorePipeline_VerificationMismatch(t *testing.T) {
	id := freshIdentity(t)
	dir := t.TempDir()

	// Provide a file list that references a file that won't exist after restore.
	entries := []restore.FileEntry{
		{Path: "missing.txt", Type: "file", Size: 100},
	}
	backend := &mockBackend{
		listFilesFn: func(_ context.Context, _ string, _ []string) ([]restore.FileEntry, error) {
			return entries, nil
		},
		restoreFn: func(_ context.Context, _, _ string, _ []string, _ bool) error {
			return nil // simulate restore but don't actually create the file
		},
	}
	out := &bytes.Buffer{}
	p := newTestPipeline(t, backend, &mockUnsealer{}, &mockKeys{identity: id}, &mockEscrow{}, &mockBrowser{}, &bytes.Buffer{}, out)

	result, err := p.Run(context.Background(), restore.Options{
		SnapshotID: "latest",
		TargetDir:  dir,
	})
	require.NoError(t, err)
	assert.False(t, result.Verified)
	assert.NotEmpty(t, result.VerifyErrors)
	assert.Contains(t, out.String(), "VERIFICATION MISMATCH")
}

func TestRestorePipeline_SelectiveIncludes(t *testing.T) {
	id := freshIdentity(t)
	dir := t.TempDir()

	var capturedIncludes []string
	backend := &mockBackend{
		restoreFn: func(_ context.Context, _, _ string, includes []string, _ bool) error {
			capturedIncludes = includes
			return nil
		},
	}
	p := newTestPipeline(t, backend, &mockUnsealer{}, &mockKeys{identity: id}, &mockEscrow{}, &mockBrowser{}, &bytes.Buffer{}, &bytes.Buffer{})

	_, err := p.Run(context.Background(), restore.Options{
		SnapshotID: "latest",
		TargetDir:  dir,
		Includes:   []string{"/home/user/docs"},
		NoVerify:   true,
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"/home/user/docs"}, capturedIncludes)
}

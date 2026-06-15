package drill

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// fakeRestorer is a test double for the Restorer interface.
type fakeRestorer struct {
	snapshots   map[Source]string   // source → snapshot ID
	files       map[string][]string // snapshotID → file paths (relative)
	fileContent map[string][]byte   // relative path → content
	freeSpace   uint64
	restoreErr  error
}

func (f *fakeRestorer) LatestSnapshotID(_ context.Context, src Source) (string, bool, error) {
	id, ok := f.snapshots[src]
	return id, ok, nil
}

func (f *fakeRestorer) ListSnapshotFiles(_ context.Context, src Source, snapshotID, _ string) ([]string, error) {
	return f.files[snapshotID], nil
}

func (f *fakeRestorer) RestoreFiles(_ context.Context, _ Source, snapshotID string, paths []string, targetDir string) error {
	if f.restoreErr != nil {
		return f.restoreErr
	}
	for _, p := range paths {
		content, ok := f.fileContent[p]
		if !ok {
			content = []byte("default content for " + p)
		}
		dest := filepath.Join(targetDir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(dest), 0700); err != nil {
			return err
		}
		if err := os.WriteFile(dest, content, 0600); err != nil {
			return err
		}
	}
	return nil
}

func (f *fakeRestorer) FreeSpaceAt(_ string) (uint64, error) {
	return f.freeSpace, nil
}

// buildTestManifest writes ManifestFileName into targetDir with the SHA-256s of the
// files that will be restored from fileContent.
func buildTestManifest(t *testing.T, snapshotFiles map[string][]byte, targetDir string) {
	t.Helper()
	m := &SnapshotManifest{CreatedAt: time.Now(), Files: make(map[string]string)}
	for rel, content := range snapshotFiles {
		if rel == ManifestFileName {
			continue
		}
		hash, err := hashBytes(content)
		if err != nil {
			t.Fatal(err)
		}
		m.Files[rel] = hash
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	snapshotFiles[ManifestFileName] = data
}

func hashBytes(b []byte) (string, error) {
	tmp, err := os.CreateTemp("", "hashme-*")
	if err != nil {
		return "", err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	return ComputeFileSHA256(name)
}

func newTestEngine(t *testing.T, r *fakeRestorer) (*Engine, string) {
	t.Helper()
	dir := t.TempDir()
	cfg := Config{
		ScratchBaseDir:    filepath.Join(dir, "scratch"),
		HistoryPath:       filepath.Join(dir, "history.json"),
		StatusPath:        filepath.Join(dir, "status.json"),
		MaxSampleFiles:    5,
		MinFreeSpaceBytes: 1, // small for tests
		HistoryMaxLen:     50,
	}
	e := NewEngine(cfg, r, nil, slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})))
	return e, dir
}

func TestEnginePassDrill(t *testing.T) {
	fileContent := map[string][]byte{
		CanaryFileName: []byte("redoubt canary v1"),
		"source/a.go":  []byte("package main"),
	}
	buildTestManifest(t, fileContent, "")

	r := &fakeRestorer{
		snapshots:   map[Source]string{SourceVault: "snap-abc"},
		files:       map[string][]string{"snap-abc": {CanaryFileName, ManifestFileName, "source/a.go"}},
		fileContent: fileContent,
		freeSpace:   10 * 1024 * 1024 * 1024,
	}
	e, dir := newTestEngine(t, r)

	result, err := e.Run(context.Background(), RunOptions{Source: SourceVault})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.Status != StatusPass {
		t.Errorf("expected StatusPass, got %q — %s", result.Status, result.FailureMsg)
	}
	if result.Verified <= 0 {
		t.Errorf("expected verified > 0, got %d", result.Verified)
	}

	// History should have one entry.
	h, err := LoadHistory(filepath.Join(dir, "history.json"))
	if err != nil || len(h.Results) != 1 {
		t.Errorf("expected 1 history entry, got %d (err=%v)", len(h.Results), err)
	}

	// Status file should exist.
	if _, statErr := os.Stat(filepath.Join(dir, "status.json")); statErr != nil {
		t.Errorf("status.json not written: %v", statErr)
	}
}

func TestEngineSkipNoSnapshots(t *testing.T) {
	r := &fakeRestorer{
		snapshots: map[Source]string{}, // no snapshots
		freeSpace: 10 * 1024 * 1024 * 1024,
	}
	e, _ := newTestEngine(t, r)

	result, err := e.Run(context.Background(), RunOptions{Source: SourceVault})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != StatusSkip {
		t.Errorf("expected StatusSkip, got %q", result.Status)
	}
	if result.SkipReason == "" {
		t.Error("expected non-empty SkipReason")
	}
}

func TestEngineSkipLowSpace(t *testing.T) {
	r := &fakeRestorer{
		snapshots: map[Source]string{SourceVault: "snap-abc"},
		freeSpace: 0, // 0 bytes — always below MinFreeSpaceBytes
	}
	e, _ := newTestEngine(t, r)

	result, err := e.Run(context.Background(), RunOptions{Source: SourceVault})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != StatusSkip {
		t.Errorf("expected StatusSkip for low space, got %q", result.Status)
	}
}

func TestEngineFailOnRestoreError(t *testing.T) {
	r := &fakeRestorer{
		snapshots:  map[Source]string{SourceVault: "snap-abc"},
		files:      map[string][]string{"snap-abc": {CanaryFileName}},
		freeSpace:  10 * 1024 * 1024 * 1024,
		restoreErr: fmt.Errorf("restic: connection refused"),
	}
	e, _ := newTestEngine(t, r)

	var alerted bool
	e.alerter = alertFunc(func(Result) { alerted = true })

	result, err := e.Run(context.Background(), RunOptions{Source: SourceVault})
	if err != nil {
		t.Fatalf("unexpected infrastructure error: %v", err)
	}
	if result.Status != StatusFail {
		t.Errorf("expected StatusFail on restore error, got %q", result.Status)
	}
	if !alerted {
		t.Error("expected alerter to be called on failure")
	}
}

func TestEngineFailOnHashMismatch(t *testing.T) {
	fileContent := map[string][]byte{
		CanaryFileName: []byte("canary"),
		"data.txt":     []byte("original"),
	}
	buildTestManifest(t, fileContent, "")

	// Tamper: the restorer will write different content than what's in the manifest.
	tamperedContent := map[string][]byte{
		CanaryFileName:   []byte("canary"),
		ManifestFileName: fileContent[ManifestFileName], // keep manifest honest
		"data.txt":       []byte("tampered!"),           // mismatch vs manifest
	}

	r := &fakeRestorer{
		snapshots:   map[Source]string{SourceVault: "snap-x"},
		files:       map[string][]string{"snap-x": {CanaryFileName, ManifestFileName, "data.txt"}},
		fileContent: tamperedContent,
		freeSpace:   10 * 1024 * 1024 * 1024,
	}
	e, _ := newTestEngine(t, r)

	result, err := e.Run(context.Background(), RunOptions{Source: SourceVault})
	if err != nil {
		t.Fatalf("unexpected infrastructure error: %v", err)
	}
	if result.Status != StatusFail {
		t.Errorf("expected StatusFail on hash mismatch, got %q", result.Status)
	}
	if result.Failed <= 0 {
		t.Errorf("expected Failed > 0, got %d", result.Failed)
	}
}

// alertFunc implements Alerter using a plain function.
type alertFunc func(Result)

func (f alertFunc) DrillFailed(r Result) { f(r) }

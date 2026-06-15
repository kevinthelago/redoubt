package drill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestComputeFileSHA256(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(p, []byte("hello redoubt"), 0600); err != nil {
		t.Fatal(err)
	}
	hash, err := ComputeFileSHA256(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(hash) != 64 {
		t.Fatalf("expected 64-char hex SHA-256, got %q (len %d)", hash, len(hash))
	}
	// Same file → same hash.
	hash2, _ := ComputeFileSHA256(p)
	if hash != hash2 {
		t.Errorf("non-deterministic hash: %q vs %q", hash, hash2)
	}
}

func TestBuildAndVerifyManifest(t *testing.T) {
	src := t.TempDir()
	files := map[string]string{
		"a.txt":     "content of a",
		"sub/b.txt": "content of b",
	}
	for rel, content := range files {
		full := filepath.Join(src, filepath.FromSlash(rel))
		_ = os.MkdirAll(filepath.Dir(full), 0700)
		if err := os.WriteFile(full, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}

	m, err := BuildManifest(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Files) != len(files) {
		t.Fatalf("expected %d manifest entries, got %d", len(files), len(m.Files))
	}

	// Verify against itself → all pass.
	verified, failures := m.Verify(src)
	if len(failures) != 0 {
		t.Errorf("unexpected failures: %v", failures)
	}
	if verified != len(files) {
		t.Errorf("expected %d verified, got %d", len(files), verified)
	}

	// Corrupt one file → expect one failure.
	if err := os.WriteFile(filepath.Join(src, "a.txt"), []byte("tampered"), 0600); err != nil {
		t.Fatal(err)
	}
	_, failures = m.Verify(src)
	if len(failures) != 1 {
		t.Errorf("expected 1 failure after corruption, got %d: %v", len(failures), failures)
	}
	if !strings.Contains(failures[0], "SHA-256 mismatch") {
		t.Errorf("failure message should mention SHA-256 mismatch: %q", failures[0])
	}
}

func TestVerifyMissingFile(t *testing.T) {
	dir := t.TempDir()
	m := &SnapshotManifest{Files: map[string]string{"gone.txt": "abc123"}}
	_, failures := m.Verify(dir)
	if len(failures) == 0 {
		t.Error("expected failure for missing file, got none")
	}
}

package drill

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// ManifestFileName is the name of the manifest file included in every redoubt snapshot.
// The backup stream writes this file; the drill engine reads it.
const ManifestFileName = ".redoubt-manifest.json"

// CanaryFileName is the fixed canary file included in every redoubt snapshot.
const CanaryFileName = ".redoubt-canary"

// SnapshotManifest is stored as ManifestFileName inside each restic snapshot.
// The backup stream writes it at capture time; the drill engine verifies against it.
type SnapshotManifest struct {
	CreatedAt time.Time         `json:"created_at"`
	Files     map[string]string `json:"files"` // relative path → hex SHA-256
}

// LoadManifestFromDir reads and parses ManifestFileName from dir.
func LoadManifestFromDir(dir string) (*SnapshotManifest, error) {
	path := filepath.Join(dir, ManifestFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read manifest %q: %w", path, err)
	}
	var m SnapshotManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	return &m, nil
}

// ComputeFileSHA256 returns the hex-encoded SHA-256 of the file at path.
func ComputeFileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open %q: %w", path, err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("hash %q: %w", path, err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Verify checks that every file listed in m can be found under restoreDir and
// matches its expected SHA-256. It returns a slice of per-file error strings
// (non-nil on mismatch or missing file) and the count of files verified OK.
func (m *SnapshotManifest) Verify(restoreDir string) (verified int, failures []string) {
	for rel, expected := range m.Files {
		full := filepath.Join(restoreDir, filepath.FromSlash(rel))
		got, err := ComputeFileSHA256(full)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: cannot hash: %v", rel, err))
			continue
		}
		if got != expected {
			failures = append(failures,
				fmt.Sprintf("%s: SHA-256 mismatch (want %.8s… got %.8s…)", rel, expected, got))
			continue
		}
		verified++
	}
	return verified, failures
}

// VerifyFiles checks only the specified restored paths against m.
// Paths not in the manifest are skipped (not a failure), except for the canary
// which must always be in the manifest. ManifestFileName itself is always skipped.
func (m *SnapshotManifest) VerifyFiles(restoreDir string, paths []string) (verified int, failures []string) {
	for _, rel := range paths {
		if filepath.Base(rel) == ManifestFileName {
			continue
		}
		expected, ok := m.Files[rel]
		if !ok {
			if filepath.Base(rel) == CanaryFileName {
				failures = append(failures,
					fmt.Sprintf("canary %q not found in snapshot manifest — backup may be missing canary support", rel))
			}
			continue
		}
		full := filepath.Join(restoreDir, filepath.FromSlash(rel))
		got, err := ComputeFileSHA256(full)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: cannot hash restored file: %v", rel, err))
			continue
		}
		if got != expected {
			failures = append(failures,
				fmt.Sprintf("%s: SHA-256 mismatch (want %.8s… got %.8s…)", rel, expected, got))
			continue
		}
		verified++
	}
	return verified, failures
}

// BuildManifest walks dir and produces a SnapshotManifest from all files found.
// Used by tests and the backup stream to generate manifests.
func BuildManifest(dir string) (*SnapshotManifest, error) {
	m := &SnapshotManifest{
		CreatedAt: time.Now(),
		Files:     make(map[string]string),
	}
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		hash, err := ComputeFileSHA256(path)
		if err != nil {
			return err
		}
		m.Files[filepath.ToSlash(rel)] = hash
		return nil
	})
	return m, err
}

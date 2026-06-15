package assets

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const assetsFileName = "assets.json"

// Logger is the structured logging interface consumed by this package.
// Foundation provides the concrete implementation via internal/logging.
type Logger interface {
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
	Debug(msg string, args ...any)
}

type nopLogger struct{}

func (nopLogger) Info(string, ...any)  {}
func (nopLogger) Warn(string, ...any)  {}
func (nopLogger) Error(string, ...any) {}
func (nopLogger) Debug(string, ...any) {}

// NopLogger returns a Logger that discards all output.
func NopLogger() Logger { return nopLogger{} }

// Store persists the set of tracked assets to {dataDir}/assets.json.
type Store struct {
	dataDir string
	log     Logger
	items   []Asset
}

// NewStore creates a Store backed by dataDir.
// Call Load to read any previously persisted assets.
func NewStore(dataDir string, log Logger) *Store {
	if log == nil {
		log = NopLogger()
	}
	return &Store{dataDir: dataDir, log: log}
}

// Load reads assets.json from the data directory.
// A missing file is not an error; it simply means no assets have been tracked yet.
func (s *Store) Load() error {
	path := filepath.Join(s.dataDir, assetsFileName)
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("reading assets file: %w", err)
	}
	var items []Asset
	if err := json.Unmarshal(data, &items); err != nil {
		return fmt.Errorf("parsing assets file: %w", err)
	}
	s.items = items
	return nil
}

// Save writes the current tracked set to assets.json, creating the data
// directory if necessary.
func (s *Store) Save() error {
	if err := os.MkdirAll(s.dataDir, 0o700); err != nil {
		return fmt.Errorf("creating data directory: %w", err)
	}
	data, err := json.MarshalIndent(s.items, "", "  ")
	if err != nil {
		return fmt.Errorf("serialising assets: %w", err)
	}
	path := filepath.Join(s.dataDir, assetsFileName)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("writing assets file: %w", err)
	}
	return nil
}

// List returns a copy of the tracked asset set.
func (s *Store) List() []Asset {
	out := make([]Asset, len(s.items))
	copy(out, s.items)
	return out
}

// IsEmpty reports whether no assets are tracked.
func (s *Store) IsEmpty() bool { return len(s.items) == 0 }

// Add validates and registers path with the given category.
//
//   - Non-existent paths are rejected.
//   - Database assets require a non-empty dumpCmd whose leading binary is on PATH.
//   - Exact-duplicate paths are silently skipped (with a warning log).
//   - Parent/child overlaps with existing assets produce a warning but are allowed.
func (s *Store) Add(path string, cat Category, dumpCmd string) error {
	if !cat.IsValid() {
		return fmt.Errorf("unknown category %q; valid values: source, secrets, database, config", cat)
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolving path: %w", err)
	}
	if _, err := os.Stat(abs); errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("path does not exist: %s", abs)
	} else if err != nil {
		return fmt.Errorf("accessing path: %w", err)
	}

	if cat == CategoryDatabase {
		if err := ValidateDumpCommand(dumpCmd); err != nil {
			return err
		}
	}

	// Exact-duplicate guard.
	for _, existing := range s.items {
		if existing.Path == abs {
			s.log.Warn("path is already tracked; skipping", "path", abs)
			return nil
		}
	}

	// Overlap check: warn when new path is inside, or contains, an existing asset.
	for _, existing := range s.items {
		if isDescendantOrEqual(abs, existing.Path) {
			s.log.Warn("new path is nested inside an already-tracked path",
				"new", abs, "existing", existing.Path)
		} else if isDescendantOrEqual(existing.Path, abs) {
			s.log.Warn("new path contains an already-tracked path",
				"new", abs, "existing", existing.Path)
		}
	}

	s.items = append(s.items, Asset{
		Path:        abs,
		Category:    cat,
		DumpCommand: dumpCmd,
		AddedAt:     time.Now().UTC(),
	})
	return s.Save()
}

// Remove removes the tracked asset matching path. Returns an error if the path
// is not currently tracked.
func (s *Store) Remove(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolving path: %w", err)
	}
	for i, a := range s.items {
		if a.Path == abs {
			s.items = append(s.items[:i], s.items[i+1:]...)
			return s.Save()
		}
	}
	return fmt.Errorf("path is not tracked: %s", abs)
}

// DiskUsage returns the total size in bytes for path (recursive for directories).
// Returns -1 if the path cannot be accessed.
func DiskUsage(path string) int64 {
	var total int64
	err := filepath.WalkDir(path, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		total += info.Size()
		return nil
	})
	if err != nil {
		return -1
	}
	return total
}

// FormatSize converts a byte count to a human-readable IEC string (e.g. "1.4 MiB").
func FormatSize(bytes int64) string {
	if bytes < 0 {
		return "?"
	}
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// isDescendantOrEqual reports whether candidate is the same path as, or
// nested under, base.
func isDescendantOrEqual(candidate, base string) bool {
	rel, err := filepath.Rel(base, candidate)
	if err != nil {
		return false
	}
	return !strings.HasPrefix(rel, "..")
}

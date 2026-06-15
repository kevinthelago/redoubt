package snapshots

import (
	"io/fs"
	"strings"
	"time"
)

// Snapshot represents a single restic snapshot.
type Snapshot struct {
	ID        string
	ShortID   string
	Time      time.Time
	Hostname  string
	Tags      []string // category tags e.g. "source", "secrets", "database", "config"
	Paths     []string
	BytesAdded int64 // deduplicated bytes added to the repo by this snapshot
}

// HasTag reports whether the snapshot carries the given category tag.
func (s Snapshot) HasTag(tag string) bool {
	for _, t := range s.Tags {
		if strings.EqualFold(t, tag) {
			return true
		}
	}
	return false
}

// IsSecretSnapshot reports whether all files within this snapshot are age-sealed
// (the entire snapshot was sealed before backup under the "secrets" category).
func (s Snapshot) IsSecretSnapshot() bool {
	return s.HasTag("secrets")
}

// TreeNode represents a file or directory entry within a snapshot.
type TreeNode struct {
	Name    string
	Path    string
	Type    string // "file", "dir", "symlink"
	Size    int64
	Mode    fs.FileMode
	ModTime time.Time
	// IsSealed is true when this entry is age-sealed and its contents must never
	// be shown — the filename is displayed but decryption is forbidden here.
	IsSealed bool
}

// sealedExtension marks individual age-sealed files.
const sealedExtension = ".age"

// isSealed reports whether a path represents an age-sealed file.
func isSealed(path string) bool {
	return strings.HasSuffix(path, sealedExtension)
}

// DiffEntry is one changed path between two snapshots.
type DiffEntry struct {
	Path       string
	ChangeType string // "added", "removed", "changed"
	IsSealed   bool
}

// FindResult groups the matching nodes found within a single snapshot.
type FindResult struct {
	Snapshot Snapshot
	Matches  []TreeNode
}

// SnapshotFilter narrows which snapshots are returned by List or Find.
// Zero values mean "no constraint" for that field.
type SnapshotFilter struct {
	Tags     []string   // AND-matched: snapshot must carry every listed tag
	Hostname string     // exact match; empty = any host
	After    time.Time  // zero = no lower bound
	Before   time.Time  // zero = no upper bound
}

// matchesFilter reports whether s satisfies f.
func (s Snapshot) matchesFilter(f SnapshotFilter) bool {
	if f.Hostname != "" && !strings.EqualFold(s.Hostname, f.Hostname) {
		return false
	}
	for _, want := range f.Tags {
		if !s.HasTag(want) {
			return false
		}
	}
	if !f.After.IsZero() && s.Time.Before(f.After) {
		return false
	}
	if !f.Before.IsZero() && s.Time.After(f.Before) {
		return false
	}
	return true
}

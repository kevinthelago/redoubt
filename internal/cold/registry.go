package cold

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/BurntSushi/toml"
)

// DriveRecord holds everything Redoubt knows about a registered cold drive.
type DriveRecord struct {
	// Label is the user-assigned friendly name (e.g. "wd-blue-1").
	// It is used as the primary key in the registry.
	Label string `toml:"label"`

	// MountPath is the expected root path of the drive when connected
	// (e.g. "E:\" on Windows or "/media/user/WD" on Linux).
	MountPath string `toml:"mount_path"`

	// RepoSubdir is the directory inside MountPath where the restic
	// repository lives. Defaults to "redoubt" if empty.
	RepoSubdir string `toml:"repo_subdir"`

	// LastCopyTime is the time of the last successful cold copy to this
	// drive. Zero means the drive has never been used for a cold copy.
	LastCopyTime time.Time `toml:"last_copy_time"`

	// LastVerifyTime is the time of the last successful restic check on
	// this drive's repository.
	LastVerifyTime time.Time `toml:"last_verify_time"`

	// TotalCopies is the lifetime count of successful cold copies to this
	// drive.
	TotalCopies int `toml:"total_copies"`

	// Active marks the drive most recently used for a cold copy. At most
	// one record in the registry has Active == true.
	Active bool `toml:"active"`
}

// RepoPath returns the full filesystem path to the restic repository on the
// drive, combining MountPath and RepoSubdir.
func (d DriveRecord) RepoPath() string {
	subdir := d.RepoSubdir
	if subdir == "" {
		subdir = "redoubt"
	}
	return d.MountPath + string(os.PathSeparator) + subdir
}

// IsMounted reports whether the drive's MountPath currently exists and is a
// directory. It performs a lightweight filesystem stat; no write is attempted.
func (d DriveRecord) IsMounted() bool {
	info, err := os.Stat(d.MountPath)
	return err == nil && info.IsDir()
}

// registryFile is the on-disk shape of the TOML registry.
type registryFile struct {
	Drives []DriveRecord `toml:"drive"`
}

// Registry manages the set of registered cold drives and persists them to a
// TOML file. All public methods are safe for concurrent use.
type Registry struct {
	path   string
	drives []DriveRecord
	mu     sync.RWMutex
}

// LoadRegistry reads (or creates) the drive registry at path. If the file
// does not exist, an empty registry is returned and the file is created on
// the first call to a mutating method.
func LoadRegistry(path string) (*Registry, error) {
	r := &Registry{path: path}
	if err := r.reload(); err != nil {
		return nil, err
	}
	return r, nil
}

// reload reads the registry from disk. The caller must not hold r.mu.
func (r *Registry) reload() error {
	data, err := os.ReadFile(r.path)
	if errors.Is(err, os.ErrNotExist) {
		// first use: start with an empty registry
		r.drives = nil
		return nil
	}
	if err != nil {
		return fmt.Errorf("cold registry: read %s: %w", r.path, err)
	}
	var f registryFile
	if _, err := toml.Decode(string(data), &f); err != nil {
		return fmt.Errorf("cold registry: parse %s: %w", r.path, err)
	}
	r.drives = f.Drives
	return nil
}

// save writes the current in-memory state to disk atomically. The caller
// must hold r.mu (write lock).
func (r *Registry) save() error {
	f, err := os.CreateTemp("", "cold-registry-*.toml")
	if err != nil {
		return fmt.Errorf("cold registry: create temp file: %w", err)
	}
	tmpName := f.Name()

	if err := toml.NewEncoder(f).Encode(registryFile{Drives: r.drives}); err != nil {
		f.Close()
		os.Remove(tmpName)
		return fmt.Errorf("cold registry: encode: %w", err)
	}
	if err := f.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("cold registry: close temp file: %w", err)
	}
	if err := os.Rename(tmpName, r.path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("cold registry: write %s: %w", r.path, err)
	}
	return nil
}

// indexOf returns the slice index of the drive with the given label, or -1.
// The caller must hold at least r.mu (read lock).
func (r *Registry) indexOf(label string) int {
	for i, d := range r.drives {
		if d.Label == label {
			return i
		}
	}
	return -1
}

// Add registers a new drive. Returns an error if a drive with the same label
// already exists.
func (r *Registry) Add(d DriveRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.indexOf(d.Label) >= 0 {
		return fmt.Errorf("cold registry: drive %q already registered", d.Label)
	}
	r.drives = append(r.drives, d)
	return r.save()
}

// Remove deletes the drive with the given label. Returns an error if not found.
func (r *Registry) Remove(label string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	i := r.indexOf(label)
	if i < 0 {
		return fmt.Errorf("cold registry: drive %q not found", label)
	}
	r.drives = append(r.drives[:i], r.drives[i+1:]...)
	return r.save()
}

// Get returns the drive record for the given label.
func (r *Registry) Get(label string) (DriveRecord, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	i := r.indexOf(label)
	if i < 0 {
		return DriveRecord{}, false
	}
	return r.drives[i], true
}

// List returns a copy of all registered drives, sorted by label.
func (r *Registry) List() []DriveRecord {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]DriveRecord, len(r.drives))
	copy(out, r.drives)
	sort.Slice(out, func(i, j int) bool { return out[i].Label < out[j].Label })
	return out
}

// Update replaces the stored record for d.Label. Returns an error if not found.
func (r *Registry) Update(d DriveRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	i := r.indexOf(d.Label)
	if i < 0 {
		return fmt.Errorf("cold registry: drive %q not found", d.Label)
	}
	r.drives[i] = d
	return r.save()
}

// RecordCopy updates the drive's copy timestamp and total count, and marks it
// as the active drive. All other drives have Active cleared.
func (r *Registry) RecordCopy(label string, at time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	i := r.indexOf(label)
	if i < 0 {
		return fmt.Errorf("cold registry: drive %q not found", label)
	}
	for j := range r.drives {
		r.drives[j].Active = false
	}
	r.drives[i].LastCopyTime = at
	r.drives[i].TotalCopies++
	r.drives[i].Active = true
	return r.save()
}

// RecordVerify updates the drive's verify timestamp.
func (r *Registry) RecordVerify(label string, at time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	i := r.indexOf(label)
	if i < 0 {
		return fmt.Errorf("cold registry: drive %q not found", label)
	}
	r.drives[i].LastVerifyTime = at
	return r.save()
}

// BestCandidate returns the registered drive that should receive the next cold
// copy. Among currently mounted drives it picks the one with the oldest
// LastCopyTime (drives never copied sort before drives that have been copied).
// Returns (zero, false) if no registered drive is currently mounted.
func (r *Registry) BestCandidate() (DriveRecord, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	mounted := make([]DriveRecord, 0, len(r.drives))
	for _, d := range r.drives {
		if d.IsMounted() {
			mounted = append(mounted, d)
		}
	}
	if len(mounted) == 0 {
		return DriveRecord{}, false
	}
	sort.Slice(mounted, func(i, j int) bool {
		ti, tj := mounted[i].LastCopyTime, mounted[j].LastCopyTime
		if ti.IsZero() {
			return true // never copied: always comes first
		}
		if tj.IsZero() {
			return false
		}
		return ti.Before(tj)
	})
	return mounted[0], true
}

// Reload re-reads the registry from disk, picking up any external changes.
func (r *Registry) Reload() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.reload()
}

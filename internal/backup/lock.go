package backup

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/gofrs/flock"
)

// acquireLock acquires an exclusive file-based lock at path using the OS-level
// locking primitives provided by gofrs/flock (LockFileEx on Windows, flock(2)
// on Unix). Returns a release function on success.
//
// Returns [ErrLocked] if another process already holds the lock.
func acquireLock(path string) (release func(), err error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create lock directory: %w", err)
	}

	fl := flock.New(path)
	locked, err := fl.TryLock()
	if err != nil {
		return nil, fmt.Errorf("acquire backup lock: %w", err)
	}
	if !locked {
		return nil, ErrLocked
	}

	return func() {
		_ = fl.Unlock()
	}, nil
}

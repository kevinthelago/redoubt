package drill

import (
	"fmt"
	"os"
	"path/filepath"
)

// CreateScratch creates a uniquely named temp directory under baseDir (mode 0700).
func CreateScratch(baseDir string) (string, error) {
	if err := os.MkdirAll(baseDir, 0700); err != nil {
		return "", fmt.Errorf("create scratch base %q: %w", baseDir, err)
	}
	dir, err := os.MkdirTemp(baseDir, "drill-*")
	if err != nil {
		return "", fmt.Errorf("create scratch dir: %w", err)
	}
	return dir, nil
}

// SecureWipe overwrites every regular file in dir with zeros, then removes the tree.
// Wipe errors are returned but removal proceeds regardless — the caller must treat
// any non-nil return as a warning to investigate, not a blocker.
func SecureWipe(dir string) error {
	var firstWipeErr error
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if e := zeroFile(path); e != nil && firstWipeErr == nil {
			firstWipeErr = fmt.Errorf("zero %q: %w", path, e)
		}
		return nil
	})
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("remove scratch %q: %w", dir, err)
	}
	return firstWipeErr
}

// zeroFile overwrites the contents of path with zeros.
func zeroFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	size := info.Size()
	if size == 0 {
		return nil
	}

	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	defer f.Close()

	buf := make([]byte, min(size, 32*1024))
	for written := int64(0); written < size; {
		n := int64(len(buf))
		if written+n > size {
			n = size - written
		}
		if _, err := f.Write(buf[:n]); err != nil {
			return err
		}
		written += n
	}
	return f.Sync()
}

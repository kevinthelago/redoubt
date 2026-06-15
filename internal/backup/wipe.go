package backup

import (
	"io/fs"
	"os"
	"path/filepath"
)

// wipeTempDir overwrites every regular file in dir with zeros and then removes
// the entire tree. This is a best-effort measure against casual recovery from the
// filesystem; on SSDs with wear-leveling it cannot guarantee physical erasure.
//
// The wipe-then-remove order is intentional: even a partial wipe reduces
// recovery risk for secrets that were briefly written to the OS temp directory.
func wipeTempDir(dir string) error {
	// Zero all regular files before deletion.
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		zeroFile(path)
		return nil
	})

	return os.RemoveAll(dir)
}

// zeroFile overwrites the file's bytes with zeros and syncs to storage.
// Errors are silently swallowed: the subsequent RemoveAll handles true cleanup.
func zeroFile(path string) {
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil || info.Size() == 0 {
		return
	}

	const chunkSize = 4096
	zeros := make([]byte, chunkSize)
	remaining := info.Size()

	for remaining > 0 {
		n := chunkSize
		if remaining < int64(chunkSize) {
			n = int(remaining)
		}
		written, err := f.Write(zeros[:n])
		if err != nil {
			return
		}
		remaining -= int64(written)
	}

	_ = f.Sync()
}

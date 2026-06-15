package restore

import (
	"fmt"
	"os"
	"path/filepath"
)

// preflight checks that the restore can proceed before writing anything.
func preflight(targetDir string, overwrite bool, entries []FileEntry) error {
	if err := checkWritable(targetDir); err != nil {
		return err
	}
	if !overwrite {
		if err := checkEmpty(targetDir); err != nil {
			return err
		}
	}
	needed := totalBytes(entries)
	if err := checkFreeSpace(targetDir, needed); err != nil {
		return err
	}
	return nil
}

// checkWritable verifies the target directory exists and is writable.
func checkWritable(dir string) error {
	info, err := os.Stat(dir)
	if os.IsNotExist(err) {
		// Attempt to create it; missing parent → clear error.
		if err2 := os.MkdirAll(dir, 0755); err2 != nil {
			return fmt.Errorf("target directory %q does not exist and cannot be created: %w", dir, err2)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("target directory %q: %w", dir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("target %q is not a directory", dir)
	}

	// Probe writability by creating and removing a temp file.
	probe := filepath.Join(dir, ".redoubt-write-probe")
	f, err := os.Create(probe)
	if err != nil {
		return fmt.Errorf("target directory %q is not writable: %w", dir, err)
	}
	f.Close()
	os.Remove(probe)
	return nil
}

// checkEmpty returns an error if the directory is non-empty and overwrite is
// not set. Prevents accidental destructive restores.
func checkEmpty(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read target directory: %w", err)
	}
	if len(entries) > 0 {
		return fmt.Errorf(
			"target directory %q is not empty (%d item(s)); use --overwrite to restore into it "+
				"(WARNING: existing files may be overwritten)",
			dir, len(entries),
		)
	}
	return nil
}

// checkFreeSpace verifies the target filesystem has at least needed bytes free.
func checkFreeSpace(dir string, needed int64) error {
	free, err := freeSpace(dir)
	if err != nil {
		// Non-fatal: log and proceed if we can't determine free space.
		return nil
	}
	if needed > 0 && int64(free) < needed {
		return fmt.Errorf(
			"insufficient free space in %q: need %s, have %s",
			dir, humanBytes(needed), humanBytes(int64(free)),
		)
	}
	return nil
}

// totalBytes sums the sizes of all file entries.
func totalBytes(entries []FileEntry) int64 {
	var total int64
	for _, e := range entries {
		if e.Type == "file" {
			total += e.Size
		}
	}
	return total
}

func humanBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}

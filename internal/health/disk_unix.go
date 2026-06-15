//go:build !windows

package health

import (
	"os"
	"syscall"
)

// diskUsedPct returns the percentage of disk space used on the volume that holds
// the Redoubt data directory, falling back to the filesystem root so that disk
// always returns a grade even before the data directory is created.
func diskUsedPct() (float64, error) {
	dir, err := DataDir()
	if err != nil {
		dir = "/"
	}

	var stat syscall.Statfs_t
	if err := syscall.Statfs(dir, &stat); err != nil {
		return 0, err
	}
	total := stat.Blocks * uint64(stat.Bsize)
	free := stat.Bfree * uint64(stat.Bsize)
	if total == 0 {
		return 0, nil
	}
	used := total - free
	return float64(used) / float64(total) * 100.0, nil
}

//go:build !windows

package cli

import (
	"fmt"
	"syscall"
)

func freeSpaceAt(path string) (uint64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, fmt.Errorf("statfs %q: %w", path, err)
	}
	return stat.Bavail * uint64(stat.Bsize), nil
}

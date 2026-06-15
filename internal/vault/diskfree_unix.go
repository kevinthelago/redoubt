//go:build !windows

package vault

import (
	"fmt"
	"syscall"
)

func diskFreeBytes(path string) (free, total int64, err error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, 0, fmt.Errorf("statfs: %w", err)
	}
	blockSize := int64(stat.Bsize)
	return int64(stat.Bavail) * blockSize, int64(stat.Blocks) * blockSize, nil
}

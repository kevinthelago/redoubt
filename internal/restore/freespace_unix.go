//go:build !windows

package restore

import "golang.org/x/sys/unix"

func freeSpace(path string) (uint64, error) {
	var stat unix.Statfs_t
	if err := unix.Statfs(path, &stat); err != nil {
		return 0, err
	}
	//nolint:gosec // Bsize is always positive in a healthy filesystem
	return stat.Bavail * uint64(stat.Bsize), nil
}

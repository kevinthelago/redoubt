//go:build windows

package vault

import (
	"fmt"

	"golang.org/x/sys/windows"
)

func diskFreeBytes(path string) (free, total int64, err error) {
	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, 0, fmt.Errorf("encode path: %w", err)
	}
	var freeAvail, totalBytes, totalFree uint64
	if err := windows.GetDiskFreeSpaceEx(pathPtr, &freeAvail, &totalBytes, &totalFree); err != nil {
		return 0, 0, fmt.Errorf("GetDiskFreeSpaceEx: %w", err)
	}
	return int64(freeAvail), int64(totalBytes), nil
}

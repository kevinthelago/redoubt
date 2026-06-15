//go:build windows

package cli

import (
	"fmt"
	"syscall"
	"unsafe"
)

func freeSpaceAt(path string) (uint64, error) {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	getDiskFreeSpaceEx := kernel32.NewProc("GetDiskFreeSpaceExW")

	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, fmt.Errorf("convert path: %w", err)
	}

	var freeBytes, totalBytes, totalFreeBytes uint64
	r, _, callErr := getDiskFreeSpaceEx.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		uintptr(unsafe.Pointer(&freeBytes)),
		uintptr(unsafe.Pointer(&totalBytes)),
		uintptr(unsafe.Pointer(&totalFreeBytes)),
	)
	if r == 0 {
		return 0, fmt.Errorf("GetDiskFreeSpaceEx: %w", callErr)
	}
	return freeBytes, nil
}

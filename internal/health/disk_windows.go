//go:build windows

package health

import (
	"os"
	"syscall"
	"unsafe"
)

// diskUsedPct returns the percentage of disk space used on the volume that holds
// the Redoubt data directory.
func diskUsedPct() (float64, error) {
	dir, err := DataDir()
	if err != nil {
		dir = os.TempDir()
	}

	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	getDiskFreeSpaceEx := kernel32.NewProc("GetDiskFreeSpaceExW")

	dirW, err := syscall.UTF16PtrFromString(dir)
	if err != nil {
		return 0, err
	}

	var freeBytesAvailable, totalBytes, totalFreeBytes uint64
	r1, _, err := getDiskFreeSpaceEx.Call(
		uintptr(unsafe.Pointer(dirW)),
		uintptr(unsafe.Pointer(&freeBytesAvailable)),
		uintptr(unsafe.Pointer(&totalBytes)),
		uintptr(unsafe.Pointer(&totalFreeBytes)),
	)
	if r1 == 0 {
		return 0, err
	}
	if totalBytes == 0 {
		return 0, nil
	}
	used := totalBytes - totalFreeBytes
	return float64(used) / float64(totalBytes) * 100.0, nil
}

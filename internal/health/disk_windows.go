//go:build windows

package health

import (
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

// diskUsedPct returns the percentage of disk space used on the volume that holds
// the Redoubt data directory. It always resolves to the volume root (e.g. C:\)
// so GetDiskFreeSpaceExW never fails on a not-yet-created data directory.
func diskUsedPct() (float64, error) {
	// Determine the volume root. filepath.VolumeName("C:\foo\bar") → "C:".
	dir, _ := DataDir()
	vol := filepath.VolumeName(dir)
	if vol == "" {
		vol = os.Getenv("SystemDrive")
		if vol == "" {
			vol = "C:"
		}
	}
	root := vol + `\`

	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	getDiskFreeSpaceEx := kernel32.NewProc("GetDiskFreeSpaceExW")

	rootW, err := syscall.UTF16PtrFromString(root)
	if err != nil {
		return 0, err
	}

	var freeBytesAvailable, totalBytes, totalFreeBytes uint64
	r1, _, callErr := getDiskFreeSpaceEx.Call(
		uintptr(unsafe.Pointer(rootW)),
		uintptr(unsafe.Pointer(&freeBytesAvailable)),
		uintptr(unsafe.Pointer(&totalBytes)),
		uintptr(unsafe.Pointer(&totalFreeBytes)),
	)
	if r1 == 0 {
		return 0, callErr
	}
	if totalBytes == 0 {
		return 0, nil
	}
	used := totalBytes - totalFreeBytes
	return float64(used) / float64(totalBytes) * 100.0, nil
}

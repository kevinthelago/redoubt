// Package config holds Redoubt configuration types.
// STUB — foundation stream (F2) owns the full implementation.
package config

import (
	"os"
	"path/filepath"
	"time"
)

// DrillConfig holds drill-specific settings.
type DrillConfig struct {
	ScratchBaseDir    string
	HistoryPath       string
	StatusPath        string // machine-readable status.json for health monitoring
	MaxSampleFiles    int
	MinFreeSpaceBytes uint64
	VaultCadence      time.Duration
	ColdCadence       time.Duration
}

// DefaultDrillConfig returns sensible defaults using the OS config dir.
func DefaultDrillConfig() DrillConfig {
	base := configBase()
	return DrillConfig{
		ScratchBaseDir:    filepath.Join(os.TempDir(), "redoubt-drill"),
		HistoryPath:       filepath.Join(base, "drill-history.json"),
		StatusPath:        filepath.Join(base, "drill-status.json"),
		MaxSampleFiles:    10,
		MinFreeSpaceBytes: 500 * 1024 * 1024, // 500 MiB
		VaultCadence:      7 * 24 * time.Hour,
		ColdCadence:       30 * 24 * time.Hour,
	}
}

func configBase() string {
	if dir, err := os.UserConfigDir(); err == nil {
		return filepath.Join(dir, "redoubt")
	}
	return filepath.Join(os.TempDir(), "redoubt")
}

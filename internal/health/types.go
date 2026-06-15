// Package health aggregates signals from the scheduler, drill, cold-copy, and
// vault subsystems into a protection grade and raises alerts on state change.
package health

import "time"

// Grade is the health level of a signal or the overall system.
type Grade int

const (
	// GradeUnknown means there is no history yet — never display this as safe.
	GradeUnknown Grade = iota
	// GradeHealthy means all thresholds are satisfied.
	GradeHealthy
	// GradeWarning means a threshold is approaching or slightly exceeded.
	GradeWarning
	// GradeCritical means a threshold is exceeded by a significant margin.
	GradeCritical
)

func (g Grade) String() string {
	switch g {
	case GradeHealthy:
		return "healthy"
	case GradeWarning:
		return "warning"
	case GradeCritical:
		return "critical"
	default:
		return "unknown"
	}
}

// MarshalText implements encoding.TextMarshaler for JSON/text serialization.
func (g Grade) MarshalText() ([]byte, error) {
	return []byte(g.String()), nil
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (g *Grade) UnmarshalText(b []byte) error {
	switch string(b) {
	case "healthy":
		*g = GradeHealthy
	case "warning":
		*g = GradeWarning
	case "critical":
		*g = GradeCritical
	default:
		*g = GradeUnknown
	}
	return nil
}

// Signal names match the keys used in Status.Signals and status.json.
const (
	SignalLastBackup = "last_backup"
	SignalSchedule   = "schedule"
	SignalIntegrity  = "integrity"
	SignalDrill      = "drill"
	SignalColdCopy   = "cold_copy"
	SignalVault      = "vault"
	SignalDisk       = "disk"
)

// Thresholds holds the configurable warning/critical bounds for each signal.
// When config.toml is parsed by the config package, these values are populated
// from the [health] section; the defaults below match the plan's specs.
type Thresholds struct {
	// Backup age thresholds.
	BackupWarnAge    time.Duration
	BackupCriticalAge time.Duration

	// Drill age thresholds.
	DrillWarnAge    time.Duration
	DrillCriticalAge time.Duration

	// Cold-copy age thresholds.
	ColdCopyWarnAge    time.Duration
	ColdCopyCriticalAge time.Duration

	// Disk space thresholds (0–100 percent used).
	DiskWarnPct    float64
	DiskCriticalPct float64

	// VaultCheckTTL is how long a vault reachability result stays fresh.
	VaultCheckTTL time.Duration
}

// DefaultThresholds returns sensible defaults matching the plan specifications.
func DefaultThresholds() Thresholds {
	return Thresholds{
		BackupWarnAge:       26 * time.Hour,       // daily + 2 h grace
		BackupCriticalAge:   50 * time.Hour,       // ~2 days missed
		DrillWarnAge:        31 * 24 * time.Hour,  // monthly + 1 day grace
		DrillCriticalAge:    92 * 24 * time.Hour,  // ~3 months
		ColdCopyWarnAge:     8 * 24 * time.Hour,   // weekly + 1 day grace
		ColdCopyCriticalAge: 32 * 24 * time.Hour,  // ~1 month
		DiskWarnPct:         80.0,
		DiskCriticalPct:     90.0,
		VaultCheckTTL:       2 * time.Minute,
	}
}

// SignalStatus is the health grade and human-readable message for one signal.
type SignalStatus struct {
	Grade   Grade  `json:"grade"`
	Message string `json:"message"`
}

// Status is the fully aggregated health snapshot at a point in time.
// This is the type written to status.json and read by the CLI.
type Status struct {
	Overall     Grade                   `json:"overall"`
	Signals     map[string]SignalStatus `json:"signals"`
	EvaluatedAt time.Time               `json:"evaluated_at"`
}

// ExitCode returns the shell exit code that matches the overall grade:
// 0 = healthy, 1 = warning, 2 = critical, 3 = unknown/not-yet-protected.
func (s Status) ExitCode() int {
	switch s.Overall {
	case GradeHealthy:
		return 0
	case GradeWarning:
		return 1
	case GradeCritical:
		return 2
	default:
		return 3
	}
}

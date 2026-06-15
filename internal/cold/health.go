package cold

import (
	"math"
	"time"
)

// StalenessLevel classifies how stale the cold-copy state is.
type StalenessLevel int

const (
	// StalenessUnknown means the drive has never had a successful cold copy.
	StalenessUnknown StalenessLevel = iota
	// StalenessOK means the last copy is within the warning threshold.
	StalenessOK
	// StalenessWarning means the last copy is past the warning threshold but
	// within the error threshold.
	StalenessWarning
	// StalenessError means the last copy exceeds the error threshold.
	StalenessError
)

// String returns a human-readable label for the staleness level.
func (l StalenessLevel) String() string {
	switch l {
	case StalenessUnknown:
		return "unknown"
	case StalenessOK:
		return "ok"
	case StalenessWarning:
		return "warning"
	case StalenessError:
		return "error"
	default:
		return "unknown"
	}
}

// Staleness describes how stale a single cold drive's last-copy state is.
type Staleness struct {
	// Level is the severity.
	Level StalenessLevel

	// DaysSinceLastCopy is the number of whole days since the last
	// successful cold copy. -1 when no copy has ever been made.
	DaysSinceLastCopy int
}

// computeStaleness returns the staleness for a drive given the current config.
func computeStaleness(lastCopy time.Time, cfg ColdConfig) Staleness {
	if lastCopy.IsZero() {
		return Staleness{Level: StalenessUnknown, DaysSinceLastCopy: -1}
	}
	days := int(math.Floor(time.Since(lastCopy).Hours() / 24))
	level := StalenessOK
	switch {
	case days >= cfg.StalenessErrorDays:
		level = StalenessError
	case days >= cfg.StalenessWarningDays:
		level = StalenessWarning
	}
	return Staleness{Level: level, DaysSinceLastCopy: days}
}

// DriveStatus is a snapshot of one drive's state as seen by the health system.
type DriveStatus struct {
	// Drive is the full registry record.
	Drive DriveRecord

	// Mounted reports whether the drive is currently plugged in.
	Mounted bool

	// Staleness is the staleness evaluation for this drive.
	Staleness Staleness
}

// OverallStaleness returns the worst staleness level across all drives.
// It is used by the health aggregator (H1 stream) for its cold-copy signal.
func OverallStaleness(statuses []DriveStatus) StalenessLevel {
	worst := StalenessUnknown
	for _, s := range statuses {
		if s.Staleness.Level > worst {
			worst = s.Staleness.Level
		}
	}
	return worst
}

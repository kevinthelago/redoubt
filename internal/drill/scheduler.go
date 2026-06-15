package drill

import "time"

// DefaultVaultCadence is the default interval between vault drills.
const DefaultVaultCadence = 7 * 24 * time.Hour // weekly

// DefaultColdCadence is the default interval between cold-drive drills.
const DefaultColdCadence = 30 * 24 * time.Hour // ~monthly

// IsDue returns true if the drill is due given the last run and the cadence.
// A zero lastRun means the drill has never run and is always due.
func IsDue(lastRun time.Time, cadence time.Duration) bool {
	if lastRun.IsZero() {
		return true
	}
	return time.Since(lastRun) >= cadence
}

// ParseCadence converts a human string ("daily", "weekly", "monthly") to a Duration.
func ParseCadence(s string) (time.Duration, bool) {
	switch s {
	case "daily":
		return 24 * time.Hour, true
	case "weekly":
		return 7 * 24 * time.Hour, true
	case "monthly":
		return 30 * 24 * time.Hour, true
	}
	return 0, false
}

// FormatCadence returns a human string for a cadence duration.
func FormatCadence(d time.Duration) string {
	switch d {
	case 24 * time.Hour:
		return "daily"
	case 7 * 24 * time.Hour:
		return "weekly"
	case 30 * 24 * time.Hour:
		return "monthly"
	}
	return d.String()
}

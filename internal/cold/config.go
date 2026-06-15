package cold

// RetentionPolicy defines how many snapshots to keep in the cold repository
// after a successful copy. Zero means "no limit" for that dimension.
// This policy is independent of the vault retention managed by the schedule
// stream — cold drives typically keep more history.
type RetentionPolicy struct {
	KeepLast    int `toml:"keep_last"`
	KeepDaily   int `toml:"keep_daily"`
	KeepWeekly  int `toml:"keep_weekly"`
	KeepMonthly int `toml:"keep_monthly"`
	KeepYearly  int `toml:"keep_yearly"`
}

// ColdConfig is the cold-copy section of the application configuration.
// internal/config unmarshals this from the user's TOML config file and
// exposes it through the ConfigProvider interface.
type ColdConfig struct {
	// StalenessWarningDays is the number of days since the last successful
	// cold copy before a warning is raised. Default: 30.
	StalenessWarningDays int `toml:"staleness_warning_days"`

	// StalenessErrorDays is the number of days before staleness becomes an
	// error-level signal. Default: 90.
	StalenessErrorDays int `toml:"staleness_error_days"`

	// ReminderSchedule is an optional cron expression for desktop reminder
	// notifications (e.g. "0 9 1 * *" = 09:00 on the 1st of each month).
	// Empty string disables reminders.
	ReminderSchedule string `toml:"reminder_schedule"`

	// Retention is the independent cold-drive retention policy applied
	// after each successful copy.
	Retention RetentionPolicy `toml:"retention"`
}

// DefaultColdConfig returns sensible defaults suitable for most home users.
// Callers may override individual fields.
func DefaultColdConfig() ColdConfig {
	return ColdConfig{
		StalenessWarningDays: 30,
		StalenessErrorDays:   90,
		Retention: RetentionPolicy{
			KeepLast:    10,
			KeepMonthly: 12,
			KeepYearly:  5,
		},
	}
}

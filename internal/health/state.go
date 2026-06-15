package health

import "time"

// The structs below match the JSON files written by their respective streams.
// These are the seam contracts between the health package and its dependencies.
// File paths are relative to the Redoubt data directory (see DataDir()).

// BackupState is written by the back-up-now stream to backup_status.json.
type BackupState struct {
	LastBackupAt    time.Time `json:"last_backup_at"`
	Result          string    `json:"result"`           // "ok" | "error" | ""
	SnapshotCount   int       `json:"snapshot_count"`
	HasHistory      bool      `json:"has_history"`
}

// ScheduleState is written by the schedule-backups stream to schedule_status.json.
type ScheduleState struct {
	LastRunAt  time.Time `json:"last_run_at"`
	NextRunAt  time.Time `json:"next_run_at"`
	Result     string    `json:"result"`     // "ok" | "error" | ""
	Configured bool      `json:"configured"`
	HasHistory bool      `json:"has_history"`
}

// DrillRecord is one entry in the drill history.
type DrillRecord struct {
	Date   time.Time `json:"date"`
	Target string    `json:"target"`
	Source string    `json:"source"`
	Pass   bool      `json:"pass"`
	Detail string    `json:"detail"`
}

// DrillHistory is written by the run-restore-drill stream to drill_history.json.
type DrillHistory []DrillRecord

// ColdDriveRecord is one entry in the cold-drive registry.
type ColdDriveRecord struct {
	DriveID       string    `json:"drive_id"`
	Label         string    `json:"label"`
	LastCopy      time.Time `json:"last_copy"`
	SnapshotCount int       `json:"snapshot_count"`
}

// ColdRegistry is written by the make-cold-copy stream to cold_registry.json.
type ColdRegistry []ColdDriveRecord

// VaultState is written by the set-up-vault stream (and refreshed during health
// checks) to vault_status.json.
type VaultState struct {
	Reachable    bool      `json:"reachable"`
	LastCheckedAt time.Time `json:"last_checked_at"`
	Configured   bool      `json:"configured"`
}

// IntegrityState is written by integrity.go to integrity_status.json.
type IntegrityState struct {
	LastCheckAt time.Time `json:"last_check_at"`
	Errors      int       `json:"errors"`
	Message     string    `json:"message"`
	HasHistory  bool      `json:"has_history"`
}

// AlertState tracks the last-notified grade per signal for dedup/cooldown.
// Written by notify.go to alert_state.json.
type AlertState struct {
	LastNotifiedAt time.Time        `json:"last_notified_at"`
	LastGrades     map[string]Grade `json:"last_grades"` // signal → grade at last notification
}

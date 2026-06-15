package engine

// Grade represents the health grade of a backup signal or overall status.
type Grade string

const (
	GradeUnknown  Grade = "unknown"
	GradeHealthy  Grade = "healthy"
	GradeWarning  Grade = "warning"
	GradeCritical Grade = "critical"
)

// SignalStatus is the grade + human-readable message for one status signal.
type SignalStatus struct {
	Grade   Grade  `json:"grade"`
	Message string `json:"message"`
}

// StatusResult is the JSON output of `redoubt status --json`.
type StatusResult struct {
	Overall     Grade                   `json:"overall"`
	Signals     map[string]SignalStatus `json:"signals"`
	EvaluatedAt string                  `json:"evaluated_at"`
}

// Asset is a tracked path returned by `redoubt track list --json`.
type Asset struct {
	Path        string `json:"path"`
	Category    string `json:"category"`
	DumpCommand string `json:"dump_command,omitempty"`
	AddedAt     string `json:"added_at"`
}

// SnapshotFilter controls which snapshots are returned.
type SnapshotFilter struct {
	Tags     []string `json:"tags"`
	Hostname string   `json:"hostname"`
	After    string   `json:"after"`
	Before   string   `json:"before"`
	Source   string   `json:"source"` // "vault" or "cold"
}

// Snapshot is one entry from `redoubt snapshots list --json`.
type Snapshot struct {
	ID         string   `json:"id"`
	ShortID    string   `json:"short_id"`
	Time       string   `json:"time"`
	Hostname   string   `json:"hostname"`
	Tags       []string `json:"tags"`
	Paths      []string `json:"paths"`
	BytesAdded int64    `json:"bytes_added"`
}

// TreeNode is one entry in a snapshot file tree.
type TreeNode struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	Type     string `json:"type"` // "file", "dir", "symlink"
	Size     int64  `json:"size"`
	ModTime  string `json:"mod_time"`
	IsSealed bool   `json:"is_sealed"`
}

// DiffEntry is one changed path between two snapshots.
type DiffEntry struct {
	Path       string `json:"path"`
	ChangeType string `json:"change_type"` // "added", "removed", "changed"
	IsSealed   bool   `json:"is_sealed"`
}

// FindResult groups matches from a cross-snapshot search.
type FindResult struct {
	Snapshot Snapshot   `json:"snapshot"`
	Matches  []TreeNode `json:"matches"`
}

// VaultStatus describes the configured remote vault.
type VaultStatus struct {
	Configured bool   `json:"configured"`
	URL        string `json:"url"`
	Reachable  bool   `json:"reachable"`
	Repository string `json:"repository"`
}

// KeyStatus describes the encryption key and escrow state.
type KeyStatus struct {
	Exists         bool   `json:"exists"`
	EscrowVerified bool   `json:"escrow_verified"`
	ShareCount     int    `json:"share_count"`
	LastVerified   string `json:"last_verified,omitempty"`
}

// DriveRecord is one entry from `redoubt cold list --json`.
type DriveRecord struct {
	Label          string `json:"label"`
	MountPath      string `json:"mount_path"`
	RepoSubdir     string `json:"repo_subdir"`
	LastCopyTime   string `json:"last_copy_time"`
	LastVerifyTime string `json:"last_verify_time"`
	TotalCopies    int    `json:"total_copies"`
	Active         bool   `json:"active"`
	IsMounted      bool   `json:"is_mounted"`
}

// DrillResult is one entry from `redoubt drill history --json`.
type DrillResult struct {
	Date   string `json:"date"`
	Asset  string `json:"asset"`
	Pass   bool   `json:"pass"`
	Detail string `json:"detail"`
	Source string `json:"source"` // "vault" or "cold"
}

// AppConfig holds user-configurable settings persisted by the GUI.
type AppConfig struct {
	VaultURL     string          `json:"vault_url"`
	ScheduleCron string          `json:"schedule_cron"`
	Theme        string          `json:"theme"`
	Thresholds   ThresholdConfig `json:"thresholds"`
}

// ThresholdConfig controls the warning/critical thresholds for status signals.
type ThresholdConfig struct {
	MaxHoursWithoutBackup int     `json:"max_hours_without_backup"`
	MaxRepoSizeGB         float64 `json:"max_repo_size_gb"`
}

// BackupProgress is emitted as JSON lines during a backup run.
type BackupProgress struct {
	Asset       string  `json:"asset"`
	Files       int     `json:"files"`
	Bytes       int64   `json:"bytes"`
	DedupeRatio float64 `json:"dedupe_ratio"`
	Done        bool    `json:"done"`
	Error       string  `json:"error,omitempty"`
}

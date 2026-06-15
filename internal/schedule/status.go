// Package schedule manages the OS-level scheduled backup task and records
// run outcomes for health reporting.
package schedule

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/kevinthelago/redoubt/internal/config"
)

// Status records the outcome of the most recently completed scheduled run.
type Status struct {
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at"`
	ExitCode   int       `json:"exit_code"`
	Error      string    `json:"error,omitempty"`
}

// IsZero reports whether no scheduled run has been recorded yet.
func (s Status) IsZero() bool { return s.StartedAt.IsZero() }

func statusPath() string {
	return filepath.Join(config.DataDir(), "schedule-status.json")
}

// ReadStatus reads the last-run status from disk.
// Returns a zero Status (IsZero() == true) when no record exists yet.
func ReadStatus() (Status, error) {
	data, err := os.ReadFile(statusPath())
	if errors.Is(err, os.ErrNotExist) {
		return Status{}, nil
	}
	if err != nil {
		return Status{}, err
	}
	var s Status
	return s, json.Unmarshal(data, &s)
}

// WriteStatus persists a run outcome to disk atomically.
func WriteStatus(s Status) error {
	path := statusPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

package health

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// DataDir returns the Redoubt data directory. When the internal/config package
// lands (F2), the CLI should pass the config-derived path instead of relying on
// this helper. This is a safe fallback for the health package's own use.
func DataDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "redoubt"), nil
}

// readJSON reads a JSON file into dst. Returns (false, nil) when the file does
// not exist — that is the "not yet configured / no history" case.
func readJSON(path string, dst any) (found bool, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return true, json.Unmarshal(data, dst)
}

// writeJSON writes dst to path, creating parent dirs as needed.
func writeJSON(path string, src any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(src, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}

// StateReader reads all the per-stream state files from a single data directory.
type StateReader struct {
	dir string
}

// NewStateReader creates a StateReader rooted at dir.
func NewStateReader(dir string) *StateReader {
	return &StateReader{dir: dir}
}

func (r *StateReader) path(name string) string {
	return filepath.Join(r.dir, name)
}

// ReadBackup returns the last backup state, or a zero value with HasHistory=false
// when the file does not exist.
func (r *StateReader) ReadBackup() (BackupState, error) {
	var s BackupState
	found, err := readJSON(r.path("backup_status.json"), &s)
	if err != nil {
		return BackupState{}, err
	}
	s.HasHistory = found
	return s, nil
}

// ReadSchedule returns the schedule state.
func (r *StateReader) ReadSchedule() (ScheduleState, error) {
	var s ScheduleState
	found, err := readJSON(r.path("schedule_status.json"), &s)
	if err != nil {
		return ScheduleState{}, err
	}
	s.HasHistory = found
	return s, nil
}

// ReadDrillHistory returns the full drill history.
func (r *StateReader) ReadDrillHistory() (DrillHistory, error) {
	var h DrillHistory
	found, err := readJSON(r.path("drill_history.json"), &h)
	if !found || err != nil {
		return nil, err
	}
	return h, nil
}

// ReadColdRegistry returns the cold-drive registry.
func (r *StateReader) ReadColdRegistry() (ColdRegistry, error) {
	var reg ColdRegistry
	found, err := readJSON(r.path("cold_registry.json"), &reg)
	if !found || err != nil {
		return nil, err
	}
	return reg, nil
}

// ReadVault returns the vault reachability state.
func (r *StateReader) ReadVault() (VaultState, error) {
	var s VaultState
	_, err := readJSON(r.path("vault_status.json"), &s)
	return s, err
}

// ReadIntegrity returns the last restic-check result.
func (r *StateReader) ReadIntegrity() (IntegrityState, error) {
	var s IntegrityState
	found, err := readJSON(r.path("integrity_status.json"), &s)
	if err != nil {
		return IntegrityState{}, err
	}
	s.HasHistory = found
	return s, nil
}

// ReadStatus returns the last written health status.
func (r *StateReader) ReadStatus() (Status, error) {
	var s Status
	_, err := readJSON(r.path("status.json"), &s)
	return s, err
}

// ReadAlertState returns the alert dedupe/cooldown state.
func (r *StateReader) ReadAlertState() (AlertState, error) {
	var s AlertState
	_, err := readJSON(r.path("alert_state.json"), &s)
	return s, err
}

// WriteStatus persists a Status to status.json.
func (r *StateReader) WriteStatus(s Status) error {
	return writeJSON(r.path("status.json"), s)
}

// WriteIntegrity persists an IntegrityState to integrity_status.json.
func (r *StateReader) WriteIntegrity(s IntegrityState) error {
	return writeJSON(r.path("integrity_status.json"), s)
}

// WriteAlertState persists the alert state to alert_state.json.
func (r *StateReader) WriteAlertState(s AlertState) error {
	return writeJSON(r.path("alert_state.json"), s)
}

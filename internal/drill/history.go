package drill

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

// Status is the outcome of a drill run.
type Status string

const (
	StatusPass Status = "pass"
	// StatusSkip means the drill was intentionally not run (no snapshots or low
	// space). It is NOT an error and must not trigger alerts.
	StatusSkip Status = "skip"
	StatusFail Status = "fail"
)

// Source identifies which snapshot repository was drilled.
type Source string

const (
	SourceVault Source = "vault"
	SourceCold  Source = "cold"
)

// Result is the record of one drill run.
type Result struct {
	Timestamp  time.Time `json:"timestamp"`
	Source     Source    `json:"source"`
	SnapshotID string    `json:"snapshot_id,omitempty"`
	Restored   int       `json:"restored"`
	Verified   int       `json:"verified"`
	Failed     int       `json:"failed"`
	Status     Status    `json:"status"`
	SkipReason string    `json:"skip_reason,omitempty"`
	FailureMsg string    `json:"failure_msg,omitempty"`
	// DurationMS is the wall-clock time of the drill in milliseconds.
	DurationMS int64 `json:"duration_ms"`
}

// History is the ordered list of drill results, newest first.
type History struct {
	Results []Result `json:"results"`
}

// LoadHistory reads drill history from path.
// Returns an empty History if the file does not exist.
func LoadHistory(path string) (*History, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &History{}, nil
	}
	if err != nil {
		return nil, err
	}
	var h History
	if err := json.Unmarshal(data, &h); err != nil {
		return nil, err
	}
	return &h, nil
}

// Save writes the history to path (0600, creates or truncates).
func (h *History) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(h, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

// Append prepends result (newest first) and caps the list at maxLen.
func (h *History) Append(result Result, maxLen int) {
	h.Results = append([]Result{result}, h.Results...)
	if maxLen > 0 && len(h.Results) > maxLen {
		h.Results = h.Results[:maxLen]
	}
}

// Last returns the most recent result for src, and false if none exists.
func (h *History) Last(src Source) (Result, bool) {
	for _, r := range h.Results {
		if r.Source == src {
			return r, true
		}
	}
	return Result{}, false
}

// Summary is the machine-readable health signal written to StatusPath after
// each drill. The health stream (H1) reads this file.
type Summary struct {
	LastVault *Result `json:"last_vault,omitempty"`
	LastCold  *Result `json:"last_cold,omitempty"`
}

// WriteSummary derives a Summary from h and writes it as JSON to path.
func (h *History) WriteSummary(path string) error {
	s := Summary{}
	if r, ok := h.Last(SourceVault); ok {
		s.LastVault = &r
	}
	if r, ok := h.Last(SourceCold); ok {
		s.LastCold = &r
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

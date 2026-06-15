package schedule

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/kevinthelago/redoubt/internal/config"
)

// redirectDataDir points config.DataDir() at a temp directory for the duration of t.
func redirectDataDir(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	// Set the env var that DataDir() reads on each platform.
	switch runtime.GOOS {
	case "windows":
		t.Setenv("APPDATA", tmp)
	case "darwin":
		// DataDir() on darwin uses UserHomeDir; redirect via HOME.
		t.Setenv("HOME", tmp)
	default:
		t.Setenv("XDG_DATA_HOME", tmp)
	}
	return config.DataDir()
}

func TestReadStatusNoFile(t *testing.T) {
	redirectDataDir(t)

	s, err := ReadStatus()
	if err != nil {
		t.Fatalf("ReadStatus with no file: %v", err)
	}
	if !s.IsZero() {
		t.Fatal("expected zero status when no file exists")
	}
}

func TestWriteReadStatusRoundTrip(t *testing.T) {
	redirectDataDir(t)

	want := Status{
		StartedAt:  time.Date(2025, 1, 15, 2, 0, 0, 0, time.UTC),
		FinishedAt: time.Date(2025, 1, 15, 2, 3, 42, 0, time.UTC),
		ExitCode:   0,
	}

	if err := WriteStatus(want); err != nil {
		t.Fatalf("WriteStatus: %v", err)
	}

	got, err := ReadStatus()
	if err != nil {
		t.Fatalf("ReadStatus: %v", err)
	}
	if got.IsZero() {
		t.Fatal("ReadStatus returned zero status after write")
	}
	if !got.StartedAt.Equal(want.StartedAt) {
		t.Errorf("StartedAt: got %v, want %v", got.StartedAt, want.StartedAt)
	}
	if got.ExitCode != want.ExitCode {
		t.Errorf("ExitCode: got %d, want %d", got.ExitCode, want.ExitCode)
	}
}

func TestWriteStatusFailedRun(t *testing.T) {
	redirectDataDir(t)

	want := Status{
		StartedAt:  time.Now().UTC(),
		FinishedAt: time.Now().UTC().Add(5 * time.Second),
		ExitCode:   1,
		Error:      "exit status 1",
	}

	if err := WriteStatus(want); err != nil {
		t.Fatalf("WriteStatus: %v", err)
	}

	got, err := ReadStatus()
	if err != nil {
		t.Fatalf("ReadStatus: %v", err)
	}
	if got.ExitCode != 1 {
		t.Errorf("ExitCode: got %d, want 1", got.ExitCode)
	}
	if got.Error != want.Error {
		t.Errorf("Error: got %q, want %q", got.Error, want.Error)
	}
}

func TestWriteStatusCreatesDirectory(t *testing.T) {
	redirectDataDir(t)

	path := statusPath()
	dir := filepath.Dir(path)
	// Remove the dir so WriteStatus must create it.
	_ = os.RemoveAll(dir)

	s := Status{StartedAt: time.Now().UTC(), FinishedAt: time.Now().UTC()}
	if err := WriteStatus(s); err != nil {
		t.Fatalf("WriteStatus: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("status file not created: %v", err)
	}
}

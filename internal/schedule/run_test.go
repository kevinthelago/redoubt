package schedule

import (
	"context"
	"os"
	"runtime"
	"strconv"
	"testing"
)

// TestMain intercepts subprocess invocations during run tests: when
// REDOUBT_TEST_SUBPROCESS=1 this process acts as the fake backup command,
// exiting with the code in REDOUBT_TEST_EXIT_CODE instead of running tests.
func TestMain(m *testing.M) {
	if os.Getenv("REDOUBT_TEST_SUBPROCESS") == "1" {
		code, _ := strconv.Atoi(os.Getenv("REDOUBT_TEST_EXIT_CODE"))
		os.Exit(code)
	}
	os.Exit(m.Run())
}

func TestRunWritesStatusOnSuccess(t *testing.T) {
	redirectDataDir(t)
	// Tell the subprocess (this test binary re-invoked via os.Executable) to
	// exit 0 immediately rather than running any tests.
	t.Setenv("REDOUBT_TEST_SUBPROCESS", "1")
	t.Setenv("REDOUBT_TEST_EXIT_CODE", "0")
	t.Setenv("REDOUBT_IS_VAULT", "")

	if err := Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	s, err := ReadStatus()
	if err != nil {
		t.Fatalf("ReadStatus: %v", err)
	}
	if s.IsZero() {
		t.Fatal("status not written after successful Run")
	}
	if s.ExitCode != 0 {
		t.Errorf("ExitCode: got %d, want 0", s.ExitCode)
	}
	if s.FinishedAt.Before(s.StartedAt) {
		t.Error("FinishedAt is before StartedAt")
	}
}

func TestRunWritesStatusOnFailure(t *testing.T) {
	redirectDataDir(t)
	t.Setenv("REDOUBT_TEST_SUBPROCESS", "1")
	t.Setenv("REDOUBT_TEST_EXIT_CODE", "1")
	t.Setenv("REDOUBT_IS_VAULT", "")

	err := Run(context.Background())
	if err == nil {
		t.Fatal("Run: expected non-nil error when backup exits 1")
	}

	s, readErr := ReadStatus()
	if readErr != nil {
		t.Fatalf("ReadStatus: %v", readErr)
	}
	if s.IsZero() {
		t.Fatal("status not written after failed Run")
	}
	if s.ExitCode != 1 {
		t.Errorf("ExitCode: got %d, want 1", s.ExitCode)
	}
	if s.Error == "" {
		t.Error("Error field should be non-empty after a failed run")
	}
}

func TestRunSkipsRetentionOnDevBox(t *testing.T) {
	redirectDataDir(t)
	t.Setenv("REDOUBT_TEST_SUBPROCESS", "1")
	t.Setenv("REDOUBT_TEST_EXIT_CODE", "0")
	t.Setenv("REDOUBT_IS_VAULT", "") // not the vault host

	// applyRetention must not invoke restic when REDOUBT_IS_VAULT is unset.
	// We confirm this by verifying Run succeeds even though there is no
	// configured vault URL or password — if retention ran it would fail.
	if err := Run(context.Background()); err != nil {
		t.Fatalf("Run on dev box: %v", err)
	}
}

func TestTimeToCron(t *testing.T) {
	cases := []struct{ in, want string }{
		{"02:00", "0 2 * * *"},
		{"14:30", "30 14 * * *"},
		{"00:00", "0 0 * * *"},
		{"23:59", "59 23 * * *"},
	}
	for _, c := range cases {
		got := timeToCron(c.in)
		if got != c.want {
			t.Errorf("timeToCron(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestValidateTime(t *testing.T) {
	valid := []string{"02:00", "00:00", "23:59", "9:05"}
	for _, s := range valid {
		if err := validateTime(s); err != nil {
			t.Errorf("validateTime(%q): unexpected error: %v", s, err)
		}
	}

	invalid := []string{"24:00", "02:60", "abc", "2pm", "2:00:00", ""}
	for _, s := range invalid {
		if err := validateTime(s); err == nil {
			t.Errorf("validateTime(%q): expected error, got nil", s)
		}
	}
}

// Ensure os.Executable() resolves correctly (sanity check used by runBackup).
func TestExecutableResolves(t *testing.T) {
	if runtime.GOOS == "js" {
		t.Skip("os.Executable not supported on js/wasm")
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	if exe == "" {
		t.Fatal("os.Executable returned empty string")
	}
}

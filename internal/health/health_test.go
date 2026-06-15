package health

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

var (
	now  = time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	defs = DefaultThresholds()
)

// writeState writes a value as JSON to dir/name.
func writeState(t *testing.T, dir, name string, v any) {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

// newReader creates a StateReader backed by a temp dir pre-populated with the
// given states. Unset fields default to zero/missing (no history).
type stateFixture struct {
	backup    *BackupState
	schedule  *ScheduleState
	drills    DrillHistory
	coldReg   ColdRegistry
	vault     *VaultState
	integrity *IntegrityState
}

func makeReader(t *testing.T, f stateFixture) *StateReader {
	t.Helper()
	dir := t.TempDir()
	if f.backup != nil {
		writeState(t, dir, "backup_status.json", f.backup)
	}
	if f.schedule != nil {
		writeState(t, dir, "schedule_status.json", f.schedule)
	}
	if f.drills != nil {
		writeState(t, dir, "drill_history.json", f.drills)
	}
	if f.coldReg != nil {
		writeState(t, dir, "cold_registry.json", f.coldReg)
	}
	if f.vault != nil {
		writeState(t, dir, "vault_status.json", f.vault)
	}
	if f.integrity != nil {
		writeState(t, dir, "integrity_status.json", f.integrity)
	}
	return NewStateReader(dir)
}

// TestNoHistory verifies that a fresh installation with no state files grades
// every signal Unknown and the overall grade is Unknown (never a false green).
func TestNoHistory(t *testing.T) {
	reader := makeReader(t, stateFixture{})
	s, err := Evaluate(reader, defs, now)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	// Disk is always graded from actual system data — skip it here.
	for name, sig := range s.Signals {
		if name == SignalDisk {
			continue
		}
		if sig.Grade != GradeUnknown {
			t.Errorf("signal %s = %v, want Unknown", name, sig.Grade)
		}
	}
	// Core invariant: overall must NEVER be Healthy when there is no history.
	// (It will be Unknown on a fresh system, or Warning/Critical if disk is elevated.)
	if s.Overall == GradeHealthy {
		t.Errorf("overall = Healthy with no backup history — false green!")
	}
}

// TestAllHealthy verifies that all signals within thresholds → overall Healthy.
func TestAllHealthy(t *testing.T) {
	reader := makeReader(t, stateFixture{
		backup: &BackupState{
			LastBackupAt: now.Add(-12 * time.Hour),
			Result:       "ok",
			HasHistory:   true,
		},
		schedule: &ScheduleState{
			LastRunAt:  now.Add(-12 * time.Hour),
			NextRunAt:  now.Add(12 * time.Hour),
			Result:     "ok",
			Configured: true,
			HasHistory: true,
		},
		drills: DrillHistory{{
			Date:   now.Add(-7 * 24 * time.Hour),
			Target: "src",
			Pass:   true,
		}},
		coldReg: ColdRegistry{{
			DriveID:  "drive-1",
			Label:    "WD-BACKUP-01",
			LastCopy: now.Add(-3 * 24 * time.Hour),
		}},
		vault: &VaultState{
			Reachable:     true,
			LastCheckedAt: now.Add(-1 * time.Minute),
			Configured:    true,
		},
		integrity: &IntegrityState{
			LastCheckAt: now.Add(-6 * time.Hour),
			Errors:      0,
			HasHistory:  true,
		},
	})

	s, err := Evaluate(reader, defs, now)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	for name, sig := range s.Signals {
		if name == SignalDisk {
			continue // disk grade depends on the test host
		}
		if sig.Grade != GradeHealthy {
			t.Errorf("signal %s = %v (%s), want Healthy", name, sig.Grade, sig.Message)
		}
	}

	// Overall is Healthy unless disk alone is elevated on the test host.
	diskGrade := s.Signals[SignalDisk].Grade
	wantOverall := GradeHealthy
	if diskGrade > GradeHealthy {
		wantOverall = diskGrade
	}
	if s.Overall != wantOverall {
		t.Errorf("overall = %v, want %v (disk signal = %v)", s.Overall, wantOverall, diskGrade)
	}
}

// TestBackupWarning verifies that a backup just past the warn threshold → Warning.
func TestBackupWarning(t *testing.T) {
	reader := makeReader(t, stateFixture{
		backup: &BackupState{
			LastBackupAt: now.Add(-(defs.BackupWarnAge + time.Minute)),
			Result:       "ok",
			HasHistory:   true,
		},
	})
	s, err := Evaluate(reader, defs, now)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if s.Signals[SignalLastBackup].Grade != GradeWarning {
		t.Errorf("backup signal = %v, want Warning", s.Signals[SignalLastBackup].Grade)
	}
}

// TestBackupCritical verifies that a backup past the critical threshold → Critical.
func TestBackupCritical(t *testing.T) {
	reader := makeReader(t, stateFixture{
		backup: &BackupState{
			LastBackupAt: now.Add(-(defs.BackupCriticalAge + time.Minute)),
			Result:       "ok",
			HasHistory:   true,
		},
	})
	s, err := Evaluate(reader, defs, now)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if s.Signals[SignalLastBackup].Grade != GradeCritical {
		t.Errorf("backup signal = %v, want Critical", s.Signals[SignalLastBackup].Grade)
	}
}

// TestBackupFailResult verifies that a failed backup result → Critical.
func TestBackupFailResult(t *testing.T) {
	reader := makeReader(t, stateFixture{
		backup: &BackupState{
			LastBackupAt: now.Add(-1 * time.Hour),
			Result:       "error",
			HasHistory:   true,
		},
	})
	s, err := Evaluate(reader, defs, now)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if s.Signals[SignalLastBackup].Grade != GradeCritical {
		t.Errorf("backup signal = %v, want Critical for error result", s.Signals[SignalLastBackup].Grade)
	}
}

// TestOneCriticalRaisesOverall verifies that one Critical signal → overall Critical
// even when all other signals are Healthy.
func TestOneCriticalRaisesOverall(t *testing.T) {
	reader := makeReader(t, stateFixture{
		backup: &BackupState{
			LastBackupAt: now.Add(-(defs.BackupCriticalAge + time.Hour)),
			Result:       "ok",
			HasHistory:   true,
		},
		schedule: &ScheduleState{
			Configured: true,
			HasHistory: true,
			Result:     "ok",
			LastRunAt:  now.Add(-1 * time.Hour),
			NextRunAt:  now.Add(23 * time.Hour),
		},
		drills: DrillHistory{{Date: now.Add(-5 * 24 * time.Hour), Pass: true}},
		coldReg: ColdRegistry{{LastCopy: now.Add(-2 * 24 * time.Hour)}},
		vault: &VaultState{
			Reachable:     true,
			LastCheckedAt: now.Add(-30 * time.Second),
			Configured:    true,
		},
		integrity: &IntegrityState{
			LastCheckAt: now.Add(-2 * time.Hour),
			HasHistory:  true,
		},
	})
	s, err := Evaluate(reader, defs, now)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if s.Overall != GradeCritical {
		t.Errorf("overall = %v, want Critical", s.Overall)
	}
}

// TestDrillFailed verifies that a failed drill → Critical.
func TestDrillFailed(t *testing.T) {
	reader := makeReader(t, stateFixture{
		drills: DrillHistory{{
			Date:   now.Add(-2 * 24 * time.Hour),
			Pass:   false,
			Detail: "canary hash mismatch",
		}},
	})
	s, err := Evaluate(reader, defs, now)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if s.Signals[SignalDrill].Grade != GradeCritical {
		t.Errorf("drill signal = %v, want Critical", s.Signals[SignalDrill].Grade)
	}
}

// TestIntegrityErrors verifies that a restic check with errors → Critical.
func TestIntegrityErrors(t *testing.T) {
	reader := makeReader(t, stateFixture{
		integrity: &IntegrityState{
			LastCheckAt: now.Add(-1 * time.Hour),
			Errors:      2,
			Message:     "pack file corrupted",
			HasHistory:  true,
		},
	})
	s, err := Evaluate(reader, defs, now)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if s.Signals[SignalIntegrity].Grade != GradeCritical {
		t.Errorf("integrity signal = %v, want Critical", s.Signals[SignalIntegrity].Grade)
	}
}

// TestVaultUnreachable verifies that a reachable=false vault state → Critical.
func TestVaultUnreachable(t *testing.T) {
	reader := makeReader(t, stateFixture{
		vault: &VaultState{
			Reachable:     false,
			LastCheckedAt: now.Add(-30 * time.Second),
			Configured:    true,
		},
	})
	s, err := Evaluate(reader, defs, now)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if s.Signals[SignalVault].Grade != GradeCritical {
		t.Errorf("vault signal = %v, want Critical", s.Signals[SignalVault].Grade)
	}
}

// TestVaultNotConfigured verifies that an unconfigured vault → Unknown (not green).
func TestVaultNotConfigured(t *testing.T) {
	reader := makeReader(t, stateFixture{
		vault: &VaultState{Configured: false},
	})
	s, err := Evaluate(reader, defs, now)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if s.Signals[SignalVault].Grade != GradeUnknown {
		t.Errorf("vault signal = %v, want Unknown for unconfigured", s.Signals[SignalVault].Grade)
	}
}

// TestColdCopyWarning verifies that a cold copy past the warn threshold → Warning.
func TestColdCopyWarning(t *testing.T) {
	reader := makeReader(t, stateFixture{
		coldReg: ColdRegistry{{
			DriveID:  "drive-1",
			LastCopy: now.Add(-(defs.ColdCopyWarnAge + 24*time.Hour)),
		}},
	})
	s, err := Evaluate(reader, defs, now)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if s.Signals[SignalColdCopy].Grade != GradeWarning {
		t.Errorf("cold copy signal = %v, want Warning", s.Signals[SignalColdCopy].Grade)
	}
}

// TestDiskGrading verifies gradeDisk at various usage levels.
func TestDiskGrading(t *testing.T) {
	tests := []struct {
		pct  float64
		want Grade
	}{
		{50.0, GradeHealthy},
		{80.0, GradeWarning},
		{85.0, GradeWarning},
		{90.0, GradeCritical},
		{95.0, GradeCritical},
	}
	for _, tc := range tests {
		got := gradeDisk(tc.pct, nil, defs)
		if got.Grade != tc.want {
			t.Errorf("gradeDisk(%.0f%%) = %v, want %v", tc.pct, got.Grade, tc.want)
		}
	}
}

// TestExitCodes verifies the mapping from Grade to exit code.
func TestExitCodes(t *testing.T) {
	tests := []struct {
		grade Grade
		want  int
	}{
		{GradeHealthy, 0},
		{GradeWarning, 1},
		{GradeCritical, 2},
		{GradeUnknown, 3},
	}
	for _, tc := range tests {
		s := Status{Overall: tc.grade}
		if got := s.ExitCode(); got != tc.want {
			t.Errorf("ExitCode(%v) = %d, want %d", tc.grade, got, tc.want)
		}
	}
}

// TestStatusRoundTrip verifies that Status serializes and deserializes without loss.
func TestStatusRoundTrip(t *testing.T) {
	original := Status{
		Overall: GradeWarning,
		Signals: map[string]SignalStatus{
			SignalLastBackup: {Grade: GradeWarning, Message: "last backup 30h ago"},
			SignalSchedule:   {Grade: GradeHealthy, Message: "next run Mon 10:00"},
			SignalIntegrity:  {Grade: GradeHealthy, Message: "checked 4h ago"},
			SignalDrill:      {Grade: GradeUnknown, Message: "no drill history"},
			SignalColdCopy:   {Grade: GradeHealthy, Message: "last cold copy 3 days ago"},
			SignalVault:      {Grade: GradeHealthy, Message: "reachable"},
			SignalDisk:       {Grade: GradeHealthy, Message: "62% used"},
		},
		EvaluatedAt: now,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var restored Status
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if restored.Overall != original.Overall {
		t.Errorf("overall: got %v, want %v", restored.Overall, original.Overall)
	}
	for name, orig := range original.Signals {
		rest, ok := restored.Signals[name]
		if !ok {
			t.Errorf("signal %s missing after round-trip", name)
			continue
		}
		if rest.Grade != orig.Grade || rest.Message != orig.Message {
			t.Errorf("signal %s: got {%v, %q}, want {%v, %q}", name, rest.Grade, rest.Message, orig.Grade, orig.Message)
		}
	}
}

package cold_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kevinthelago/redoubt/internal/cold"
)

// -- mock backend -----------------------------------------------------------

type mockBackend struct {
	mu sync.Mutex

	// configured failures
	initErr   error
	readyErr  error
	copyErr   error
	checkErr  error
	forgetErr error

	// configured state
	isReady bool // return value of IsLocalRepoReady

	// call tracking
	initCalled   []string
	readyCalled  []string
	copyCalled   []string
	checkCalled  []string
	forgetCalled []string
}

func (m *mockBackend) InitLocalRepo(_ context.Context, path string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.initCalled = append(m.initCalled, path)
	return m.initErr
}

func (m *mockBackend) IsLocalRepoReady(_ context.Context, path string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.readyCalled = append(m.readyCalled, path)
	return m.isReady, m.readyErr
}

func (m *mockBackend) CopySnapshotsTo(_ context.Context, path string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.copyCalled = append(m.copyCalled, path)
	return m.copyErr
}

func (m *mockBackend) CheckLocalRepo(_ context.Context, path string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.checkCalled = append(m.checkCalled, path)
	return m.checkErr
}

func (m *mockBackend) ForgetInLocalRepo(_ context.Context, path string, _ cold.RetentionPolicy) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.forgetCalled = append(m.forgetCalled, path)
	return m.forgetErr
}

// -- mock config provider ---------------------------------------------------

type mockConfig struct {
	dir string
	cfg cold.ColdConfig
}

func newMockConfig(dir string) *mockConfig {
	return &mockConfig{
		dir: dir,
		cfg: cold.DefaultColdConfig(),
	}
}

func (c *mockConfig) ColdConfig() cold.ColdConfig { return c.cfg }
func (c *mockConfig) DriveRegistryPath() string   { return filepath.Join(c.dir, "cold_drives.toml") }

// -- helpers ----------------------------------------------------------------

// mountedDir creates a temporary directory that simulates a mounted drive.
func mountedDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	return dir
}

func collectProgress(t *testing.T) (func(string), *[]string) {
	t.Helper()
	var msgs []string
	var mu sync.Mutex
	fn := func(s string) {
		mu.Lock()
		msgs = append(msgs, s)
		mu.Unlock()
	}
	return fn, &msgs
}

// -- tests ------------------------------------------------------------------

func TestEngine_Run_HappyPath_FirstTime(t *testing.T) {
	dir := t.TempDir()
	mnt := mountedDir(t)

	cfg := newMockConfig(dir)
	b := &mockBackend{isReady: false} // repo not yet initialised
	eng, err := cold.NewEngine(b, cfg)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	// Register the drive (mount path is the temp dir, which exists).
	if err := eng.Register(cold.DriveRecord{Label: "drive-a", MountPath: mnt}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	progress, msgs := collectProgress(t)
	result, err := eng.Run(context.Background(), cold.RunOptions{Progress: progress})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if !result.Initialised {
		t.Error("expected Initialised=true on first use")
	}
	if result.Drive.Label != "drive-a" {
		t.Errorf("Drive.Label: got %q, want drive-a", result.Drive.Label)
	}
	if len(b.initCalled) != 1 {
		t.Errorf("InitLocalRepo call count: got %d, want 1", len(b.initCalled))
	}
	if len(b.copyCalled) != 1 {
		t.Errorf("CopySnapshotsTo call count: got %d, want 1", len(b.copyCalled))
	}
	if len(b.checkCalled) != 1 {
		t.Errorf("CheckLocalRepo call count: got %d, want 1", len(b.checkCalled))
	}

	// Verify progress messages contain expected milestones.
	allMsgs := strings.Join(*msgs, "\n")
	for _, want := range []string{"Initialising", "Copying", "Verifying", "complete"} {
		if !strings.Contains(allMsgs, want) {
			t.Errorf("progress messages: missing %q in:\n%s", want, allMsgs)
		}
	}
}

func TestEngine_Run_HappyPath_Subsequent(t *testing.T) {
	dir := t.TempDir()
	mnt := mountedDir(t)

	cfg := newMockConfig(dir)
	b := &mockBackend{isReady: true} // repo already initialised
	eng, _ := cold.NewEngine(b, cfg)
	_ = eng.Register(cold.DriveRecord{Label: "drive-a", MountPath: mnt})

	_, err := eng.Run(context.Background(), cold.RunOptions{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(b.initCalled) != 0 {
		t.Errorf("InitLocalRepo should not be called on subsequent run (called %d times)", len(b.initCalled))
	}
	if len(b.copyCalled) != 1 {
		t.Errorf("CopySnapshotsTo call count: got %d, want 1", len(b.copyCalled))
	}
}

func TestEngine_Run_DriveAbsent_NamedDrive(t *testing.T) {
	dir := t.TempDir()
	cfg := newMockConfig(dir)
	b := &mockBackend{isReady: true}
	eng, _ := cold.NewEngine(b, cfg)

	// Register with a path that does not exist.
	_ = eng.Register(cold.DriveRecord{Label: "drive-a", MountPath: "/nonexistent/path/9x3q"})

	_, err := eng.Run(context.Background(), cold.RunOptions{DriveLabel: "drive-a"})
	if err == nil {
		t.Fatal("expected error for absent drive, got nil")
	}
	var notMounted *cold.ErrDriveNotMounted
	if !errors.As(err, &notMounted) {
		t.Errorf("error type: got %T (%v), want *cold.ErrDriveNotMounted", err, err)
	}

	// No repository operations should have occurred.
	if len(b.initCalled) > 0 || len(b.copyCalled) > 0 || len(b.checkCalled) > 0 {
		t.Error("backend was called despite drive not being mounted (partial state)")
	}
}

func TestEngine_Run_DriveAbsent_AutoDetect(t *testing.T) {
	dir := t.TempDir()
	cfg := newMockConfig(dir)
	b := &mockBackend{isReady: true}
	eng, _ := cold.NewEngine(b, cfg)
	_ = eng.Register(cold.DriveRecord{Label: "drive-a", MountPath: "/nonexistent/path/7z2w"})

	_, err := eng.Run(context.Background(), cold.RunOptions{})
	var noDrive *cold.ErrNoDriveMounted
	if !errors.As(err, &noDrive) {
		t.Errorf("error type: got %T (%v), want *cold.ErrNoDriveMounted", err, err)
	}
}

func TestEngine_Run_NoDriveRegistered(t *testing.T) {
	dir := t.TempDir()
	cfg := newMockConfig(dir)
	b := &mockBackend{}
	eng, _ := cold.NewEngine(b, cfg)

	_, err := eng.Run(context.Background(), cold.RunOptions{})
	var noReg *cold.ErrNoDriveRegistered
	if !errors.As(err, &noReg) {
		t.Errorf("error type: got %T (%v), want *cold.ErrNoDriveRegistered", err, err)
	}
}

func TestEngine_Run_CopyError_NoRegistryUpdate(t *testing.T) {
	dir := t.TempDir()
	mnt := mountedDir(t)
	cfg := newMockConfig(dir)
	copyErr := errors.New("simulated copy failure")
	b := &mockBackend{isReady: true, copyErr: copyErr}
	eng, _ := cold.NewEngine(b, cfg)
	_ = eng.Register(cold.DriveRecord{Label: "drive-a", MountPath: mnt})

	_, err := eng.Run(context.Background(), cold.RunOptions{})
	if err == nil {
		t.Fatal("expected copy error, got nil")
	}
	if !strings.Contains(err.Error(), "copy snapshots") {
		t.Errorf("error: got %q, want it to mention copy failure", err.Error())
	}

	// Registry must not record a copy time on failure.
	statuses := eng.Status()
	if len(statuses) == 0 {
		t.Fatal("no statuses returned")
	}
	if !statuses[0].Drive.LastCopyTime.IsZero() {
		t.Error("LastCopyTime should remain zero after copy failure")
	}
}

func TestEngine_Run_CheckError_NoRegistryUpdate(t *testing.T) {
	dir := t.TempDir()
	mnt := mountedDir(t)
	cfg := newMockConfig(dir)
	checkErr := errors.New("repo integrity failure")
	b := &mockBackend{isReady: true, checkErr: checkErr}
	eng, _ := cold.NewEngine(b, cfg)
	_ = eng.Register(cold.DriveRecord{Label: "drive-a", MountPath: mnt})

	_, err := eng.Run(context.Background(), cold.RunOptions{})
	if err == nil {
		t.Fatal("expected check error, got nil")
	}

	statuses := eng.Status()
	if !statuses[0].Drive.LastCopyTime.IsZero() {
		t.Error("LastCopyTime should remain zero after check failure")
	}
}

func TestEngine_Run_Idempotent(t *testing.T) {
	dir := t.TempDir()
	mnt := mountedDir(t)
	cfg := newMockConfig(dir)
	b := &mockBackend{isReady: true}
	eng, _ := cold.NewEngine(b, cfg)
	_ = eng.Register(cold.DriveRecord{Label: "drive-a", MountPath: mnt})

	// Run twice.
	for i := range 2 {
		_, err := eng.Run(context.Background(), cold.RunOptions{})
		if err != nil {
			t.Fatalf("Run[%d]: %v", i, err)
		}
	}

	// Both runs should have called copy (restic copy is idempotent).
	if len(b.copyCalled) != 2 {
		t.Errorf("CopySnapshotsTo call count: got %d, want 2", len(b.copyCalled))
	}
	// TotalCopies in registry should reflect both runs.
	statuses := eng.Status()
	if statuses[0].Drive.TotalCopies != 2 {
		t.Errorf("TotalCopies: got %d, want 2", statuses[0].Drive.TotalCopies)
	}
}

func TestEngine_Run_Rotation(t *testing.T) {
	dir := t.TempDir()
	cfg := newMockConfig(dir)
	b := &mockBackend{isReady: true}
	eng, _ := cold.NewEngine(b, cfg)

	// Two drives, both mounted. wd-1 never copied, wd-2 copied recently.
	mnt1 := filepath.Join(dir, "mnt1")
	mnt2 := filepath.Join(dir, "mnt2")
	_ = os.MkdirAll(mnt1, 0o755)
	_ = os.MkdirAll(mnt2, 0o755)

	_ = eng.Register(cold.DriveRecord{Label: "wd-1", MountPath: mnt1})
	_ = eng.Register(cold.DriveRecord{
		Label:        "wd-2",
		MountPath:    mnt2,
		LastCopyTime: time.Now().Add(-1 * time.Hour),
	})

	result, err := eng.Run(context.Background(), cold.RunOptions{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// Should choose wd-1 (never copied, oldest).
	if result.Drive.Label != "wd-1" {
		t.Errorf("rotation: got drive %q, want wd-1", result.Drive.Label)
	}
}

func TestEngine_Run_SkipRetention(t *testing.T) {
	dir := t.TempDir()
	mnt := mountedDir(t)
	cfg := newMockConfig(dir)
	b := &mockBackend{isReady: true}
	eng, _ := cold.NewEngine(b, cfg)
	_ = eng.Register(cold.DriveRecord{Label: "drive-a", MountPath: mnt})

	_, err := eng.Run(context.Background(), cold.RunOptions{SkipRetention: true})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(b.forgetCalled) != 0 {
		t.Errorf("ForgetInLocalRepo called despite SkipRetention=true")
	}
}

func TestEngine_Run_RetentionOverride(t *testing.T) {
	dir := t.TempDir()
	mnt := mountedDir(t)
	cfg := newMockConfig(dir)
	cfg.cfg.Retention = cold.RetentionPolicy{KeepLast: 5}
	b := &mockBackend{isReady: true}
	eng, _ := cold.NewEngine(b, cfg)
	_ = eng.Register(cold.DriveRecord{Label: "drive-a", MountPath: mnt})

	override := cold.RetentionPolicy{KeepLast: 99}
	_, err := eng.Run(context.Background(), cold.RunOptions{RetentionOverride: &override})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(b.forgetCalled) != 1 {
		t.Fatalf("ForgetInLocalRepo not called; got %d calls", len(b.forgetCalled))
	}
	// The override was applied; the mock doesn't inspect the policy value but
	// we confirm forget was called exactly once.
}

func TestEngine_Run_SpecificDriveLabel(t *testing.T) {
	dir := t.TempDir()
	mnt1 := filepath.Join(dir, "mnt1")
	mnt2 := filepath.Join(dir, "mnt2")
	_ = os.MkdirAll(mnt1, 0o755)
	_ = os.MkdirAll(mnt2, 0o755)

	cfg := newMockConfig(dir)
	b := &mockBackend{isReady: true}
	eng, _ := cold.NewEngine(b, cfg)
	_ = eng.Register(cold.DriveRecord{Label: "wd-1", MountPath: mnt1})
	_ = eng.Register(cold.DriveRecord{Label: "wd-2", MountPath: mnt2})

	result, err := eng.Run(context.Background(), cold.RunOptions{DriveLabel: "wd-2"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Drive.Label != "wd-2" {
		t.Errorf("got drive %q, want wd-2", result.Drive.Label)
	}
}

func TestEngine_Status(t *testing.T) {
	dir := t.TempDir()
	cfg := newMockConfig(dir)
	cfg.cfg.StalenessWarningDays = 30
	cfg.cfg.StalenessErrorDays = 90

	b := &mockBackend{isReady: true}
	eng, _ := cold.NewEngine(b, cfg)

	// never copied
	_ = eng.Register(cold.DriveRecord{Label: "new-drive", MountPath: "/nonexistent/z1"})

	// copied recently (OK)
	_ = eng.Register(cold.DriveRecord{
		Label:        "fresh-drive",
		MountPath:    "/nonexistent/z2",
		LastCopyTime: time.Now().Add(-5 * 24 * time.Hour),
	})

	// copied 60 days ago (Warning)
	_ = eng.Register(cold.DriveRecord{
		Label:        "stale-drive",
		MountPath:    "/nonexistent/z3",
		LastCopyTime: time.Now().Add(-60 * 24 * time.Hour),
	})

	// copied 100 days ago (Error)
	_ = eng.Register(cold.DriveRecord{
		Label:        "error-drive",
		MountPath:    "/nonexistent/z4",
		LastCopyTime: time.Now().Add(-100 * 24 * time.Hour),
	})

	statuses := eng.Status()
	if len(statuses) != 4 {
		t.Fatalf("Status: got %d entries, want 4", len(statuses))
	}

	byLabel := make(map[string]cold.DriveStatus)
	for _, s := range statuses {
		byLabel[s.Drive.Label] = s
	}

	cases := []struct {
		label string
		want  cold.StalenessLevel
	}{
		{"new-drive", cold.StalenessUnknown},
		{"fresh-drive", cold.StalenessOK},
		{"stale-drive", cold.StalenessWarning},
		{"error-drive", cold.StalenessError},
	}
	for _, tc := range cases {
		s, ok := byLabel[tc.label]
		if !ok {
			t.Errorf("Status: missing entry for %q", tc.label)
			continue
		}
		if s.Staleness.Level != tc.want {
			t.Errorf("%s: staleness level got %v, want %v", tc.label, s.Staleness.Level, tc.want)
		}
	}
}

func TestEngine_Register_EmptyLabel(t *testing.T) {
	dir := t.TempDir()
	cfg := newMockConfig(dir)
	b := &mockBackend{}
	eng, _ := cold.NewEngine(b, cfg)

	if err := eng.Register(cold.DriveRecord{Label: "", MountPath: "/tmp"}); err == nil {
		t.Error("expected error for empty label")
	}
}

func TestEngine_Register_EmptyMountPath(t *testing.T) {
	dir := t.TempDir()
	cfg := newMockConfig(dir)
	b := &mockBackend{}
	eng, _ := cold.NewEngine(b, cfg)

	if err := eng.Register(cold.DriveRecord{Label: "wd-1", MountPath: ""}); err == nil {
		t.Error("expected error for empty mount path")
	}
}

func TestOverallStaleness(t *testing.T) {
	cases := []struct {
		name     string
		statuses []cold.DriveStatus
		want     cold.StalenessLevel
	}{
		{
			name:     "empty",
			statuses: nil,
			want:     cold.StalenessUnknown,
		},
		{
			name: "all ok",
			statuses: []cold.DriveStatus{
				{Staleness: cold.Staleness{Level: cold.StalenessOK}},
				{Staleness: cold.Staleness{Level: cold.StalenessOK}},
			},
			want: cold.StalenessOK,
		},
		{
			name: "one warning",
			statuses: []cold.DriveStatus{
				{Staleness: cold.Staleness{Level: cold.StalenessOK}},
				{Staleness: cold.Staleness{Level: cold.StalenessWarning}},
			},
			want: cold.StalenessWarning,
		},
		{
			name: "error wins",
			statuses: []cold.DriveStatus{
				{Staleness: cold.Staleness{Level: cold.StalenessWarning}},
				{Staleness: cold.Staleness{Level: cold.StalenessError}},
			},
			want: cold.StalenessError,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := cold.OverallStaleness(tc.statuses)
			if got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

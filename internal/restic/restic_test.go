package restic_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/kevinthelago/redoubt/internal/restic"
)

const testPassword = "test-password-redoubt"

func skipIfResticMissing(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("restic"); err != nil {
		t.Skip("restic binary not in PATH; skipping integration test")
	}
}

func newLocalRunner(repoPath string) *restic.Runner {
	return restic.New(
		restic.Backend{Kind: restic.BackendLocal, Path: repoPath},
		restic.WithPassword(testPassword),
	)
}

func TestInit_And_Snapshots_Empty(t *testing.T) {
	skipIfResticMissing(t)
	dir := t.TempDir()
	repoDir := filepath.Join(dir, "repo")

	r := newLocalRunner(repoDir)
	ctx := context.Background()

	if err := r.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}

	snaps, err := r.Snapshots(ctx, nil)
	if err != nil {
		t.Fatalf("Snapshots: %v", err)
	}
	if len(snaps) != 0 {
		t.Errorf("expected 0 snapshots after init, got %d", len(snaps))
	}
}

func TestBackup_And_Snapshots(t *testing.T) {
	skipIfResticMissing(t)
	dir := t.TempDir()
	repoDir := filepath.Join(dir, "repo")
	srcDir := filepath.Join(dir, "src")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "hello.txt"), []byte("hello redoubt"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := newLocalRunner(repoDir)
	ctx := context.Background()

	if err := r.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}

	summary, err := r.Backup(ctx, []string{srcDir}, []string{"test"}, nil)
	if err != nil {
		t.Fatalf("Backup: %v", err)
	}
	if summary.SnapshotID == "" {
		t.Error("BackupSummary.SnapshotID is empty")
	}
	if summary.FilesNew < 1 {
		t.Errorf("FilesNew = %d, want >= 1", summary.FilesNew)
	}

	snaps, err := r.Snapshots(ctx, nil)
	if err != nil {
		t.Fatalf("Snapshots: %v", err)
	}
	if len(snaps) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(snaps))
	}
	if snaps[0].ShortID == "" {
		t.Error("Snapshot.ShortID is empty")
	}
}

func TestCheck_EmptyRepo(t *testing.T) {
	skipIfResticMissing(t)
	dir := t.TempDir()
	repoDir := filepath.Join(dir, "repo")

	r := newLocalRunner(repoDir)
	ctx := context.Background()

	if err := r.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := r.Check(ctx, false); err != nil {
		t.Errorf("Check on empty repo: %v", err)
	}
}

func TestWrongPassword_ReturnsError(t *testing.T) {
	skipIfResticMissing(t)
	dir := t.TempDir()
	repoDir := filepath.Join(dir, "repo")

	// Init with one password.
	r := newLocalRunner(repoDir)
	ctx := context.Background()
	if err := r.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Open with the wrong password.
	wrong := restic.New(
		restic.Backend{Kind: restic.BackendLocal, Path: repoDir},
		restic.WithPassword("wrong-password"),
	)
	_, err := wrong.Snapshots(ctx, nil)
	if err == nil {
		t.Fatal("expected error with wrong password, got nil")
	}
}

func TestBinaryMissing_ReturnsError(t *testing.T) {
	r := restic.New(
		restic.Backend{Kind: restic.BackendLocal, Path: "/tmp/norepo"},
		restic.WithBinary("/nonexistent/restic"),
		restic.WithPassword(testPassword),
	)
	err := r.Init(context.Background())
	if err == nil {
		t.Fatal("expected error for missing binary, got nil")
	}
}

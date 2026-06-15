package assets_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kevinthelago/redoubt/internal/assets"
)

func TestStore_AddAndList(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	mustMkdir(t, src)

	s := newStore(t, dir)

	if err := s.Add(src, assets.CategorySource, ""); err != nil {
		t.Fatalf("Add: %v", err)
	}
	items := s.List()
	if len(items) != 1 {
		t.Fatalf("want 1 item, got %d", len(items))
	}
	if items[0].Path != src {
		t.Errorf("want path %s, got %s", src, items[0].Path)
	}
	if items[0].Category != assets.CategorySource {
		t.Errorf("want category source, got %s", items[0].Category)
	}
}

func TestStore_Load_Roundtrip(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	mustMkdir(t, src)

	s := newStore(t, dir)
	mustAdd(t, s, src, assets.CategoryConfig, "")

	// Reload from disk.
	s2 := assets.NewStore(dir, assets.NopLogger())
	if err := s2.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	items := s2.List()
	if len(items) != 1 {
		t.Fatalf("want 1 item after reload, got %d", len(items))
	}
	if items[0].Category != assets.CategoryConfig {
		t.Errorf("category not persisted: got %s", items[0].Category)
	}
}

func TestStore_Add_NonexistentPath(t *testing.T) {
	dir := t.TempDir()
	s := newStore(t, dir)
	err := s.Add(filepath.Join(dir, "missing"), assets.CategorySource, "")
	if err == nil {
		t.Fatal("expected error for non-existent path")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("unexpected error text: %v", err)
	}
}

func TestStore_Add_InvalidCategory(t *testing.T) {
	dir := t.TempDir()
	s := newStore(t, dir)
	err := s.Add(dir, "unknown", "")
	if err == nil {
		t.Fatal("expected error for invalid category")
	}
}

func TestStore_Add_DuplicatePath(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	mustMkdir(t, src)

	s := newStore(t, dir)
	mustAdd(t, s, src, assets.CategorySource, "")
	// Second add of the same path should be silently skipped.
	if err := s.Add(src, assets.CategorySource, ""); err != nil {
		t.Fatalf("second Add returned error: %v", err)
	}
	if len(s.List()) != 1 {
		t.Errorf("want 1 item (duplicate skipped), got %d", len(s.List()))
	}
}

func TestStore_Add_OverlappingPaths(t *testing.T) {
	dir := t.TempDir()
	parent := filepath.Join(dir, "parent")
	child := filepath.Join(parent, "child")
	mustMkdir(t, child)

	var warnings []string
	log := capturingLogger{warnFn: func(msg string, _ ...any) { warnings = append(warnings, msg) }}

	s := assets.NewStore(dir, log)
	if err := s.Load(); err != nil {
		t.Fatal(err)
	}
	mustAdd(t, s, parent, assets.CategorySource, "")
	if err := s.Add(child, assets.CategorySource, ""); err != nil {
		t.Fatalf("Add child: %v", err)
	}
	if len(warnings) == 0 {
		t.Error("expected overlap warning, got none")
	}
	// Both paths should be tracked despite the overlap.
	if len(s.List()) != 2 {
		t.Errorf("want 2 items, got %d", len(s.List()))
	}
}

func TestStore_Add_Database_MissingDumpCommand(t *testing.T) {
	dir := t.TempDir()
	db := filepath.Join(dir, "db")
	mustMkdir(t, db)

	s := newStore(t, dir)
	err := s.Add(db, assets.CategoryDatabase, "")
	if err == nil {
		t.Fatal("expected error for missing dump command")
	}
}

func TestStore_Add_Database_UnknownBinary(t *testing.T) {
	dir := t.TempDir()
	db := filepath.Join(dir, "db")
	mustMkdir(t, db)

	s := newStore(t, dir)
	err := s.Add(db, assets.CategoryDatabase, "no_such_binary_xyz_abc --flag")
	if err == nil {
		t.Fatal("expected error for unknown dump binary")
	}
	if !strings.Contains(err.Error(), "not found in PATH") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestStore_Remove(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	mustMkdir(t, src)

	s := newStore(t, dir)
	mustAdd(t, s, src, assets.CategorySource, "")
	if err := s.Remove(src); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if len(s.List()) != 0 {
		t.Error("expected empty list after Remove")
	}
}

func TestStore_Remove_NotTracked(t *testing.T) {
	dir := t.TempDir()
	s := newStore(t, dir)
	err := s.Remove(filepath.Join(dir, "ghost"))
	if err == nil {
		t.Fatal("expected error removing untracked path")
	}
	if !strings.Contains(err.Error(), "not tracked") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestStore_IsEmpty(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	mustMkdir(t, src)

	s := newStore(t, dir)
	if !s.IsEmpty() {
		t.Error("new store should be empty")
	}
	mustAdd(t, s, src, assets.CategorySource, "")
	if s.IsEmpty() {
		t.Error("store should not be empty after Add")
	}
}

func TestFormatSize(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.0 KiB"},
		{1024 * 1024, "1.0 MiB"},
		{-1, "?"},
	}
	for _, c := range cases {
		got := assets.FormatSize(c.in)
		if got != c.want {
			t.Errorf("FormatSize(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}

// helpers

func newStore(t *testing.T, dir string) *assets.Store {
	t.Helper()
	s := assets.NewStore(dir, assets.NopLogger())
	if err := s.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	return s
}

func mustAdd(t *testing.T, s *assets.Store, path string, cat assets.Category, dump string) {
	t.Helper()
	if err := s.Add(path, cat, dump); err != nil {
		t.Fatalf("Add(%s, %s): %v", path, cat, err)
	}
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

// capturingLogger captures Warn calls.
type capturingLogger struct {
	warnFn func(string, ...any)
}

func (l capturingLogger) Info(msg string, args ...any)  {}
func (l capturingLogger) Error(msg string, args ...any) {}
func (l capturingLogger) Debug(msg string, args ...any) {}
func (l capturingLogger) Warn(msg string, args ...any) {
	if l.warnFn != nil {
		l.warnFn(msg, args...)
	}
}

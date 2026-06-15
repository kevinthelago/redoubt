package cold_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kevinthelago/redoubt/internal/cold"
)

func tempRegistryPath(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	return filepath.Join(dir, "cold_drives.toml")
}

func makeRecord(label, mountPath string) cold.DriveRecord {
	return cold.DriveRecord{
		Label:     label,
		MountPath: mountPath,
	}
}

func TestRegistry_AddAndGet(t *testing.T) {
	path := tempRegistryPath(t)
	reg, err := cold.LoadRegistry(path)
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}

	d := makeRecord("wd-1", "/tmp/drive1")
	if err := reg.Add(d); err != nil {
		t.Fatalf("Add: %v", err)
	}

	got, ok := reg.Get("wd-1")
	if !ok {
		t.Fatal("Get: expected to find wd-1")
	}
	if got.Label != d.Label {
		t.Errorf("label: got %q, want %q", got.Label, d.Label)
	}
	if got.MountPath != d.MountPath {
		t.Errorf("mount path: got %q, want %q", got.MountPath, d.MountPath)
	}
}

func TestRegistry_DuplicateLabelRejected(t *testing.T) {
	path := tempRegistryPath(t)
	reg, _ := cold.LoadRegistry(path)
	_ = reg.Add(makeRecord("wd-1", "/tmp/d1"))
	if err := reg.Add(makeRecord("wd-1", "/tmp/d2")); err == nil {
		t.Fatal("expected error for duplicate label, got nil")
	}
}

func TestRegistry_Remove(t *testing.T) {
	path := tempRegistryPath(t)
	reg, _ := cold.LoadRegistry(path)
	_ = reg.Add(makeRecord("wd-1", "/tmp/d1"))
	if err := reg.Remove("wd-1"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, ok := reg.Get("wd-1"); ok {
		t.Fatal("drive still present after Remove")
	}
}

func TestRegistry_RemoveNotFound(t *testing.T) {
	path := tempRegistryPath(t)
	reg, _ := cold.LoadRegistry(path)
	if err := reg.Remove("nonexistent"); err == nil {
		t.Fatal("expected error removing nonexistent drive")
	}
}

func TestRegistry_List_Sorted(t *testing.T) {
	path := tempRegistryPath(t)
	reg, _ := cold.LoadRegistry(path)
	_ = reg.Add(makeRecord("wd-2", "/tmp/d2"))
	_ = reg.Add(makeRecord("wd-1", "/tmp/d1"))
	_ = reg.Add(makeRecord("wd-3", "/tmp/d3"))

	list := reg.List()
	if len(list) != 3 {
		t.Fatalf("List: got %d drives, want 3", len(list))
	}
	labels := []string{list[0].Label, list[1].Label, list[2].Label}
	want := []string{"wd-1", "wd-2", "wd-3"}
	for i, l := range labels {
		if l != want[i] {
			t.Errorf("list[%d]: got %q, want %q", i, l, want[i])
		}
	}
}

func TestRegistry_Persistence(t *testing.T) {
	path := tempRegistryPath(t)

	// write
	reg1, _ := cold.LoadRegistry(path)
	_ = reg1.Add(makeRecord("wd-1", "/mnt/wd1"))

	// read back in a new instance
	reg2, err := cold.LoadRegistry(path)
	if err != nil {
		t.Fatalf("LoadRegistry round-trip: %v", err)
	}
	d, ok := reg2.Get("wd-1")
	if !ok {
		t.Fatal("drive not found after reload")
	}
	if d.MountPath != "/mnt/wd1" {
		t.Errorf("mount path after reload: got %q, want %q", d.MountPath, "/mnt/wd1")
	}
}

func TestRegistry_RecordCopy(t *testing.T) {
	path := tempRegistryPath(t)
	reg, _ := cold.LoadRegistry(path)
	_ = reg.Add(makeRecord("wd-1", "/tmp/d1"))
	_ = reg.Add(makeRecord("wd-2", "/tmp/d2"))

	now := time.Now().Truncate(time.Second)
	if err := reg.RecordCopy("wd-1", now); err != nil {
		t.Fatalf("RecordCopy: %v", err)
	}

	d, _ := reg.Get("wd-1")
	if !d.LastCopyTime.Equal(now) {
		t.Errorf("LastCopyTime: got %v, want %v", d.LastCopyTime, now)
	}
	if d.TotalCopies != 1 {
		t.Errorf("TotalCopies: got %d, want 1", d.TotalCopies)
	}
	if !d.Active {
		t.Error("Active should be true after RecordCopy")
	}

	// wd-2 should have Active cleared
	d2, _ := reg.Get("wd-2")
	if d2.Active {
		t.Error("wd-2 Active should be false after another drive's RecordCopy")
	}
}

func TestRegistry_BestCandidate_PrefersMountedWithOldestCopy(t *testing.T) {
	// Create a temp dir to simulate mount points.
	dir := t.TempDir()
	mnt1 := filepath.Join(dir, "mnt1")
	mnt2 := filepath.Join(dir, "mnt2")
	if err := os.MkdirAll(mnt1, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(mnt2, 0o755); err != nil {
		t.Fatal(err)
	}

	path := tempRegistryPath(t)
	reg, _ := cold.LoadRegistry(path)

	older := time.Now().Add(-48 * time.Hour)
	newer := time.Now().Add(-2 * time.Hour)

	d1 := cold.DriveRecord{Label: "wd-1", MountPath: mnt1, LastCopyTime: older}
	d2 := cold.DriveRecord{Label: "wd-2", MountPath: mnt2, LastCopyTime: newer}
	_ = reg.Add(d1)
	_ = reg.Add(d2)

	best, ok := reg.BestCandidate()
	if !ok {
		t.Fatal("BestCandidate: got none, expected wd-1")
	}
	if best.Label != "wd-1" {
		t.Errorf("BestCandidate: got %q, want %q", best.Label, "wd-1")
	}
}

func TestRegistry_BestCandidate_NeverCopiedFirst(t *testing.T) {
	dir := t.TempDir()
	mnt1 := filepath.Join(dir, "mnt1")
	mnt2 := filepath.Join(dir, "mnt2")
	_ = os.MkdirAll(mnt1, 0o755)
	_ = os.MkdirAll(mnt2, 0o755)

	path := tempRegistryPath(t)
	reg, _ := cold.LoadRegistry(path)

	// wd-1 has never been copied; wd-2 was copied recently.
	d1 := cold.DriveRecord{Label: "wd-1", MountPath: mnt1}
	d2 := cold.DriveRecord{Label: "wd-2", MountPath: mnt2, LastCopyTime: time.Now()}
	_ = reg.Add(d1)
	_ = reg.Add(d2)

	best, ok := reg.BestCandidate()
	if !ok {
		t.Fatal("BestCandidate: expected a candidate")
	}
	if best.Label != "wd-1" {
		t.Errorf("BestCandidate: got %q, want wd-1 (never copied)", best.Label)
	}
}

func TestRegistry_BestCandidate_NoneWhenUnmounted(t *testing.T) {
	path := tempRegistryPath(t)
	reg, _ := cold.LoadRegistry(path)
	// mount path does not exist
	_ = reg.Add(makeRecord("wd-1", "/nonexistent/path/drive1"))

	_, ok := reg.BestCandidate()
	if ok {
		t.Fatal("BestCandidate: expected false when no drive is mounted")
	}
}

func TestDriveRecord_RepoPath(t *testing.T) {
	tests := []struct {
		label   string
		mount   string
		subdir  string
		wantSep string
	}{
		{label: "default-subdir", mount: "/mnt/drive", subdir: "", wantSep: "redoubt"},
		{label: "custom-subdir", mount: "/mnt/drive", subdir: "backup", wantSep: "backup"},
	}
	for _, tt := range tests {
		t.Run(tt.label, func(t *testing.T) {
			d := cold.DriveRecord{MountPath: tt.mount, RepoSubdir: tt.subdir}
			got := d.RepoPath()
			if filepath.Base(got) != tt.wantSep {
				t.Errorf("RepoPath base: got %q, want %q", filepath.Base(got), tt.wantSep)
			}
		})
	}
}

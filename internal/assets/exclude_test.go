package assets_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kevinthelago/redoubt/internal/assets"
)

func TestExcluder_BasicGlob(t *testing.T) {
	ex := assets.NewExcluder([]string{"*.log", "tmp/"})

	cases := []struct {
		path     string
		excluded bool
	}{
		{"access.log", true},
		{"dir/access.log", true},
		{"access.txt", false},
		{"tmp/file", true},
		{"other/file", false},
	}
	for _, c := range cases {
		got := ex.IsExcluded(c.path)
		if got != c.excluded {
			t.Errorf("IsExcluded(%q) = %v, want %v", c.path, got, c.excluded)
		}
	}
}

func TestExcluder_Negation(t *testing.T) {
	ex := assets.NewExcluder([]string{"*.log", "!important.log"})

	if !ex.IsExcluded("debug.log") {
		t.Error("debug.log should be excluded")
	}
	if ex.IsExcluded("important.log") {
		t.Error("important.log should NOT be excluded (negation)")
	}
}

func TestExcluder_Comments(t *testing.T) {
	ex := assets.NewExcluder([]string{"# this is a comment", "*.bak"})
	if !ex.IsExcluded("file.bak") {
		t.Error("file.bak should be excluded")
	}
}

func TestExcluder_EmptyExcluder(t *testing.T) {
	ex := assets.NewExcluder(nil)
	if ex.IsExcluded("anything") {
		t.Error("empty excluder should not exclude anything")
	}
}

func TestExcluder_Merge(t *testing.T) {
	a := assets.NewExcluder([]string{"*.log"})
	b := assets.NewExcluder([]string{"*.tmp"})
	merged := a.Merge(b)

	if !merged.IsExcluded("run.log") {
		t.Error("merged excluder should exclude *.log")
	}
	if !merged.IsExcluded("work.tmp") {
		t.Error("merged excluder should exclude *.tmp")
	}
	if merged.IsExcluded("main.go") {
		t.Error("merged excluder should not exclude main.go")
	}
}

func TestLoadExcludeFile_Missing(t *testing.T) {
	ex, err := assets.LoadExcludeFile(filepath.Join(t.TempDir(), "noexist"))
	if err != nil {
		t.Fatalf("missing file should not error: %v", err)
	}
	if ex.IsExcluded("anything") {
		t.Error("excluder from missing file should exclude nothing")
	}
}

func TestLoadExcludeFile_Roundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".redoubtignore")
	content := "# ignore logs\n*.log\n!important.log\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	ex, err := assets.LoadExcludeFile(path)
	if err != nil {
		t.Fatalf("LoadExcludeFile: %v", err)
	}
	if !ex.IsExcluded("error.log") {
		t.Error("error.log should be excluded")
	}
	if ex.IsExcluded("important.log") {
		t.Error("important.log should not be excluded")
	}
}

func TestExcluder_AnchoredPattern(t *testing.T) {
	// Pattern with a slash is matched against the full path.
	ex := assets.NewExcluder([]string{"build/output"})

	if !ex.IsExcluded("build/output") {
		t.Error("build/output should be excluded")
	}
	if ex.IsExcluded("output") {
		t.Error("bare output should NOT be excluded by anchored pattern")
	}
}

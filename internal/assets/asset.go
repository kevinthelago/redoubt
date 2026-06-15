// Package assets manages the set of filesystem paths that Redoubt backs up.
package assets

import (
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Category identifies the kind of asset being tracked.
type Category string

const (
	CategorySource   Category = "source"
	CategorySecrets  Category = "secrets"
	CategoryDatabase Category = "database"
	CategoryConfig   Category = "config"
)

// ValidCategories lists all recognised Category values.
var ValidCategories = []Category{
	CategorySource,
	CategorySecrets,
	CategoryDatabase,
	CategoryConfig,
}

// IsValid reports whether c is a recognised category.
func (c Category) IsValid() bool {
	for _, v := range ValidCategories {
		if c == v {
			return true
		}
	}
	return false
}

// DBPreset describes a known database dump tool and its default command template.
// %s in Template is replaced with the database name / file supplied by the user.
type DBPreset struct {
	Name     string
	Binary   string
	Template string
}

// DBPresets lists the built-in database dump presets.
var DBPresets = []DBPreset{
	{Name: "pg_dump", Binary: "pg_dump", Template: "pg_dump -Fc %s"},
	{Name: "sqlite3", Binary: "sqlite3", Template: "sqlite3 %s .dump"},
	{Name: "mysqldump", Binary: "mysqldump", Template: "mysqldump --single-transaction --no-tablespaces %s"},
}

// PresetCommand returns the template string for a named preset ("pg_dump", "sqlite3",
// "mysqldump"). Returns ("", false) if name is not a recognised preset.
func PresetCommand(name string) (string, bool) {
	for _, p := range DBPresets {
		if p.Name == name {
			return p.Template, true
		}
	}
	return "", false
}

// ValidateDumpCommand verifies that cmd is non-empty and that its leading binary
// is findable on PATH. It does not execute the command.
func ValidateDumpCommand(cmd string) error {
	if strings.TrimSpace(cmd) == "" {
		return fmt.Errorf("database assets require a dump command (pass --dump-command or a preset name: pg_dump, sqlite3, mysqldump)")
	}
	fields := strings.Fields(cmd)
	if len(fields) == 0 {
		return fmt.Errorf("dump command must not be blank")
	}
	if _, err := exec.LookPath(fields[0]); err != nil {
		return fmt.Errorf("dump command binary %q not found in PATH", fields[0])
	}
	return nil
}

// Asset is a tracked filesystem path with its backup category and options.
type Asset struct {
	Path        string    `json:"path"`
	Category    Category  `json:"category"`
	DumpCommand string    `json:"dump_command,omitempty"` // database category only
	AddedAt     time.Time `json:"added_at"`
}

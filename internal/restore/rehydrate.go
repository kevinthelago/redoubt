package restore

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// detectDBDumps scans the restored file tree for files that look like database
// dumps, based on snapshot category tags and known dump file extensions.
func detectDBDumps(targetDir string, entries []FileEntry) []DBDump {
	var dumps []DBDump
	for _, e := range entries {
		if e.Type != "file" {
			continue
		}
		preset := dumpPreset(e.Path, e.Tags)
		fullPath := filepath.Join(targetDir, normalizePath(e.Path))
		if preset != "" || isDBDumpExtension(e.Path) {
			dumps = append(dumps, DBDump{
				Path:        fullPath,
				Category:    "database",
				RehydratCmd: expandPreset(preset, fullPath),
			})
		}
	}
	return dumps
}

// OfferRehydrate presents each detected dump to the operator and, after
// explicit per-dump "yes" confirmation, runs its preset import command.
// It never imports automatically — the operator types "yes" for each one.
func (p *Pipeline) OfferRehydrate(ctx context.Context, dumps []DBDump) error {
	if len(dumps) == 0 {
		return nil
	}

	scanner := bufio.NewScanner(p.in)
	for _, dump := range dumps {
		fmt.Fprintf(p.out, "\nDatabase dump detected: %s\n", dump.Path)
		if dump.RehydratCmd == "" {
			fmt.Fprintln(p.out, "No preset import command configured. Import manually when ready.")
			continue
		}

		fmt.Fprintf(p.out, "Preset import command: %s\n", dump.RehydratCmd)
		fmt.Fprint(p.out, "Run this import now? This will modify the target database. (yes/N): ")

		if !scanner.Scan() {
			return nil // EOF or no input → skip all remaining
		}
		answer := strings.TrimSpace(scanner.Text())
		if answer != "yes" {
			fmt.Fprintln(p.out, "Import skipped. Run the command manually when ready.")
			continue
		}

		fmt.Fprintf(p.out, "Running: %s\n", dump.RehydratCmd)
		parts := strings.Fields(dump.RehydratCmd)
		if len(parts) == 0 {
			fmt.Fprintln(p.out, "Import command is empty; skipping.")
			continue
		}
		//nolint:gosec // operator-supplied command from their own backup config
		cmd := exec.CommandContext(ctx, parts[0], parts[1:]...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			fmt.Fprintf(p.out, "Import failed: %v\n%s\n", err, out)
		} else {
			fmt.Fprintf(p.out, "Import completed.\n%s\n", out)
		}
	}
	return nil
}

// dumpPreset returns the preset import command template for a dump file.
// Returns "" if the file has no recognised import preset.
func dumpPreset(path string, tags []string) string {
	// Snapshot tags added by the backup pipeline take precedence.
	for _, t := range tags {
		if strings.HasPrefix(t, "preset:") {
			return strings.TrimPrefix(t, "preset:")
		}
	}
	// Heuristic by file extension.
	switch {
	case hasSuffix(path, ".sql", ".sql.gz"):
		return "psql --file={{.File}}"
	case hasSuffix(path, ".dump"):
		return "pg_restore --dbname=<dbname> {{.File}}"
	}
	return ""
}

func isDBDumpExtension(path string) bool {
	return hasSuffix(path, ".sql", ".sql.gz", ".dump", ".sqlite", ".sqlite3")
}

func expandPreset(preset, filePath string) string {
	return strings.ReplaceAll(preset, "{{.File}}", filePath)
}

func hasSuffix(path string, suffixes ...string) bool {
	lower := strings.ToLower(path)
	for _, s := range suffixes {
		if strings.HasSuffix(lower, s) {
			return true
		}
	}
	return false
}

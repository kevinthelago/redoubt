package backup

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// dumpTimeout caps how long a database dump command may run.
// 15 minutes is generous for local dumps; a hung pg_dump shouldn't block forever.
const dumpTimeout = 15 * time.Minute

// dumpDB runs the asset's dump command and captures its stdout to a temp file.
// Returns the path of the dump file on success; the caller owns cleanup.
//
// Stderr from the dump command is captured but never logged verbatim — it may
// contain connection strings or credentials. Only the byte count is logged.
func dumpDB(ctx context.Context, a TrackedAsset, tmpDir string, log Logger) (string, error) {
	if len(a.DumpCmd) == 0 {
		return "", fmt.Errorf("asset %q has no dump command configured", a.ID)
	}

	dumpCtx, cancel := context.WithTimeout(ctx, dumpTimeout)
	defer cancel()

	dumpPath := filepath.Join(tmpDir, "dump-"+sanitizeName(a.ID)+".dump")

	f, err := os.OpenFile(dumpPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", fmt.Errorf("create dump file for asset %q: %w", a.ID, err)
	}

	var stderr bytes.Buffer
	cmd := exec.CommandContext(dumpCtx, a.DumpCmd[0], a.DumpCmd[1:]...)
	cmd.Stdout = f
	cmd.Stderr = &stderr // captured; never echoed (may contain credentials)

	log.Info("dumping database asset", "asset", a.ID, "executable", a.DumpCmd[0])

	runErr := cmd.Run()
	closeErr := f.Close()

	if runErr != nil {
		os.Remove(dumpPath) // remove partial dump; wipe not needed (empty or partial)
		// Log stderr byte count only, never its content.
		log.Warn("dump command failed",
			"asset", a.ID,
			"err", runErr,
			"stderr_bytes", stderr.Len(),
		)
		return "", fmt.Errorf("dump command for asset %q failed: %w", a.ID, runErr)
	}
	if closeErr != nil {
		os.Remove(dumpPath)
		return "", fmt.Errorf("flush dump file for asset %q: %w", a.ID, closeErr)
	}

	log.Info("dump complete", "asset", a.ID, "path", dumpPath)
	return dumpPath, nil
}

// sanitizeName replaces characters that are unsafe in a filename with underscores.
func sanitizeName(s string) string {
	out := make([]byte, len(s))
	for i, c := range []byte(s) {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '-':
			out[i] = c
		default:
			out[i] = '_'
		}
	}
	return string(out)
}

package engine

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Runner executes the redoubt CLI binary and parses its output.
type Runner struct {
	enginePath string
}

// NewRunner creates a Runner. It searches for the redoubt binary at startup.
// If the binary is not found, Run calls will return an error.
func NewRunner() *Runner {
	r := &Runner{}
	if path, err := r.FindEngine(); err == nil {
		r.enginePath = path
	}
	return r
}

// FindEngine locates the redoubt binary. It checks:
//  1. The REDOUBT_BIN environment variable.
//  2. A sibling binary next to the current executable.
//  3. The system PATH via exec.LookPath.
func (r *Runner) FindEngine() (string, error) {
	// 1. Explicit env override.
	if env := os.Getenv("REDOUBT_BIN"); env != "" {
		if _, err := os.Stat(env); err == nil {
			return env, nil
		}
	}

	// 2. Sibling binary (installed next to the GUI).
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		candidates := []string{
			filepath.Join(dir, "redoubt"),
			filepath.Join(dir, "redoubt.exe"),
		}
		for _, c := range candidates {
			if _, err := os.Stat(c); err == nil {
				return c, nil
			}
		}
	}

	// 3. System PATH.
	if path, err := exec.LookPath("redoubt"); err == nil {
		return path, nil
	}

	return "", fmt.Errorf("redoubt binary not found: set REDOUBT_BIN or place it in PATH")
}

// engineBin returns the engine path or an error if it was never found.
func (r *Runner) engineBin() (string, error) {
	if r.enginePath != "" {
		return r.enginePath, nil
	}
	path, err := r.FindEngine()
	if err != nil {
		return "", err
	}
	r.enginePath = path
	return path, nil
}

// Run executes `redoubt <args>` and returns the combined stdout bytes.
// Stderr is captured and included in the error message on failure.
func (r *Runner) Run(args ...string) ([]byte, error) {
	bin, err := r.engineBin()
	if err != nil {
		return nil, err
	}

	cmd := exec.Command(bin, args...)
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if ok := asExitError(err, &exitErr); ok {
			return nil, fmt.Errorf("redoubt %s failed (exit %d): %s",
				strings.Join(args, " "), exitErr.ExitCode(), string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("redoubt %s: %w", strings.Join(args, " "), err)
	}
	return out, nil
}

// RunStreaming executes `redoubt <args>` and calls emit for each JSON line on
// stdout. It returns when the process exits or the context is cancelled.
func (r *Runner) RunStreaming(ctx context.Context, emit func(BackupProgress), args ...string) error {
	bin, err := r.engineBin()
	if err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, bin, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start: %w", err)
	}

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Bytes()
		var p BackupProgress
		if err := json.Unmarshal(line, &p); err != nil {
			// Skip non-JSON lines (e.g. log output).
			continue
		}
		emit(p)
	}

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("redoubt %s: %w", strings.Join(args, " "), err)
	}
	return nil
}

// asExitError is a helper to avoid importing errors in the call site.
func asExitError(err error, target **exec.ExitError) bool {
	e, ok := err.(*exec.ExitError)
	if ok {
		*target = e
	}
	return ok
}

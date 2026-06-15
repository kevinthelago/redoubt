// Package restic wraps the restic binary, parsing its JSON output into typed
// Go structs and surfacing errors as sentinel values.
package restic

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Sentinel errors returned by Runner operations.
var (
	ErrRepoNotFound  = errors.New("restic: repository not found")
	ErrWrongPassword = errors.New("restic: wrong password or key")
	ErrBinaryMissing = errors.New("restic: binary not found in PATH")
)

// BackendKind identifies the type of restic repository backend.
type BackendKind string

const (
	BackendLocal BackendKind = "local"
	BackendREST  BackendKind = "rest"
)

// Backend describes the restic repository to operate on.
type Backend struct {
	Kind BackendKind

	// Local backend: filesystem path to the repository.
	Path string

	// REST backend: full HTTPS URL (e.g. "rest:https://vault.lan:8000/repo").
	URL string

	// CACert is the path to a PEM-encoded CA certificate used for TLS
	// verification of REST backends. Leave empty to use the system trust store.
	CACert string
}

// RepoURL returns the value to pass as RESTIC_REPOSITORY.
func (b Backend) RepoURL() string {
	if b.Kind == BackendREST {
		return b.URL
	}
	return b.Path
}

// Runner invokes the restic binary against a specific backend.
type Runner struct {
	binary   string
	backend  Backend
	password string // never logged
}

// Option configures a Runner.
type Option func(*Runner)

// WithBinary sets the path to the restic binary. Defaults to "restic" (PATH lookup).
func WithBinary(path string) Option {
	return func(r *Runner) { r.binary = path }
}

// WithPassword sets the repository password. The value is passed via environment
// variable and is never included in log output.
func WithPassword(pw string) Option {
	return func(r *Runner) { r.password = pw }
}

// New creates a Runner for the given backend.
func New(backend Backend, opts ...Option) *Runner {
	r := &Runner{binary: "restic", backend: backend}
	for _, o := range opts {
		o(r)
	}
	return r
}

// Snapshot represents a single restic snapshot.
type Snapshot struct {
	ID       string    `json:"id"`
	ShortID  string    `json:"short_id"`
	Time     time.Time `json:"time"`
	Tree     string    `json:"tree"`
	Paths    []string  `json:"paths"`
	Tags     []string  `json:"tags,omitempty"`
	Hostname string    `json:"hostname"`
	Username string    `json:"username"`
}

// BackupSummary holds the final statistics from a completed backup operation.
type BackupSummary struct {
	FilesNew        int    `json:"files_new"`
	FilesChanged    int    `json:"files_changed"`
	FilesUnmodified int    `json:"files_unmodified"`
	BytesProcessed  int64  `json:"total_bytes_processed"`
	DataAdded       int64  `json:"data_added"`
	SnapshotID      string `json:"snapshot_id"`
}

// ForgetPolicy maps to restic's retention flags.
type ForgetPolicy struct {
	KeepHourly  int
	KeepDaily   int
	KeepWeekly  int
	KeepMonthly int
}

// Init initialises a new repository at the configured backend.
func (r *Runner) Init(ctx context.Context) error {
	_, err := r.run(ctx, nil, "init", "--json")
	return err
}

// Backup backs up paths with optional tags and exclude patterns.
// Returns the summary from restic's final JSON summary line.
func (r *Runner) Backup(ctx context.Context, paths []string, tags []string, excludes []string) (*BackupSummary, error) {
	args := []string{"backup", "--json"}
	for _, t := range tags {
		args = append(args, "--tag", t)
	}
	for _, ex := range excludes {
		args = append(args, "--exclude", ex)
	}
	args = append(args, paths...)

	out, err := r.run(ctx, nil, args...)
	if err != nil {
		return nil, err
	}
	return parseBackupSummary(out)
}

// Snapshots returns the list of snapshots, optionally filtered by tags.
func (r *Runner) Snapshots(ctx context.Context, tags []string) ([]Snapshot, error) {
	args := []string{"snapshots", "--json"}
	for _, t := range tags {
		args = append(args, "--tag", t)
	}
	out, err := r.run(ctx, nil, args...)
	if err != nil {
		return nil, err
	}
	var snaps []Snapshot
	if err := json.Unmarshal(out, &snaps); err != nil {
		return nil, fmt.Errorf("restic: parse snapshots: %w", err)
	}
	return snaps, nil
}

// Restore restores snapshotID to target. If paths is non-empty, only those
// paths within the snapshot are restored.
func (r *Runner) Restore(ctx context.Context, snapshotID, target string, paths []string) error {
	args := []string{"restore", snapshotID, "--target", target, "--json"}
	for _, p := range paths {
		args = append(args, "--include", p)
	}
	_, err := r.run(ctx, nil, args...)
	return err
}

// Forget applies the given retention policy and optionally prunes unreferenced data.
func (r *Runner) Forget(ctx context.Context, policy ForgetPolicy, prune bool) error {
	args := []string{"forget", "--json",
		"--keep-hourly", fmt.Sprintf("%d", policy.KeepHourly),
		"--keep-daily", fmt.Sprintf("%d", policy.KeepDaily),
		"--keep-weekly", fmt.Sprintf("%d", policy.KeepWeekly),
		"--keep-monthly", fmt.Sprintf("%d", policy.KeepMonthly),
	}
	if prune {
		args = append(args, "--prune")
	}
	_, err := r.run(ctx, nil, args...)
	return err
}

// Check verifies the repository integrity. Pass readData=true to verify all
// pack files (slow, but thorough).
func (r *Runner) Check(ctx context.Context, readData bool) error {
	args := []string{"check", "--json"}
	if readData {
		args = append(args, "--read-data")
	}
	_, err := r.run(ctx, nil, args...)
	return err
}

// LsEntry is a single file or directory entry returned by Ls.
type LsEntry struct {
	Name  string    `json:"name"`
	Type  string    `json:"type"` // "file", "dir", "symlink", …
	Path  string    `json:"path"`
	Size  int64     `json:"size,omitempty"`
	Mtime time.Time `json:"mtime"`
}

// Ls lists the files in snapshotID, optionally restricted to paths beneath prefix.
func (r *Runner) Ls(ctx context.Context, snapshotID string, prefix string) ([]LsEntry, error) {
	args := []string{"ls", "--json", snapshotID}
	if prefix != "" {
		args = append(args, prefix)
	}
	out, err := r.run(ctx, nil, args...)
	if err != nil {
		return nil, err
	}
	return parseLsOutput(out)
}

// FindHit is one match returned by Find.
type FindHit struct {
	SnapshotID string    `json:"snapshot"`
	Path       string    `json:"path"`
	Type       string    `json:"type"`
	Mtime      time.Time `json:"mtime"`
}

// Find searches all snapshots for files matching pattern.
func (r *Runner) Find(ctx context.Context, pattern string, snapshotID string) ([]FindHit, error) {
	args := []string{"find", "--json", pattern}
	if snapshotID != "" {
		args = append(args, "--snapshot", snapshotID)
	}
	out, err := r.run(ctx, nil, args...)
	if err != nil {
		return nil, err
	}
	return parseFindOutput(out)
}

// DiffChange is a single changed path between two snapshots.
type DiffChange struct {
	Path     string `json:"path"`
	Modifier string `json:"modifier"` // "+", "-", "M", "T", "U"
	Type     string `json:"struct_type"`
}

// Diff returns the differences between two snapshots.
func (r *Runner) Diff(ctx context.Context, snapshotA, snapshotB string) ([]DiffChange, error) {
	out, err := r.run(ctx, nil, "diff", "--json", snapshotA, snapshotB)
	if err != nil {
		return nil, err
	}
	return parseDiffOutput(out)
}

// CopyOptions configures a Copy operation.
type CopyOptions struct {
	// Password for the destination repository.  Defaults to the source password
	// when empty (useful when both repos share a password).
	Password string
}

// Copy copies snapshotID from this runner's backend into dst.
// Pass snapshotID="" to copy all snapshots.
func (r *Runner) Copy(ctx context.Context, dst Backend, snapshotID string, opts CopyOptions) error {
	args := []string{"copy", "--json", "--repo2", dst.RepoURL()}
	if snapshotID != "" {
		args = append(args, snapshotID)
	}

	// The second repo credentials are injected via environment variables; there
	// are no --cacert2 / --password2 CLI flags in restic.
	pw2 := opts.Password
	if pw2 == "" {
		pw2 = r.password
	}
	extra := []string{"RESTIC_PASSWORD2=" + pw2}
	if dst.CACert != "" {
		extra = append(extra, "RESTIC_CACERT2="+dst.CACert)
	}
	_, err := r.runWithExtra(ctx, extra, nil, args...)
	return err
}

// run executes restic with args. stdin is optional.
func (r *Runner) run(ctx context.Context, stdin []byte, args ...string) ([]byte, error) {
	return r.runWithExtra(ctx, nil, stdin, args...)
}

// runWithExtra executes restic with args plus additional environment entries
// (which take precedence over the base environment).
func (r *Runner) runWithExtra(ctx context.Context, extraEnv []string, stdin []byte, args ...string) ([]byte, error) {
	path, err := exec.LookPath(r.binary)
	if err != nil {
		return nil, ErrBinaryMissing
	}

	env := r.buildEnv()
	env = append(env, extraEnv...)

	cmd := exec.CommandContext(ctx, path, args...) //nolint:gosec // path resolved by LookPath
	cmd.Env = env
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, r.classifyError(err, stderr.String())
	}
	return stdout.Bytes(), nil
}

// buildEnv constructs the restic environment by inheriting the current
// process environment and overriding/adding the RESTIC_* variables.
// Inheriting is required so restic can resolve TLS roots, proxy settings, etc.
func (r *Runner) buildEnv() []string {
	// Start with the inherited environment, stripping any existing RESTIC_* vars
	// so our values win unambiguously.
	base := os.Environ()
	out := make([]string, 0, len(base)+3)
	for _, kv := range base {
		if !strings.HasPrefix(kv, "RESTIC_") {
			out = append(out, kv)
		}
	}
	out = append(out,
		"RESTIC_REPOSITORY="+r.backend.RepoURL(),
		"RESTIC_PASSWORD="+r.password, // password isolated to env, never logged
	)
	if r.backend.CACert != "" {
		out = append(out, "RESTIC_CACERT="+r.backend.CACert)
	}
	return out
}

// classifyError maps restic's exit codes and stderr text to sentinel errors.
func (r *Runner) classifyError(runErr error, stderr string) error {
	lower := strings.ToLower(stderr)
	if strings.Contains(lower, "wrong password") ||
		strings.Contains(lower, "no key found") {
		return fmt.Errorf("%w: %s", ErrWrongPassword, strings.TrimSpace(stderr))
	}
	if strings.Contains(lower, "is not a repository") ||
		strings.Contains(lower, "unable to open config") {
		return fmt.Errorf("%w: %s", ErrRepoNotFound, strings.TrimSpace(stderr))
	}
	return fmt.Errorf("restic: %w: %s", runErr, strings.TrimSpace(stderr))
}

// parseLsOutput parses the streaming JSON from `restic ls --json`.
// The first line is the snapshot object; subsequent lines are file entries.
func parseLsOutput(out []byte) ([]LsEntry, error) {
	scanner := bufio.NewScanner(bytes.NewReader(out))
	first := true
	var entries []LsEntry
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		if first {
			first = false
			continue // skip snapshot header line
		}
		var e LsEntry
		if err := json.Unmarshal(line, &e); err != nil {
			continue
		}
		entries = append(entries, e)
	}
	return entries, nil
}

// parseFindOutput parses the JSON array returned by `restic find --json`.
func parseFindOutput(out []byte) ([]FindHit, error) {
	type findGroup struct {
		Snapshot string `json:"snapshot"`
		Hits     []struct {
			Path  string    `json:"path"`
			Type  string    `json:"type"`
			Mtime time.Time `json:"mtime"`
		} `json:"hits"`
	}
	var groups []findGroup
	if err := json.Unmarshal(out, &groups); err != nil {
		return nil, fmt.Errorf("restic: parse find: %w", err)
	}
	var hits []FindHit
	for _, g := range groups {
		for _, h := range g.Hits {
			hits = append(hits, FindHit{
				SnapshotID: g.Snapshot,
				Path:       h.Path,
				Type:       h.Type,
				Mtime:      h.Mtime,
			})
		}
	}
	return hits, nil
}

// parseDiffOutput parses the streaming JSON from `restic diff --json`.
func parseDiffOutput(out []byte) ([]DiffChange, error) {
	scanner := bufio.NewScanner(bytes.NewReader(out))
	var changes []DiffChange
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		// Each line is either a change object or a stats summary; skip stats.
		var c struct {
			MessageType string `json:"message_type"`
			Path        string `json:"path"`
			Modifier    string `json:"modifier"`
			StructType  string `json:"struct_type"`
		}
		if err := json.Unmarshal(line, &c); err != nil {
			continue
		}
		if c.Path == "" {
			continue // stats line
		}
		changes = append(changes, DiffChange{
			Path:     c.Path,
			Modifier: c.Modifier,
			Type:     c.StructType,
		})
	}
	return changes, nil
}

// parseBackupSummary extracts the summary message from restic's streaming JSON
// backup output (one JSON object per line; the summary is the last line with
// message_type == "summary").
func parseBackupSummary(out []byte) (*BackupSummary, error) {
	type rawMsg struct {
		Type string `json:"message_type"`
		BackupSummary
	}
	scanner := bufio.NewScanner(bytes.NewReader(out))
	var last *BackupSummary
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var m rawMsg
		if err := json.Unmarshal(line, &m); err != nil {
			continue
		}
		if m.Type == "summary" {
			s := m.BackupSummary
			last = &s
		}
	}
	if last == nil {
		return nil, errors.New("restic: no summary message in backup output")
	}
	return last, nil
}

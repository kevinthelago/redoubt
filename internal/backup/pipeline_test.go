package backup_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/gofrs/flock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kevinthelago/redoubt/internal/backup"
)

// dumpHelperCmd returns a cross-platform command that succeeds and writes
// predictable content to stdout, for use as a test dump command.
func dumpHelperCmd() []string {
	if runtime.GOOS == "windows" {
		return []string{"cmd.exe", "/c", "echo test-dump-content"}
	}
	return []string{"sh", "-c", "echo test-dump-content"}
}

// failCmd returns a cross-platform command that always fails.
func failCmd() []string {
	if runtime.GOOS == "windows" {
		// 'exit 1' inside cmd.exe always exits non-zero.
		return []string{"cmd.exe", "/c", "exit 1"}
	}
	return []string{"false"}
}

// ── Mock implementations ──────────────────────────────────────────────────────

type mockResolver struct {
	assets []backup.TrackedAsset
	err    error
}

func (m *mockResolver) Resolve(_ context.Context) ([]backup.TrackedAsset, error) {
	return m.assets, m.err
}

type sealCall struct{ src, dst string }

type mockSealer struct {
	calls []sealCall
	err   error
}

func (m *mockSealer) SealFile(_ context.Context, src, dst string) error {
	m.calls = append(m.calls, sealCall{src: src, dst: dst})
	return m.err
}

type mockRunner struct {
	checkErr      error
	backupResult  *backup.BackupStats
	backupErr     error
	backupCalls   []backup.RunOptions
	previewResult []string
	previewErr    error
}

func (m *mockRunner) Check(_ context.Context) error { return m.checkErr }

func (m *mockRunner) Backup(_ context.Context, opts backup.RunOptions) (*backup.BackupStats, error) {
	m.backupCalls = append(m.backupCalls, opts)
	return m.backupResult, m.backupErr
}

func (m *mockRunner) Preview(_ context.Context, opts backup.RunOptions) ([]string, error) {
	if m.previewErr != nil {
		return nil, m.previewErr
	}
	if m.previewResult != nil {
		return m.previewResult, nil
	}
	return opts.Sources, nil
}

// capturingLogger records every structured log call as a formatted string.
type capturingLogger struct {
	entries []string
}

func (c *capturingLogger) record(msg string, args ...any) {
	var sb strings.Builder
	sb.WriteString(msg)
	for i := 0; i+1 < len(args); i += 2 {
		fmt.Fprintf(&sb, " %v=%v", args[i], args[i+1])
	}
	c.entries = append(c.entries, sb.String())
}

func (c *capturingLogger) Info(msg string, args ...any)  { c.record(msg, args...) }
func (c *capturingLogger) Warn(msg string, args ...any)  { c.record(msg, args...) }
func (c *capturingLogger) Error(msg string, args ...any) { c.record(msg, args...) }
func (c *capturingLogger) Debug(msg string, args ...any) { c.record(msg, args...) }

type discardLogger struct{}

func (discardLogger) Info(msg string, args ...any)  {}
func (discardLogger) Warn(msg string, args ...any)  {}
func (discardLogger) Error(msg string, args ...any) {}
func (discardLogger) Debug(msg string, args ...any) {}

// ── Helpers ───────────────────────────────────────────────────────────────────

func newPipeline(
	t *testing.T,
	resolver backup.AssetResolver,
	sealer backup.Sealer,
	runner backup.Runner,
) (*backup.Pipeline, string) {
	t.Helper()
	lockPath := filepath.Join(t.TempDir(), "backup.lock")
	return backup.New(backup.Config{
		Resolver: resolver,
		Sealer:   sealer,
		Runner:   runner,
		Log:      discardLogger{},
		LockPath: lockPath,
		TempBase: t.TempDir(),
	}), lockPath
}

func happyStats() *backup.BackupStats {
	return &backup.BackupStats{
		SnapshotID: "abc123",
		FilesNew:   5,
		FilesTotal: 20,
		BytesAdded: 1024,
		BytesTotal: 4096,
		DedupRatio: 4.0,
	}
}

// ── Tests: pre-flight ─────────────────────────────────────────────────────────

func TestRun_EmptyTrackedSet(t *testing.T) {
	runner := &mockRunner{backupResult: happyStats()}
	p, _ := newPipeline(t, &mockResolver{}, &mockSealer{}, runner)

	_, err := p.Run(context.Background(), false)

	assert.ErrorIs(t, err, backup.ErrEmptyTrackedSet)
	assert.Empty(t, runner.backupCalls, "no backup should run with an empty tracked set")
}

func TestRun_ResolverError(t *testing.T) {
	resolveErr := errors.New("config read error")
	runner := &mockRunner{backupResult: happyStats()}
	p, _ := newPipeline(t, &mockResolver{err: resolveErr}, &mockSealer{}, runner)

	_, err := p.Run(context.Background(), false)

	assert.ErrorContains(t, err, "resolve tracked set")
	assert.ErrorIs(t, err, resolveErr)
	assert.Empty(t, runner.backupCalls)
}

func TestRun_VaultUnreachable(t *testing.T) {
	resolver := &mockResolver{assets: []backup.TrackedAsset{
		{ID: "src", Category: backup.CategorySource, Paths: []string{"/project"}},
	}}
	runner := &mockRunner{checkErr: errors.New("connection refused")}
	p, _ := newPipeline(t, resolver, &mockSealer{}, runner)

	_, err := p.Run(context.Background(), false)

	assert.ErrorIs(t, err, backup.ErrVaultUnreachable,
		"unreachable vault must return ErrVaultUnreachable")
	assert.Empty(t, runner.backupCalls, "backup must not run when vault is unreachable")
}

func TestRun_Locked(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "backup.lock")

	// Hold the lock externally.
	fl := flock.New(lockPath)
	locked, err := fl.TryLock()
	require.NoError(t, err)
	require.True(t, locked)
	t.Cleanup(func() { _ = fl.Unlock() })

	resolver := &mockResolver{assets: []backup.TrackedAsset{
		{ID: "src", Category: backup.CategorySource, Paths: []string{"/project"}},
	}}
	runner := &mockRunner{backupResult: happyStats()}
	p := backup.New(backup.Config{
		Resolver: resolver,
		Sealer:   &mockSealer{},
		Runner:   runner,
		Log:      discardLogger{},
		LockPath: lockPath,
		TempBase: t.TempDir(),
	})

	_, err = p.Run(context.Background(), false)

	assert.ErrorIs(t, err, backup.ErrLocked,
		"second concurrent backup must return ErrLocked")
	assert.Empty(t, runner.backupCalls)
}

// ── Tests: happy path ─────────────────────────────────────────────────────────

func TestRun_HappyPath_SourceAsset(t *testing.T) {
	resolver := &mockResolver{assets: []backup.TrackedAsset{{
		ID:       "myproject",
		Category: backup.CategorySource,
		Paths:    []string{"/home/user/project"},
		Excludes: []string{"node_modules", ".git"},
	}}}
	runner := &mockRunner{backupResult: happyStats()}
	p, _ := newPipeline(t, resolver, &mockSealer{}, runner)

	result, err := p.Run(context.Background(), false)

	require.NoError(t, err)
	assert.False(t, result.HasErrors)
	assert.Equal(t, "abc123", result.Stats.SnapshotID)
	require.Len(t, runner.backupCalls, 1)

	opts := runner.backupCalls[0]
	assert.Contains(t, opts.Sources, "/home/user/project")
	assert.Contains(t, opts.Excludes, "node_modules")
	assert.Contains(t, opts.Excludes, ".git")
}

func TestRun_HappyPath_ConfigAsset(t *testing.T) {
	resolver := &mockResolver{assets: []backup.TrackedAsset{{
		ID:       "dotfiles",
		Category: backup.CategoryConfig,
		Paths:    []string{"/home/user/.config"},
	}}}
	runner := &mockRunner{backupResult: happyStats()}
	p, _ := newPipeline(t, resolver, &mockSealer{}, runner)

	result, err := p.Run(context.Background(), false)

	require.NoError(t, err)
	assert.False(t, result.HasErrors)
	opts := runner.backupCalls[0]
	assert.Contains(t, opts.Sources, "/home/user/.config")
}

func TestRun_HappyPath_MultipleAssets(t *testing.T) {
	resolver := &mockResolver{assets: []backup.TrackedAsset{
		{ID: "src", Category: backup.CategorySource, Paths: []string{"/project"}},
		{ID: "cfg", Category: backup.CategoryConfig, Paths: []string{"/dotfiles"}},
	}}
	runner := &mockRunner{backupResult: happyStats()}
	p, _ := newPipeline(t, resolver, &mockSealer{}, runner)

	result, err := p.Run(context.Background(), false)

	require.NoError(t, err)
	assert.Len(t, result.Assets, 2)
	require.Len(t, runner.backupCalls, 1, "all assets go into a single restic backup call")

	opts := runner.backupCalls[0]
	assert.Contains(t, opts.Sources, "/project")
	assert.Contains(t, opts.Sources, "/dotfiles")
}

// ── Tests: category tags ──────────────────────────────────────────────────────

func TestRun_CategoryTags_AlwaysHasRedoubt(t *testing.T) {
	resolver := &mockResolver{assets: []backup.TrackedAsset{{
		ID: "src", Category: backup.CategorySource, Paths: []string{"/p"},
	}}}
	runner := &mockRunner{backupResult: happyStats()}
	p, _ := newPipeline(t, resolver, &mockSealer{}, runner)

	_, err := p.Run(context.Background(), false)
	require.NoError(t, err)

	opts := runner.backupCalls[0]
	assert.Contains(t, opts.Tags, "redoubt")
}

func TestRun_CategoryTags_MultipleCategories(t *testing.T) {
	resolver := &mockResolver{assets: []backup.TrackedAsset{
		{ID: "src", Category: backup.CategorySource, Paths: []string{"/p"}},
		{ID: "cfg", Category: backup.CategoryConfig, Paths: []string{"/cfg"}},
		{ID: "sec", Category: backup.CategorySecrets, Paths: []string{}},
	}}
	runner := &mockRunner{backupResult: happyStats()}
	sealer := &mockSealer{}
	p, _ := newPipeline(t, resolver, sealer, runner)

	_, err := p.Run(context.Background(), false)
	require.NoError(t, err)

	opts := runner.backupCalls[0]
	assert.Contains(t, opts.Tags, "redoubt")
	assert.Contains(t, opts.Tags, "category=source")
	assert.Contains(t, opts.Tags, "category=config")
	assert.Contains(t, opts.Tags, "category=secrets")
}

func TestRun_CategoryTags_NoDuplicates(t *testing.T) {
	resolver := &mockResolver{assets: []backup.TrackedAsset{
		{ID: "s1", Category: backup.CategorySource, Paths: []string{"/a"}},
		{ID: "s2", Category: backup.CategorySource, Paths: []string{"/b"}},
	}}
	runner := &mockRunner{backupResult: happyStats()}
	p, _ := newPipeline(t, resolver, &mockSealer{}, runner)

	_, err := p.Run(context.Background(), false)
	require.NoError(t, err)

	opts := runner.backupCalls[0]
	counts := map[string]int{}
	for _, tag := range opts.Tags {
		counts[tag]++
	}
	for tag, n := range counts {
		assert.Equal(t, 1, n, "duplicate tag %q", tag)
	}
}

// ── Tests: secrets handling ───────────────────────────────────────────────────

func TestRun_SecretsAsset_RawPathNotInSources(t *testing.T) {
	td := t.TempDir()
	secretPath := filepath.Join(td, ".env")
	require.NoError(t, os.WriteFile(secretPath, []byte("SECRET=value"), 0o600))

	resolver := &mockResolver{assets: []backup.TrackedAsset{{
		ID:       "env",
		Category: backup.CategorySecrets,
		Paths:    []string{secretPath},
	}}}
	runner := &mockRunner{backupResult: happyStats()}
	sealer := &mockSealer{}
	p, _ := newPipeline(t, resolver, sealer, runner)

	_, err := p.Run(context.Background(), false)
	require.NoError(t, err)

	require.Len(t, runner.backupCalls, 1)
	opts := runner.backupCalls[0]

	// The raw secret path must not appear in sources; the sealed temp dir does.
	assert.NotContains(t, opts.Sources, secretPath,
		"raw secret path must be replaced by the sealed temp dir")
	assert.NotEmpty(t, opts.Sources, "sealed dir must be in sources")
}

func TestRun_SecretsAsset_SealerInvoked(t *testing.T) {
	td := t.TempDir()
	secretFile := filepath.Join(td, ".env")
	require.NoError(t, os.WriteFile(secretFile, []byte("SECRET=hunter2"), 0o600))

	resolver := &mockResolver{assets: []backup.TrackedAsset{{
		ID:       "env",
		Category: backup.CategorySecrets,
		Paths:    []string{secretFile},
	}}}
	runner := &mockRunner{backupResult: happyStats()}
	sealer := &mockSealer{}
	p, _ := newPipeline(t, resolver, sealer, runner)

	result, err := p.Run(context.Background(), false)
	require.NoError(t, err)
	assert.False(t, result.HasErrors)

	require.Len(t, sealer.calls, 1, "sealer must be called once per secret file")
	call := sealer.calls[0]
	assert.Equal(t, secretFile, call.src)
	assert.True(t, strings.HasSuffix(call.dst, ".age"),
		"sealed file must have .age extension; got %s", call.dst)
}

func TestRun_NoSecretInLogs(t *testing.T) {
	td := t.TempDir()
	secretValue := "super-secret-password-NEVERLOGTHIS"
	secretFile := filepath.Join(td, ".env")
	require.NoError(t, os.WriteFile(secretFile, []byte("PASSWORD="+secretValue), 0o600))

	capLog := &capturingLogger{}
	resolver := &mockResolver{assets: []backup.TrackedAsset{{
		ID:       "env",
		Category: backup.CategorySecrets,
		Paths:    []string{secretFile},
	}}}
	runner := &mockRunner{backupResult: happyStats()}
	sealer := &mockSealer{} // mock: does not read file content

	p := backup.New(backup.Config{
		Resolver: resolver,
		Sealer:   sealer,
		Runner:   runner,
		Log:      capLog,
		LockPath: filepath.Join(td, "lock"),
		TempBase: t.TempDir(),
	})

	_, err := p.Run(context.Background(), false)
	require.NoError(t, err)

	for _, entry := range capLog.entries {
		assert.NotContains(t, entry, secretValue,
			"secret value must never appear in logs; found in: %q", entry)
	}
}

// ── Tests: database dump ──────────────────────────────────────────────────────

func TestRun_DBAsset_DumpCommandRuns(t *testing.T) {
	resolver := &mockResolver{assets: []backup.TrackedAsset{{
		ID:       "mydb",
		Category: backup.CategoryDatabase,
		DumpCmd:  dumpHelperCmd(),
	}}}
	runner := &mockRunner{backupResult: happyStats()}
	p, _ := newPipeline(t, resolver, &mockSealer{}, runner)

	result, err := p.Run(context.Background(), false)
	require.NoError(t, err)
	assert.False(t, result.HasErrors)

	// The runner must have been called with the dump file as a source.
	require.Len(t, runner.backupCalls, 1)
	opts := runner.backupCalls[0]
	require.Len(t, opts.Sources, 1, "dump file must be the only source")

	dumpPath := opts.Sources[0]
	assert.Contains(t, filepath.Base(dumpPath), "dump-mydb",
		"dump file name must identify the asset")
	// The dump file is wiped by the deferred cleanup inside Run(), so
	// reading its content here is not reliable — wipe tests cover that.
}

// ── Tests: fail-soft ──────────────────────────────────────────────────────────

func TestRun_FailSoft_DBDumpFails_OtherAssetsStillBackUp(t *testing.T) {
	resolver := &mockResolver{assets: []backup.TrackedAsset{
		// This dump will fail (command not found).
		{ID: "baddb", Category: backup.CategoryDatabase, DumpCmd: []string{"nonexistent-dump-command-xyz"}},
		// This source asset must still be backed up.
		{ID: "src", Category: backup.CategorySource, Paths: []string{"/project"}},
	}}
	runner := &mockRunner{backupResult: happyStats()}
	p, _ := newPipeline(t, resolver, &mockSealer{}, runner)

	result, err := p.Run(context.Background(), false)

	require.NoError(t, err, "fail-soft: top-level error only for fatal pre-flight failures")
	assert.True(t, result.HasErrors, "HasErrors must be true when an asset fails")

	// Find the DB asset status.
	var dbStatus backup.AssetStatus
	for _, s := range result.Assets {
		if s.Asset.ID == "baddb" {
			dbStatus = s
		}
	}
	assert.NotNil(t, dbStatus.Err, "failed DB asset must record an error")
	assert.Equal(t, "dump", dbStatus.Stage)

	// Backup ran with the source asset's path (not the DB).
	require.Len(t, runner.backupCalls, 1)
	opts := runner.backupCalls[0]
	assert.Contains(t, opts.Sources, "/project", "source asset must still be backed up")
}

func TestRun_FailSoft_SealFails_OtherAssetsStillBackUp(t *testing.T) {
	resolver := &mockResolver{assets: []backup.TrackedAsset{
		{ID: "secrets", Category: backup.CategorySecrets, Paths: []string{"/home/user/.env"}},
		{ID: "cfg", Category: backup.CategoryConfig, Paths: []string{"/etc/config"}},
	}}
	runner := &mockRunner{backupResult: happyStats()}
	sealer := &mockSealer{err: errors.New("age: recipient key not found")}
	p, _ := newPipeline(t, resolver, sealer, runner)

	result, err := p.Run(context.Background(), false)

	require.NoError(t, err, "seal failure is fail-soft")
	assert.True(t, result.HasErrors)

	var secretsStatus backup.AssetStatus
	for _, s := range result.Assets {
		if s.Asset.ID == "secrets" {
			secretsStatus = s
		}
	}
	assert.NotNil(t, secretsStatus.Err)
	assert.Equal(t, "seal", secretsStatus.Stage)

	// Config asset still backed up.
	require.Len(t, runner.backupCalls, 1)
	assert.Contains(t, runner.backupCalls[0].Sources, "/etc/config")
}

func TestRun_AllAssetsFail_RunReturnsNoTopLevelError(t *testing.T) {
	// Even if ALL assets fail, Run returns no top-level error (fail-soft throughout).
	resolver := &mockResolver{assets: []backup.TrackedAsset{
		{ID: "baddb", Category: backup.CategoryDatabase, DumpCmd: failCmd()},
	}}
	runner := &mockRunner{backupResult: happyStats()}
	p, _ := newPipeline(t, resolver, &mockSealer{}, runner)

	result, err := p.Run(context.Background(), false)

	require.NoError(t, err)
	assert.True(t, result.HasErrors)
}

// ── Tests: backup runner failure ──────────────────────────────────────────────

func TestRun_ResticBackupFails(t *testing.T) {
	resolver := &mockResolver{assets: []backup.TrackedAsset{{
		ID: "src", Category: backup.CategorySource, Paths: []string{"/p"},
	}}}
	backupErr := errors.New("restic: repository not found")
	runner := &mockRunner{backupErr: backupErr}
	p, _ := newPipeline(t, resolver, &mockSealer{}, runner)

	_, err := p.Run(context.Background(), false)

	assert.ErrorContains(t, err, "restic backup")
	assert.ErrorIs(t, err, backupErr)
}

// ── Tests: dry-run ────────────────────────────────────────────────────────────

func TestRun_DryRun_NoVaultCheck(t *testing.T) {
	resolver := &mockResolver{assets: []backup.TrackedAsset{{
		ID: "src", Category: backup.CategorySource, Paths: []string{"/project"},
	}}}
	runner := &mockRunner{checkErr: errors.New("vault is down")} // vault unreachable
	p, _ := newPipeline(t, resolver, &mockSealer{}, runner)

	result, err := p.Run(context.Background(), true)

	require.NoError(t, err, "dry-run must not check vault connectivity")
	assert.NotNil(t, result)
	assert.True(t, result.DryRun)
	assert.Empty(t, runner.backupCalls, "dry-run must not write to the vault")
}

func TestRun_DryRun_NoLockAcquired(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "backup.lock")

	// Hold the lock.
	fl := flock.New(lockPath)
	locked, err := fl.TryLock()
	require.NoError(t, err)
	require.True(t, locked)
	t.Cleanup(func() { _ = fl.Unlock() })

	resolver := &mockResolver{assets: []backup.TrackedAsset{{
		ID: "src", Category: backup.CategorySource, Paths: []string{"/project"},
	}}}
	runner := &mockRunner{backupResult: happyStats()}
	p := backup.New(backup.Config{
		Resolver: resolver, Sealer: &mockSealer{}, Runner: runner,
		Log: discardLogger{}, LockPath: lockPath, TempBase: t.TempDir(),
	})

	result, err := p.Run(context.Background(), true)

	require.NoError(t, err, "dry-run must succeed even when lock is held")
	assert.True(t, result.DryRun)
}

func TestRun_DryRun_PreviewPaths(t *testing.T) {
	resolver := &mockResolver{assets: []backup.TrackedAsset{
		{ID: "src", Category: backup.CategorySource, Paths: []string{"/project"}},
		{ID: "db", Category: backup.CategoryDatabase, DumpCmd: []string{"pg_dump", "mydb"}},
		{ID: "sec", Category: backup.CategorySecrets, Paths: []string{"/home/user/.env"}},
	}}
	previewPaths := []string{"/project/main.go", "/project/go.mod"}
	runner := &mockRunner{previewResult: previewPaths}
	p, _ := newPipeline(t, resolver, &mockSealer{}, runner)

	result, err := p.Run(context.Background(), true)

	require.NoError(t, err)
	assert.True(t, result.DryRun)
	assert.NotEmpty(t, result.PreviewPaths)
}

func TestRun_DryRun_NoDump_NoSeal(t *testing.T) {
	td := t.TempDir()
	secretFile := filepath.Join(td, ".env")
	require.NoError(t, os.WriteFile(secretFile, []byte("SECRET=value"), 0o600))

	sealer := &mockSealer{}
	resolver := &mockResolver{assets: []backup.TrackedAsset{
		{ID: "sec", Category: backup.CategorySecrets, Paths: []string{secretFile}},
		{ID: "db", Category: backup.CategoryDatabase, DumpCmd: []string{"pg_dump", "mydb"}},
	}}
	runner := &mockRunner{}
	p, _ := newPipeline(t, resolver, sealer, runner)

	_, err := p.Run(context.Background(), true)

	require.NoError(t, err)
	assert.Empty(t, sealer.calls, "dry-run must not invoke the sealer")
	assert.Empty(t, runner.backupCalls, "dry-run must not invoke the runner")
}

// ── Tests: temp dir cleanup ───────────────────────────────────────────────────

func TestRun_TempDirWipedAfterSuccess(t *testing.T) {
	tempBase := t.TempDir()
	resolver := &mockResolver{assets: []backup.TrackedAsset{{
		ID: "src", Category: backup.CategorySource, Paths: []string{"/p"},
	}}}
	runner := &mockRunner{backupResult: happyStats()}
	p := backup.New(backup.Config{
		Resolver: resolver, Sealer: &mockSealer{}, Runner: runner,
		Log: discardLogger{}, LockPath: filepath.Join(t.TempDir(), "l"), TempBase: tempBase,
	})

	_, err := p.Run(context.Background(), false)
	require.NoError(t, err)

	entries, err := os.ReadDir(tempBase)
	require.NoError(t, err)
	assert.Empty(t, entries, "temp dir must be cleaned up after a successful backup")
}

func TestRun_TempDirWipedAfterBackupFailure(t *testing.T) {
	tempBase := t.TempDir()
	resolver := &mockResolver{assets: []backup.TrackedAsset{{
		ID: "src", Category: backup.CategorySource, Paths: []string{"/p"},
	}}}
	runner := &mockRunner{backupErr: errors.New("restic error")}
	p := backup.New(backup.Config{
		Resolver: resolver, Sealer: &mockSealer{}, Runner: runner,
		Log: discardLogger{}, LockPath: filepath.Join(t.TempDir(), "l"), TempBase: tempBase,
	})

	_, err := p.Run(context.Background(), false)
	assert.Error(t, err)

	entries, _ := os.ReadDir(tempBase)
	assert.Empty(t, entries, "temp dir must be cleaned up even when the backup fails")
}

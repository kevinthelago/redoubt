package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kevinthelago/redoubt/gui/engine"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App is the Wails application struct. All exported methods are available to
// the JavaScript frontend via the generated bindings.
type App struct {
	ctx    context.Context
	runner *engine.Runner
	config AppConfig
}

// AppConfig mirrors engine.AppConfig but lives here for the GUI layer.
// It is persisted to ~/.config/redoubt-gui/config.json.
type AppConfig = engine.AppConfig

// NewApp creates the application. Called by main.go before wails.Run.
func NewApp() *App {
	return &App{
		runner: engine.NewRunner(),
	}
}

// startup is called by Wails when the application window is ready.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	// Load persisted config; ignore errors (first run).
	_ = a.loadConfig()
}

// ---------------------------------------------------------------------------
// Status
// ---------------------------------------------------------------------------

// GetStatus runs `redoubt status --json` and returns the parsed result.
func (a *App) GetStatus() (engine.StatusResult, error) {
	out, err := a.runner.Run("status", "--json")
	if err != nil {
		return engine.StatusResult{Overall: engine.GradeUnknown}, err
	}
	var result engine.StatusResult
	if err := json.Unmarshal(out, &result); err != nil {
		return engine.StatusResult{Overall: engine.GradeUnknown},
			fmt.Errorf("parse status: %w", err)
	}
	return result, nil
}

// ---------------------------------------------------------------------------
// Config
// ---------------------------------------------------------------------------

// GetConfig returns the current GUI configuration.
func (a *App) GetConfig() (engine.AppConfig, error) {
	return a.config, nil
}

// SaveConfig persists the GUI configuration.
func (a *App) SaveConfig(cfg engine.AppConfig) error {
	a.config = cfg
	return a.saveConfig()
}

// ---------------------------------------------------------------------------
// Assets
// ---------------------------------------------------------------------------

// ListAssets runs `redoubt track list --json` and returns tracked paths.
func (a *App) ListAssets() ([]engine.Asset, error) {
	out, err := a.runner.Run("track", "list", "--json")
	if err != nil {
		return nil, err
	}
	var assets []engine.Asset
	if err := json.Unmarshal(out, &assets); err != nil {
		return nil, fmt.Errorf("parse assets: %w", err)
	}
	return assets, nil
}

// AddAsset runs `redoubt track add` for the given path and category.
func (a *App) AddAsset(path, category, dumpCmd string) error {
	args := []string{"track", "add", "--path", path, "--category", category}
	if dumpCmd != "" {
		args = append(args, "--dump-command", dumpCmd)
	}
	_, err := a.runner.Run(args...)
	return err
}

// RemoveAsset removes a tracked path.
func (a *App) RemoveAsset(path string) error {
	_, err := a.runner.Run("track", "remove", "--path", path)
	return err
}

// ---------------------------------------------------------------------------
// Snapshots
// ---------------------------------------------------------------------------

// ListSnapshots runs `redoubt snapshots list --json` with optional filters.
func (a *App) ListSnapshots(filter engine.SnapshotFilter) ([]engine.Snapshot, error) {
	args := []string{"snapshots", "list", "--json"}
	if filter.Hostname != "" {
		args = append(args, "--host", filter.Hostname)
	}
	if filter.After != "" {
		args = append(args, "--after", filter.After)
	}
	if filter.Before != "" {
		args = append(args, "--before", filter.Before)
	}
	for _, tag := range filter.Tags {
		args = append(args, "--tag", tag)
	}
	if filter.Source == "cold" {
		args = append(args, "--cold")
	}

	out, err := a.runner.Run(args...)
	if err != nil {
		return nil, err
	}
	var snaps []engine.Snapshot
	if err := json.Unmarshal(out, &snaps); err != nil {
		return nil, fmt.Errorf("parse snapshots: %w", err)
	}
	return snaps, nil
}

// GetSnapshotTree returns the file tree rooted at path within a snapshot.
func (a *App) GetSnapshotTree(id, path string) ([]engine.TreeNode, error) {
	args := []string{"snapshots", "ls", id, "--json"}
	if path != "" {
		args = append(args, "--path", path)
	}
	out, err := a.runner.Run(args...)
	if err != nil {
		return nil, err
	}
	var nodes []engine.TreeNode
	if err := json.Unmarshal(out, &nodes); err != nil {
		return nil, fmt.Errorf("parse tree: %w", err)
	}
	return nodes, nil
}

// DiffSnapshots returns the changed paths between two snapshots.
func (a *App) DiffSnapshots(snapshotA, snapshotB string) ([]engine.DiffEntry, error) {
	out, err := a.runner.Run("snapshots", "diff", snapshotA, snapshotB, "--json")
	if err != nil {
		return nil, err
	}
	var entries []engine.DiffEntry
	if err := json.Unmarshal(out, &entries); err != nil {
		return nil, fmt.Errorf("parse diff: %w", err)
	}
	return entries, nil
}

// FindInSnapshots searches for a path across all snapshots.
func (a *App) FindInSnapshots(query string) ([]engine.FindResult, error) {
	out, err := a.runner.Run("snapshots", "find", query, "--json")
	if err != nil {
		return nil, err
	}
	var results []engine.FindResult
	if err := json.Unmarshal(out, &results); err != nil {
		return nil, fmt.Errorf("parse find: %w", err)
	}
	return results, nil
}

// ---------------------------------------------------------------------------
// Backup
// ---------------------------------------------------------------------------

// BackupNow runs a backup and streams progress events to the frontend via
// Wails event "backup:progress". The method returns when the backup completes
// or fails.
func (a *App) BackupNow() error {
	ctx, cancel := context.WithCancel(a.ctx)
	defer cancel()

	emit := func(p engine.BackupProgress) {
		runtime.EventsEmit(a.ctx, "backup:progress", p)
	}

	return a.runner.RunStreaming(ctx, emit, "backup", "run")
}

// ---------------------------------------------------------------------------
// Vault
// ---------------------------------------------------------------------------

// GetVaultStatus returns the current vault configuration and reachability.
func (a *App) GetVaultStatus() (engine.VaultStatus, error) {
	out, err := a.runner.Run("vault", "status", "--json")
	if err != nil {
		return engine.VaultStatus{}, err
	}
	var vs engine.VaultStatus
	if err := json.Unmarshal(out, &vs); err != nil {
		return engine.VaultStatus{}, fmt.Errorf("parse vault status: %w", err)
	}
	return vs, nil
}

// SetupVault configures the remote vault address and repository path.
func (a *App) SetupVault(addr, repoPath string) error {
	_, err := a.runner.Run("vault", "init", "--addr", addr, "--repo", repoPath)
	return err
}

// ---------------------------------------------------------------------------
// Keys
// ---------------------------------------------------------------------------

// GetKeyStatus returns the encryption key and escrow state.
func (a *App) GetKeyStatus() (engine.KeyStatus, error) {
	out, err := a.runner.Run("key", "status", "--json")
	if err != nil {
		return engine.KeyStatus{}, err
	}
	var ks engine.KeyStatus
	if err := json.Unmarshal(out, &ks); err != nil {
		return engine.KeyStatus{}, fmt.Errorf("parse key status: %w", err)
	}
	return ks, nil
}

// ---------------------------------------------------------------------------
// Cold drives
// ---------------------------------------------------------------------------

// ListDrives returns the registry of cold-copy drives.
func (a *App) ListDrives() ([]engine.DriveRecord, error) {
	out, err := a.runner.Run("cold", "list", "--json")
	if err != nil {
		return nil, err
	}
	var drives []engine.DriveRecord
	if err := json.Unmarshal(out, &drives); err != nil {
		return nil, fmt.Errorf("parse drives: %w", err)
	}
	return drives, nil
}

// MakeColdCopy initiates a cold copy to the drive with the given label.
func (a *App) MakeColdCopy(label string) error {
	_, err := a.runner.Run("cold", "copy", "--label", label)
	return err
}

// ---------------------------------------------------------------------------
// Drills
// ---------------------------------------------------------------------------

// GetDrillHistory returns the restore-drill history.
func (a *App) GetDrillHistory() ([]engine.DrillResult, error) {
	out, err := a.runner.Run("drill", "history", "--json")
	if err != nil {
		return nil, err
	}
	var results []engine.DrillResult
	if err := json.Unmarshal(out, &results); err != nil {
		return nil, fmt.Errorf("parse drill history: %w", err)
	}
	return results, nil
}

// RunDrill runs a restore drill and returns when it completes.
func (a *App) RunDrill() error {
	_, err := a.runner.Run("drill", "run")
	return err
}

// ---------------------------------------------------------------------------
// Theme
// ---------------------------------------------------------------------------

// GetTheme returns the persisted theme preference ("dark" or "light").
func (a *App) GetTheme() string {
	if a.config.Theme == "" {
		return "dark"
	}
	return a.config.Theme
}

// SetTheme persists the theme preference.
func (a *App) SetTheme(theme string) error {
	a.config.Theme = theme
	return a.saveConfig()
}

// ---------------------------------------------------------------------------
// Config persistence
// ---------------------------------------------------------------------------

func configPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "redoubt-gui", "config.json"), nil
}

func (a *App) loadConfig() error {
	p, err := configPath()
	if err != nil {
		return err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return err // File may not exist on first run.
	}
	return json.Unmarshal(data, &a.config)
}

func (a *App) saveConfig() error {
	p, err := configPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(a.config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o600)
}

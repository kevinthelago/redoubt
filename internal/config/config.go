// Package config manages Redoubt's persistent configuration and tracked-set store.
// All state is kept in a single TOML file under the OS-specific config directory.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// VaultProfile holds the connection parameters for the restic REST backend.
// The repository password is never persisted here; it is fetched from the OS
// secret store at runtime and passed via environment variable.
type VaultProfile struct {
	URL        string `toml:"url"`
	CACert     string `toml:"ca_cert"`    // path to PEM file for pinned CA
	Repository string `toml:"repository"` // restic REST repo path segment
}

// Schedule defines when automated backups run.
type Schedule struct {
	// Cron is a standard five-field cron expression (minute hour dom month dow).
	Cron    string `toml:"cron"`
	Catchup bool   `toml:"catchup"` // run a missed backup on daemon restart
}

// Retention maps to restic's forget policy flags.
type Retention struct {
	Hourly  int `toml:"hourly"`
	Daily   int `toml:"daily"`
	Weekly  int `toml:"weekly"`
	Monthly int `toml:"monthly"`
}

// Thresholds defines the health-check alert boundaries.
type Thresholds struct {
	MaxHoursWithoutBackup int     `toml:"max_hours_without_backup"`
	MaxRepoSizeGB         float64 `toml:"max_repo_size_gb"`
}

// TrackedSet is a named collection of filesystem paths to back up together.
type TrackedSet struct {
	Name     string   `toml:"name"`
	Paths    []string `toml:"paths"`
	Tags     []string `toml:"tags,omitempty"`
	Excludes []string `toml:"excludes,omitempty"`
}

// Config is the root Redoubt configuration that persists to disk.
type Config struct {
	Vault       VaultProfile `toml:"vault"`
	Schedule    Schedule     `toml:"schedule"`
	Retention   Retention    `toml:"retention"`
	Thresholds  Thresholds   `toml:"thresholds"`
	TrackedSets []TrackedSet `toml:"tracked_set"`
}

// DefaultConfig returns a Config populated with conservative defaults.
func DefaultConfig() *Config {
	return &Config{
		Schedule: Schedule{
			Cron:    "0 * * * *", // every hour
			Catchup: true,
		},
		Retention: Retention{
			Hourly:  24,
			Daily:   7,
			Weekly:  4,
			Monthly: 6,
		},
		Thresholds: Thresholds{
			MaxHoursWithoutBackup: 25,
			MaxRepoSizeGB:         100,
		},
	}
}

// Load reads config from the default platform path. Returns DefaultConfig when
// the file is absent (first-run case).
func Load() (*Config, error) {
	return LoadFrom(ConfigPath())
}

// LoadFrom reads config from path. Returns DefaultConfig when path does not
// exist. Returns an error if the file exists but cannot be decoded.
func LoadFrom(path string) (*Config, error) {
	cfg := DefaultConfig()
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if _, err := toml.DecodeFile(path, cfg); err != nil {
		return nil, fmt.Errorf("config: decode %q: %w", path, err)
	}
	return cfg, nil
}

// Save writes cfg to the default platform config path, creating parent
// directories as needed with restricted permissions (0o700).
func Save(cfg *Config) error {
	return SaveTo(cfg, ConfigPath())
}

// SaveTo writes cfg to path, creating parent directories as needed.
func SaveTo(cfg *Config, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("config: mkdir %q: %w", filepath.Dir(path), err)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("config: open %q: %w", path, err)
	}
	defer f.Close()
	if err := toml.NewEncoder(f).Encode(cfg); err != nil {
		return fmt.Errorf("config: encode: %w", err)
	}
	return nil
}

// AddTrackedSet appends ts to the config. Returns an error if a set with the
// same name already exists.
func (c *Config) AddTrackedSet(ts TrackedSet) error {
	if _, ok := c.GetTrackedSet(ts.Name); ok {
		return fmt.Errorf("config: tracked set %q already exists", ts.Name)
	}
	c.TrackedSets = append(c.TrackedSets, ts)
	return nil
}

// RemoveTrackedSet removes the tracked set with name and returns whether it
// was found.
func (c *Config) RemoveTrackedSet(name string) bool {
	for i, ts := range c.TrackedSets {
		if ts.Name == name {
			c.TrackedSets = append(c.TrackedSets[:i], c.TrackedSets[i+1:]...)
			return true
		}
	}
	return false
}

// GetTrackedSet returns the tracked set with name, or false if not found.
func (c *Config) GetTrackedSet(name string) (*TrackedSet, bool) {
	for i := range c.TrackedSets {
		if c.TrackedSets[i].Name == name {
			return &c.TrackedSets[i], true
		}
	}
	return nil, false
}

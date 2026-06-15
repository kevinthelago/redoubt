// Package vault manages the append-only, LAN-only restic REST-server vault.
//
// The vault is the storage layer of Redoubt: a restic repository served over
// HTTPS via rest-server in append-only mode, bound to a private RFC1918 address.
// No WAN listener is ever created; ValidateLANAddress enforces this on every
// address before any bind or restic operation.
//
// Consumers (back-up-now, browse-snapshots, restore, etc.) connect using the
// Profile returned by Init and persisted in the main config. They never call Init
// themselves — only the set-up-vault stream does.
package vault

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// vaultConfig is the persisted vault runtime state, stored at <vaultBase>/vault.json.
type vaultConfig struct {
	ListenAddr    string `json:"listen_addr"`
	RepoDir       string `json:"repo_dir"`
	TLSDir        string `json:"tls_dir"`
	RestServerBin string `json:"rest_server_bin"`
}

const vaultConfigFile = "vault.json"

// Status reports live vault health. Obtained via GetStatus; never cached.
type Status struct {
	// Reachable indicates the vault REST endpoint responded within the 2-second timeout.
	Reachable bool
	// AppendOnly is always true for Redoubt vaults (enforced by service args).
	AppendOnly bool
	// FreeBytes is available disk space on the volume hosting the repository.
	FreeBytes int64
	// TotalBytes is the total capacity of that volume.
	TotalBytes int64
	// RepoInited reports whether the restic repository has been initialized.
	RepoInited bool
	// ListenAddr is the RFC1918 address the vault is bound to.
	ListenAddr string
}

// Init provisions a new vault:
//   - Validates address is a private RFC1918 LAN address (no WAN / no loopback)
//   - Generates a self-signed TLS CA and server certificate
//   - Runs `restic init` to initialize an append-only repository at repoPath
//   - Installs and starts a persistent OS service running rest-server in append-only mode
//   - Returns a Profile with the listen address and pinned CA fingerprint
//
// vaultBase is the directory where vault config, TLS certs, and profile.json are stored
// (e.g. /srv/redoubt). The restic repository lives at repoPath (e.g. /srv/redoubt/repo).
//
// Idempotent: if a profile already exists at <vaultBase>/profile.json, Init returns the
// existing Profile alongside ErrAlreadyInited (non-fatal — callers may proceed).
//
// resticPassword is the repository password from keystore.KeyMaterial.ResticPassword().
// The keystore must be initialized and unlocked before calling Init.
func Init(address, repoPath, resticPassword string) (*Profile, error) {
	return InitWithBase(address, repoPath, filepath.Dir(filepath.Clean(repoPath)), resticPassword)
}

// InitWithBase is like Init but lets the caller specify vaultBase explicitly.
// Used in integration tests where vaultBase and repoPath are in a temp directory.
func InitWithBase(address, repoPath, vaultBase, resticPassword string) (*Profile, error) {
	// Enforce no-WAN guarantee before touching the filesystem.
	if err := ValidateLANAddress(address); err != nil {
		return nil, err
	}

	// Idempotent: return the existing profile if already initialized.
	if profile, err := ReadProfile(filepath.Join(vaultBase, "profile.json")); err == nil {
		return profile, ErrAlreadyInited
	}

	// Verify repoPath is writable.
	if err := ensureWritable(repoPath); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrPathNotWritable, err)
	}

	// Check port is not already in use.
	if err := checkPortAvailable(address); err != nil {
		return nil, ErrPortInUse
	}

	// Locate required binaries before generating certs.
	resticBin, err := exec.LookPath("restic")
	if err != nil {
		return nil, ErrResticNotFound
	}
	restServerBin, err := lookupRestServer()
	if err != nil {
		return nil, err
	}

	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("parse address: %w", err)
	}

	tlsDir := filepath.Join(vaultBase, "tls")
	if err := os.MkdirAll(vaultBase, 0700); err != nil {
		return nil, fmt.Errorf("create vault base directory: %w", err)
	}

	// Generate TLS CA and server certificate.
	ca, err := GenerateCACert()
	if err != nil {
		return nil, fmt.Errorf("generate CA: %w", err)
	}
	serverCert, err := ca.GenerateServerCert(host)
	if err != nil {
		return nil, fmt.Errorf("generate server certificate: %w", err)
	}
	if err := ca.WriteTo(tlsDir); err != nil {
		return nil, fmt.Errorf("write TLS CA: %w", err)
	}
	if err := serverCert.WriteTo(tlsDir); err != nil {
		return nil, fmt.Errorf("write TLS server certificate: %w", err)
	}

	// Initialize the restic repository at repoPath.
	if err := initResticRepo(resticBin, repoPath, resticPassword); err != nil {
		return nil, err
	}

	// Persist vault config.
	cfg := vaultConfig{
		ListenAddr:    address,
		RepoDir:       repoPath,
		TLSDir:        tlsDir,
		RestServerBin: restServerBin,
	}
	if err := writeVaultConfig(filepath.Join(vaultBase, vaultConfigFile), cfg); err != nil {
		return nil, fmt.Errorf("write vault config: %w", err)
	}

	// Write connection profile (written before service install so it's available
	// even if service registration fails).
	profile := &Profile{
		Address:       address,
		CAFingerprint: ca.Fingerprint(),
		RepoPath:      repoPath,
	}
	if err := WriteProfile(filepath.Join(vaultBase, "profile.json"), *profile); err != nil {
		return nil, fmt.Errorf("write connection profile: %w", err)
	}

	// Install and start the OS service.
	svcCfg := ServiceConfig{
		ListenAddr:    address,
		RepoDir:       repoPath,
		TLSCertPath:   filepath.Join(tlsDir, "server.crt"),
		TLSKeyPath:    filepath.Join(tlsDir, "server.key"),
		RestServerBin: restServerBin,
	}
	if err := InstallService(svcCfg, vaultBase); err != nil {
		return nil, fmt.Errorf("install vault service: %w", err)
	}
	if err := StartService(); err != nil {
		return nil, fmt.Errorf("start vault service: %w", err)
	}

	return profile, nil
}

// GetStatus probes the vault for live health. Timeout: 2 seconds.
// Returns a Status where Reachable=false (rather than an error) when the vault
// is offline. Returns an error only for configuration problems (missing profile).
func GetStatus(p *Profile) (*Status, error) {
	status := &Status{
		ListenAddr: p.Address,
		AppendOnly: true, // always; enforced by service args (see buildServerArgs)
	}

	vaultBase := filepath.Dir(p.RepoPath)
	cfgPath := filepath.Join(vaultBase, vaultConfigFile)
	if cfg, err := loadVaultConfig(cfgPath); err == nil {
		status.FreeBytes, status.TotalBytes, _ = diskFreeBytes(cfg.RepoDir)
		status.RepoInited = isResticRepoInited(cfg.RepoDir)
	}

	status.Reachable = probeVault(p)
	return status, nil
}

// probeVault issues a GET to the vault REST endpoint with the pinned CA cert.
// Returns false without error on any connectivity failure.
func probeVault(p *Profile) bool {
	vaultBase := filepath.Dir(p.RepoPath)
	caPEM, err := os.ReadFile(filepath.Join(vaultBase, "tls", "ca.crt"))
	if err != nil {
		return false
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return false
	}
	client := &http.Client{
		Timeout: 2 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{RootCAs: pool},
		},
	}
	resp, err := client.Get("https://" + p.Address + "/")
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return true
}

// initResticRepo runs `restic init --repo path` with the given password.
func initResticRepo(resticBin, repoPath, password string) error {
	if err := os.MkdirAll(repoPath, 0700); err != nil {
		return fmt.Errorf("create repo directory: %w", err)
	}
	cmd := exec.Command(resticBin, "init", "--repo", repoPath)
	cmd.Env = append(filterEnv(os.Environ(), "RESTIC_PASSWORD"), "RESTIC_PASSWORD="+password)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("restic init: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// isResticRepoInited reports whether the restic repository has been initialized.
// A restic repo always contains a "config" file at its root after init.
func isResticRepoInited(repoDir string) bool {
	_, err := os.Stat(filepath.Join(repoDir, "config"))
	return err == nil
}

func writeVaultConfig(path string, cfg vaultConfig) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func loadVaultConfig(path string) (vaultConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return vaultConfig{}, err
	}
	var cfg vaultConfig
	return cfg, json.Unmarshal(data, &cfg)
}

func ensureWritable(path string) error {
	if err := os.MkdirAll(path, 0700); err != nil {
		return err
	}
	probe := filepath.Join(path, ".redoubt-write-probe")
	if err := os.WriteFile(probe, []byte{}, 0600); err != nil {
		return err
	}
	return os.Remove(probe)
}

// checkPortAvailable returns ErrPortInUse if something is already listening at addr.
// Uses a dial probe rather than a bind attempt so it works whether or not the
// address is assigned to a local interface.
func checkPortAvailable(addr string) error {
	conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
	if err != nil {
		return nil // nothing listening → port is available
	}
	conn.Close()
	return fmt.Errorf("%w: something is already listening at %s", ErrPortInUse, addr)
}

// filterEnv removes all occurrences of key=... from env.
func filterEnv(env []string, key string) []string {
	prefix := key + "="
	result := make([]string, 0, len(env))
	for _, e := range env {
		if !strings.HasPrefix(e, prefix) {
			result = append(result, e)
		}
	}
	return result
}

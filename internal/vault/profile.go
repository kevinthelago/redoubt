package vault

import (
	"encoding/json"
	"fmt"
	"os"
)

// Profile is the connection profile written by vault init and read by all downstream consumers.
// It is persisted in the main config (Config.VaultProfile) via internal/config.
type Profile struct {
	// Address is the RFC1918 LAN host:port the vault listens on, e.g. "192.168.1.10:8000".
	Address string `json:"address"`
	// CAFingerprint is the SHA-256 hex fingerprint of the self-signed CA cert,
	// used for certificate pinning by all restic clients connecting to this vault.
	CAFingerprint string `json:"ca_fingerprint"`
	// RepoPath is the filesystem path of the restic repository on the vault device.
	RepoPath string `json:"repo_path"`
}

// RepoURL returns the restic REST backend URL for this vault, e.g.:
//
//	rest:https://192.168.1.10:8000/
//
// Pass this directly to the restic client as the repository URL.
func (p *Profile) RepoURL() string {
	return "rest:https://" + p.Address + "/"
}

// WriteProfile persists the connection profile to path as JSON (mode 0600).
func WriteProfile(path string, p Profile) error {
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal profile: %w", err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("write profile %s: %w", path, err)
	}
	return nil
}

// ReadProfile loads a connection profile from path.
func ReadProfile(path string) (*Profile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read profile %s: %w", path, err)
	}
	var p Profile
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("parse profile %s: %w", path, err)
	}
	if p.Address == "" || p.CAFingerprint == "" {
		return nil, fmt.Errorf("profile %s is incomplete (missing address or CA fingerprint)", path)
	}
	return &p, nil
}

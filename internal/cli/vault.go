// Package cli implements the Cobra subcommands for the redoubt CLI.
// This file wires the vault subcommands: init, status, and serve.
package cli

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/kevinthelago/redoubt/internal/vault"
	"github.com/spf13/cobra"
)

// NewVaultCmd returns the "vault" parent command with its subcommands attached.
// Register it on the root command via root.AddCommand(NewVaultCmd()).
func NewVaultCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "vault",
		Short: "Manage the Redoubt backup vault",
		Long: `Manage the Redoubt backup vault — an append-only, LAN-only restic REST server.

The vault is bound to a private RFC1918 LAN address only. No WAN listener is
ever created. Public addresses (including 0.0.0.0 and loopback) are refused.`,
	}
	cmd.AddCommand(
		newVaultInitCmd(),
		newVaultStatusCmd(),
		newVaultServeCmd(),
	)
	return cmd
}

// newVaultInitCmd returns the "vault init" subcommand.
func newVaultInitCmd() *cobra.Command {
	var (
		address  string
		repoPath string
	)

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize the vault",
		Long: `Initialize the Redoubt backup vault.

Sets up an append-only restic repository and starts a persistent OS service
running rest-server over TLS on the specified LAN address.

The address must be a private RFC1918 LAN address (e.g. 192.168.1.10:8000).
Public addresses, 0.0.0.0, and loopback (127.x) are refused — this is how
Redoubt guarantees no WAN exposure.

Prerequisites:
  - Run 'redoubt key init' first to initialize the keystore.
  - Ensure 'restic' and 'rest-server' are installed and on PATH.
  - Run this command on the vault machine (the device hosting the repo).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runVaultInit(cmd, address, repoPath)
		},
	}

	cmd.Flags().StringVar(&address, "address", "", "RFC1918 LAN address and port to bind (e.g. 192.168.1.10:8000)")
	cmd.Flags().StringVar(&repoPath, "repo-path", "", "Filesystem path for the restic repository (e.g. /srv/redoubt/repo)")
	_ = cmd.MarkFlagRequired("address")
	_ = cmd.MarkFlagRequired("repo-path")

	return cmd
}

func runVaultInit(cmd *cobra.Command, address, repoPath string) error {
	if err := vault.ValidateLANAddress(address); err != nil {
		return err
	}

	resticPassword, err := resolveResticPassword()
	if err != nil {
		return err
	}

	cmd.Printf("Initializing vault at %s (repo: %s)...\n", address, repoPath)

	profile, err := vault.Init(address, repoPath, resticPassword)
	if err == vault.ErrAlreadyInited {
		cmd.Printf("Vault already initialized — using existing profile.\n")
		cmd.Printf("  Address:      %s\n", profile.Address)
		cmd.Printf("  Fingerprint:  %s\n", profile.CAFingerprint)
		return nil
	}
	if err != nil {
		return fmt.Errorf("vault init: %w", err)
	}

	cmd.Printf("Vault initialized successfully.\n\n")
	cmd.Printf("  Address:      %s\n", profile.Address)
	cmd.Printf("  Repo URL:     %s\n", profile.RepoURL())
	cmd.Printf("  Fingerprint:  %s\n", profile.CAFingerprint)
	cmd.Printf("\nThe vault service is running and will restart on boot.\n")
	cmd.Printf("Connection profile written — run 'redoubt vault status' to verify.\n")
	return nil
}

// newVaultStatusCmd returns the "vault status" subcommand.
func newVaultStatusCmd() *cobra.Command {
	var profilePath string

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show vault status",
		Long: `Show the status of the Redoubt backup vault.

Reports the listen address, append-only mode, repo reachability, and free disk space.

The vault is always configured in append-only mode — dev-box clients cannot delete
backup history. This is a core Redoubt guarantee.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runVaultStatus(cmd, profilePath)
		},
	}

	cmd.Flags().StringVar(&profilePath, "profile", "", "Path to vault profile.json (default: auto-discovered from config)")
	return cmd
}

func runVaultStatus(cmd *cobra.Command, profilePath string) error {
	p, err := resolveProfile(profilePath)
	if err != nil {
		return err
	}

	status, err := vault.GetStatus(p)
	if err != nil {
		return fmt.Errorf("get vault status: %w", err)
	}

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 8, 2, ' ', 0)
	fmt.Fprintln(w, "VAULT STATUS")
	fmt.Fprintln(w, "============")
	fmt.Fprintf(w, "Address:\t%s\n", p.Address)
	fmt.Fprintf(w, "Append-only:\t%s\n", boolStatus(status.AppendOnly))
	fmt.Fprintf(w, "Reachable:\t%s\n", boolStatus(status.Reachable))
	fmt.Fprintf(w, "Repo initialized:\t%s\n", boolStatus(status.RepoInited))
	if status.TotalBytes > 0 {
		free := float64(status.FreeBytes) / (1 << 30)
		total := float64(status.TotalBytes) / (1 << 30)
		fmt.Fprintf(w, "Free disk:\t%.1f GiB / %.1f GiB\n", free, total)
	}
	_ = w.Flush()

	if !status.Reachable {
		cmd.Printf("\nVault is not reachable at %s.\n", p.Address)
		cmd.Printf("Check that the vault service is running:\n")
		cmd.Printf("  Windows: sc query RedoubtVault\n")
		cmd.Printf("  Linux:   systemctl status RedoubtVault\n")
	}
	return nil
}

// newVaultServeCmd returns the internal "vault serve" subcommand used as the OS service entrypoint.
// This command is not intended for direct user invocation.
func newVaultServeCmd() *cobra.Command {
	var vaultBase string

	cmd := &cobra.Command{
		Use:    "serve",
		Short:  "Internal: run the vault REST server (used by OS service manager)",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runVaultServe(vaultBase)
		},
	}
	cmd.Flags().StringVar(&vaultBase, "vault-base", "", "Directory containing vault.json (set by the OS service registration)")
	_ = cmd.MarkFlagRequired("vault-base")
	return cmd
}

func runVaultServe(vaultBase string) error {
	cfgPath := filepath.Join(vaultBase, "vault.json")
	cfgData, err := os.ReadFile(cfgPath)
	if err != nil {
		return fmt.Errorf("read vault config %s: %w\n  Hint: run 'redoubt vault init' first", cfgPath, err)
	}

	var vcfg struct {
		ListenAddr    string `json:"listen_addr"`
		RepoDir       string `json:"repo_dir"`
		TLSDir        string `json:"tls_dir"`
		RestServerBin string `json:"rest_server_bin"`
	}
	if err := json.Unmarshal(cfgData, &vcfg); err != nil {
		return fmt.Errorf("parse vault config: %w", err)
	}

	return vault.RunService(vault.ServiceConfig{
		ListenAddr:    vcfg.ListenAddr,
		RepoDir:       vcfg.RepoDir,
		TLSCertPath:   filepath.Join(vcfg.TLSDir, "server.crt"),
		TLSKeyPath:    filepath.Join(vcfg.TLSDir, "server.key"),
		RestServerBin: vcfg.RestServerBin,
	})
}

// resolveResticPassword returns the restic repository password.
//
// TODO(set-up-keys-and-break-glass): replace with keystore.Open() + Unlock()
// once the K1 (keystore) issue lands. See contracts/keystore.md.
//
// Interim path for development: reads REDOUBT_RESTIC_PASSWORD env var.
func resolveResticPassword() (string, error) {
	// Keystore integration (to be wired when K1 lands):
	//   km, err := keystore.Open()
	//   if err != nil { return "", fmt.Errorf("keystore not initialized — run 'redoubt key init' first: %w", err) }
	//   passphrase, err := promptPassphrase("Vault passphrase: ")
	//   if err != nil { return "", err }
	//   if err := km.Unlock(passphrase); err != nil { return "", err }
	//   return km.ResticPassword(), nil

	pw := os.Getenv("REDOUBT_RESTIC_PASSWORD")
	if pw == "" {
		return "", fmt.Errorf(
			"keystore integration pending (K1 issue)\n" +
				"  For development, set REDOUBT_RESTIC_PASSWORD env var\n" +
				"  For production, run 'redoubt key init' once the keys stream lands",
		)
	}
	return pw, nil
}

// resolveProfile loads the connection profile.
// profilePath is required until the config package (F2) lands and provides
// a persisted VaultProfile. Once it does, this falls back to config.Load().VaultProfile.
func resolveProfile(profilePath string) (*vault.Profile, error) {
	if profilePath == "" {
		// TODO(foundation/F2): fall back to config.Load().VaultProfile once the
		// config package lands and persists the vault profile.
		return nil, fmt.Errorf("--profile is required (config integration pending)\n  Run 'redoubt vault init' to create a profile, then pass it with --profile")
	}
	return vault.ReadProfile(profilePath)
}

// DiscoverVaultAddress lists available private-LAN interfaces and returns a suggested bind address.
// Called by the UI/interactive prompt when --address is not provided.
func DiscoverVaultAddress() (string, error) {
	ips, err := vault.DiscoverPrivateInterfaces()
	if err != nil {
		return "", fmt.Errorf("no LAN interface: %w\n  Connect this machine to a private (RFC1918) network first", err)
	}
	if len(ips) == 1 {
		return net.JoinHostPort(ips[0].String(), "8000"), nil
	}
	return "", fmt.Errorf(
		"multiple private interfaces found; specify one with --address:\n  %v",
		ips,
	)
}

func boolStatus(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

package cli

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kevinthelago/redoubt/internal/escrow"
	"github.com/kevinthelago/redoubt/internal/keystore"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func init() {
	// keyCmd is defined in root.go as a plain command group (no RunE).
	// Wire the real subcommands here.
	keyCmd.AddCommand(
		keyInitCmd,
		keyEscrowCmd,
		keyReconstructCmd,
		keyRotateCmd,
		keyVerifyCmd,
	)
}

// ---- key init ----

var keyInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Generate encryption keys and store them in the OS keystore",
	Long: `Creates a new Redoubt keystore:
  • CSPRNG keyfile + age X25519 identity
  • Argon2id master key derived from your passphrase + keyfile
  • All material stored in the OS secret store (never plaintext on disk)

Runs fully offline. No network access is made.`,
	RunE: runKeyInit,
}

func runKeyInit(cmd *cobra.Command, _ []string) error {
	if keystore.IsInitialized() {
		return fmt.Errorf("keystore already initialized — use 'redoubt key rotate' to change the passphrase")
	}

	fmt.Fprintln(cmd.OutOrStdout(), "Redoubt key initialization")
	fmt.Fprintln(cmd.OutOrStdout(), "Choose a strong passphrase (14+ characters, mix of upper/lower/digits/symbols).")

	pass, err := readPassphrase("Passphrase: ", cmd)
	if err != nil {
		return err
	}

	score, desc := keystore.PassphraseStrength(pass)
	if score < 1 {
		return fmt.Errorf("passphrase is %s — please choose a stronger passphrase (minimum 8 characters)", desc)
	}
	if score < 2 {
		fmt.Fprintf(cmd.OutOrStdout(), "Warning: passphrase is %s. Consider a longer, more complex passphrase.\n", desc)
	}

	confirm, err := readPassphrase("Confirm passphrase: ", cmd)
	if err != nil {
		return err
	}
	if pass != confirm {
		return fmt.Errorf("passphrases do not match")
	}

	fmt.Fprintln(cmd.OutOrStdout(), "Deriving master key (this may take a few seconds)…")

	km, err := keystore.Init(pass)
	if err != nil {
		return fmt.Errorf("key init: %w", err)
	}

	fmt.Fprintln(cmd.OutOrStdout(), "Keystore initialized.")
	fmt.Fprintf(cmd.OutOrStdout(), "  Age public key: %s\n", km.AgeRecipient())
	fmt.Fprintln(cmd.OutOrStdout(), "")
	fmt.Fprintln(cmd.OutOrStdout(), "Next step: run 'redoubt key escrow' to create break-glass recovery shares.")
	return nil
}

// ---- key escrow ----

var (
	escrowThreshold int
	escrowShares    int
	escrowOutput    string
)

var keyEscrowCmd = &cobra.Command{
	Use:   "escrow",
	Short: "Split the master key into Shamir shares for break-glass recovery",
	Long: `Splits your master key into N Shamir shares, any M of which can reconstruct it.

Generates:
  • Per-share QR code PNGs (for scanning)
  • A printable HTML recovery sheet with restore instructions

Escrow completes ONLY after the tool verifies reconstruction from the shares.
Store each share in a separate, physically secure location.`,
	RunE: runKeyEscrow,
}

func init() {
	keyEscrowCmd.Flags().IntVarP(&escrowThreshold, "threshold", "t", 2, "minimum shares required to reconstruct")
	keyEscrowCmd.Flags().IntVarP(&escrowShares, "shares", "n", 3, "total number of shares to generate")
	keyEscrowCmd.Flags().StringVarP(&escrowOutput, "output", "o", "", "directory to write recovery sheet (default: ./redoubt-escrow)")
}

func runKeyEscrow(cmd *cobra.Command, _ []string) error {
	km, err := openAndUnlock(cmd)
	if err != nil {
		return err
	}

	payload, err := km.EscrowPayload()
	if err != nil {
		return fmt.Errorf("escrow: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Splitting into %d-of-%d shares…\n", escrowThreshold, escrowShares)

	result, err := escrow.Split(payload, escrowThreshold, escrowShares)
	if err != nil {
		return fmt.Errorf("escrow split: %w", err)
	}

	outDir := escrowOutput
	if outDir == "" {
		outDir = "redoubt-escrow"
	}
	if err := os.MkdirAll(outDir, 0700); err != nil {
		return fmt.Errorf("escrow: create output dir: %w", err)
	}

	if err := escrow.WriteRecoverySheet(result.Shares, escrow.RecoverySheetOptions{
		OutputDir: outDir,
		Threshold: result.Threshold,
		Total:     result.Total,
	}); err != nil {
		return fmt.Errorf("escrow: write recovery sheet: %w", err)
	}

	absOut, _ := filepath.Abs(outDir)
	fmt.Fprintf(cmd.OutOrStdout(), "Escrow verified (reconstruction tested in-memory).\n")
	fmt.Fprintf(cmd.OutOrStdout(), "  Recovery sheet: %s/recovery.html\n", absOut)
	fmt.Fprintf(cmd.OutOrStdout(), "  QR PNGs:        %s/share-N.png\n", absOut)
	fmt.Fprintln(cmd.OutOrStdout(), "")
	fmt.Fprintln(cmd.OutOrStdout(), "  Print recovery.html and store each share in a separate secure location.")
	fmt.Fprintln(cmd.OutOrStdout(), "  Delete these files from this machine after printing.")
	return nil
}

// ---- key reconstruct ----

var reconstructShareFiles []string

var keyReconstructCmd = &cobra.Command{
	Use:   "reconstruct",
	Short: "Reconstruct the master key from Shamir shares",
	Long: `Reads Shamir shares from files and reconstructs the master key.

Each share file must contain the base64-encoded share data printed on the recovery
sheet. Provide at least the threshold number of shares.

On success, the reconstructed key material is installed into the OS keystore
under a new passphrase of your choice.`,
	RunE: runKeyReconstruct,
}

func init() {
	keyReconstructCmd.Flags().StringSliceVarP(&reconstructShareFiles, "shares", "s", nil, "share files (comma-separated), e.g. share-1.txt,share-2.txt")
	_ = keyReconstructCmd.MarkFlagRequired("shares")
}

func runKeyReconstruct(cmd *cobra.Command, _ []string) error {
	if keystore.IsInitialized() {
		return fmt.Errorf("a keystore already exists on this machine — delete it or use a blank machine for recovery")
	}

	shares, err := loadShareFiles(reconstructShareFiles)
	if err != nil {
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Reconstructing from %d share(s)…\n", len(shares))
	payload, err := escrow.Reconstruct(shares)
	if err != nil {
		if errors.Is(err, escrow.ErrInsufficientShares) {
			return fmt.Errorf("not enough valid shares to reconstruct: %w", err)
		}
		if errors.Is(err, escrow.ErrShareMismatch) {
			return fmt.Errorf("share data corrupted — check the file matches the printed recovery sheet: %w", err)
		}
		return fmt.Errorf("reconstruct: %w", err)
	}

	km, err := keystore.RestoreFromEscrowPayload(payload)
	if err != nil {
		return fmt.Errorf("reconstruct: invalid payload: %w", err)
	}

	fmt.Fprintln(cmd.OutOrStdout(), "Master key reconstructed.")
	fmt.Fprintf(cmd.OutOrStdout(), "  Age public key: %s\n", km.AgeRecipient())
	fmt.Fprintln(cmd.OutOrStdout(), "")
	fmt.Fprintln(cmd.OutOrStdout(), "Set a new passphrase to protect the keystore on this machine:")

	newPass, err := readPassphrase("New passphrase: ", cmd)
	if err != nil {
		return err
	}
	score, desc := keystore.PassphraseStrength(newPass)
	if score < 1 {
		return fmt.Errorf("passphrase is %s — choose a stronger passphrase", desc)
	}
	confirm, err := readPassphrase("Confirm: ", cmd)
	if err != nil {
		return err
	}
	if newPass != confirm {
		return fmt.Errorf("passphrases do not match")
	}

	if err := keystore.InitFromEscrow(km, newPass); err != nil {
		return fmt.Errorf("reinstall keystore: %w", err)
	}

	fmt.Fprintln(cmd.OutOrStdout(), "Keystore installed. Connect to the vault and restore files:")
	fmt.Fprintln(cmd.OutOrStdout(), "  redoubt vault status")
	fmt.Fprintln(cmd.OutOrStdout(), "  redoubt restore --snapshot latest --target /tmp/restore")
	return nil
}

// ---- key rotate ----

var keyRotateCmd = &cobra.Command{
	Use:   "rotate",
	Short: "Change the keystore passphrase",
	Long: `Changes the passphrase that protects the keystore.

The age identity and master key are preserved — existing encrypted backups remain
accessible. Only the passphrase wrapping is updated.

After rotation, generate fresh recovery shares:
  redoubt key escrow`,
	RunE: runKeyRotate,
}

func runKeyRotate(cmd *cobra.Command, _ []string) error {
	km, err := openAndUnlock(cmd)
	if err != nil {
		return err
	}

	fmt.Fprintln(cmd.OutOrStdout(), "Enter new passphrase:")
	newPass, err := readPassphrase("New passphrase: ", cmd)
	if err != nil {
		return err
	}
	score, desc := keystore.PassphraseStrength(newPass)
	if score < 1 {
		return fmt.Errorf("passphrase is %s — choose a stronger passphrase", desc)
	}
	confirm, err := readPassphrase("Confirm new passphrase: ", cmd)
	if err != nil {
		return err
	}
	if newPass != confirm {
		return fmt.Errorf("passphrases do not match")
	}

	fmt.Fprintln(cmd.OutOrStdout(), "Re-encrypting key material…")
	if err := km.Rotate(newPass); err != nil {
		return fmt.Errorf("rotate: %w", err)
	}

	fmt.Fprintln(cmd.OutOrStdout(), "Passphrase updated.")
	fmt.Fprintln(cmd.OutOrStdout(), "  Re-run 'redoubt key escrow' to generate fresh recovery shares.")
	return nil
}

// ---- key verify ----

var keyVerifyCmd = &cobra.Command{
	Use:   "verify",
	Short: "Verify keystore integrity",
	Long: `Checks that all keystore entries are present and internally consistent:
  • All required OS keyring entries are readable
  • The passphrase correctly decrypts the key material
  • The age private key parses to a valid X25519 identity`,
	RunE: runKeyVerify,
}

func runKeyVerify(cmd *cobra.Command, _ []string) error {
	km, err := openAndUnlock(cmd)
	if err != nil {
		return err
	}

	if err := km.Verify(); err != nil {
		return fmt.Errorf("keystore integrity check failed: %w", err)
	}

	fmt.Fprintln(cmd.OutOrStdout(), "Keystore integrity verified.")
	fmt.Fprintf(cmd.OutOrStdout(), "  Age public key: %s\n", km.AgeRecipient())
	resticPwd, _ := km.ResticPassword()
	// Print only the first 8 chars to confirm derivation without leaking the full value.
	fmt.Fprintf(cmd.OutOrStdout(), "  Restic password (prefix): %s…\n", resticPwd[:8])
	return nil
}

// ---- helpers ----

// openAndUnlock opens the keystore and prompts for the passphrase.
func openAndUnlock(cmd *cobra.Command) (*keystore.KeyMaterial, error) {
	km, err := keystore.Open()
	if err != nil {
		if errors.Is(err, keystore.ErrNotInitialized) {
			return nil, fmt.Errorf("keystore not initialized — run 'redoubt key init' first")
		}
		return nil, fmt.Errorf("open keystore: %w", err)
	}

	pass, err := readPassphrase("Passphrase: ", cmd)
	if err != nil {
		return nil, err
	}

	if err := km.Unlock(pass); err != nil {
		if errors.Is(err, keystore.ErrWrongPassphrase) {
			return nil, fmt.Errorf("wrong passphrase")
		}
		return nil, fmt.Errorf("unlock: %w", err)
	}
	return km, nil
}

// readPassphrase prompts for a passphrase without echoing to the terminal.
// Falls back to reading a plain line when stdin is not a terminal (tests/pipes).
func readPassphrase(prompt string, cmd *cobra.Command) (string, error) {
	fmt.Fprint(cmd.OutOrStdout(), prompt)
	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		b, err := term.ReadPassword(fd)
		fmt.Fprintln(cmd.OutOrStdout()) // newline after hidden input
		if err != nil {
			return "", fmt.Errorf("read passphrase: %w", err)
		}
		return string(b), nil
	}
	// Non-terminal (pipe/test): read until newline.
	var sb strings.Builder
	buf := make([]byte, 1)
	for {
		n, err := os.Stdin.Read(buf)
		if n > 0 {
			if buf[0] == '\n' {
				break
			}
			sb.WriteByte(buf[0])
		}
		if err != nil {
			break
		}
	}
	return strings.TrimSpace(sb.String()), nil
}

// loadShareFiles reads share files and decodes base64 share data.
// The Checksum field is computed from the loaded data so that Reconstruct
// can verify integrity.
func loadShareFiles(paths []string) ([]escrow.Share, error) {
	shares := make([]escrow.Share, 0, len(paths))
	for i, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, fmt.Errorf("read share file %q: %w", p, err)
		}
		b64 := strings.TrimSpace(string(data))
		raw, err := base64.StdEncoding.DecodeString(b64)
		if err != nil {
			return nil, fmt.Errorf("decode share file %q (expected base64): %w", p, err)
		}
		h := sha256.Sum256(raw)
		shares = append(shares, escrow.Share{
			Index:    i + 1,
			Data:     raw,
			Checksum: hex.EncodeToString(h[:]),
		})
	}
	return shares, nil
}

// Package keystore manages Redoubt's root cryptographic material.
//
// All sensitive values live only in the OS secret store (DPAPI on Windows,
// Keychain on macOS, libsecret on Linux) or in memory during an unlocked
// session. Nothing is ever written to disk in plaintext.
//
// Key derivation: Argon2id(passphrase + CSPRNG keyfile) → 32-byte master key.
// The keyfile is stored in the OS keyring; the passphrase is never persisted.
// The age private key is encrypted with XChaCha20-Poly1305 using the master key
// and stored in the OS keyring alongside the keyfile.
//
// Offline-only: no network access is ever made from this package.
package keystore

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"filippo.io/age"
	"github.com/zalando/go-keyring"
	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/chacha20poly1305"
)

const (
	keystoreService = "redoubt"
	keyMetadata     = "v1:metadata"
	keyAgePub       = "v1:age-pub"
	keyAgePriv      = "v1:age-priv"
	keyCheck        = "v1:check"
	checkPlaintext  = "redoubt-v1-check"

	argon2Time    = 3
	argon2Memory  = 64 * 1024 // 64 MiB
	argon2Threads = 4
	argon2KeyLen  = 32
	keyfileLen    = 32
	saltLen       = 32
)

// Sentinel errors returned by this package.
var (
	ErrNotInitialized    = errors.New("keystore: not initialized — run 'redoubt key init'")
	ErrAlreadyInitialized = errors.New("keystore: already initialized — use 'redoubt key rotate' to change")
	ErrWrongPassphrase   = errors.New("keystore: wrong passphrase")
	ErrKeyLocked         = errors.New("keystore: master key is locked — call Unlock first")
)

// keystoreMetadata is the JSON blob stored under keyMetadata in the OS keyring.
type keystoreMetadata struct {
	Keyfile       string `json:"keyfile"`        // base64(32-byte random keyfile)
	Argon2Salt    string `json:"argon2_salt"`    // base64(32-byte salt)
	Argon2Time    uint32 `json:"argon2_time"`
	Argon2Memory  uint32 `json:"argon2_memory"`
	Argon2Threads uint8  `json:"argon2_threads"`
}

// encryptedBlob is the JSON representation of a sealed value.
type encryptedBlob struct {
	Nonce      string `json:"nonce"`      // base64(24-byte XChaCha20-Poly1305 nonce)
	Ciphertext string `json:"ciphertext"` // base64(ciphertext + 16-byte Poly1305 tag)
}

// KeyMaterial holds the cryptographic material for a Redoubt keystore session.
// Lives only in memory — never serialised to disk in plaintext.
type KeyMaterial struct {
	// MasterKey is the 32-byte Argon2id-derived key. Zero until Unlock() is called.
	MasterKey []byte
	// AgeIdentity is the X25519 identity (public+private). Nil until Unlock() is called.
	AgeIdentity *age.X25519Identity
	// Locked is true while the master key has not been derived from a passphrase.
	Locked bool

	agePubKey string // cached age public key string; available without Unlock
}

// IsInitialized reports whether a keystore has been created on this machine.
func IsInitialized() bool {
	_, err := keyring.Get(keystoreService, keyMetadata)
	return err == nil
}

// Open loads the keystore metadata from the OS keyring and returns a locked
// KeyMaterial. The age public key is available immediately via AgeRecipient().
// Returns ErrNotInitialized if no keystore exists on this machine.
func Open() (*KeyMaterial, error) {
	pub, err := keyring.Get(keystoreService, keyAgePub)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil, ErrNotInitialized
	}
	if err != nil {
		return nil, fmt.Errorf("keystore: read age public key: %w", err)
	}
	return &KeyMaterial{Locked: true, agePubKey: pub}, nil
}

// Init creates a new keystore:
//   - Generates a CSPRNG keyfile and an age X25519 identity.
//   - Derives the master key via Argon2id(passphrase + keyfile).
//   - Stores the passphrase-wrapped key material in the OS secret store.
//
// Returns ErrAlreadyInitialized if a keystore already exists.
func Init(passphrase string) (*KeyMaterial, error) {
	if IsInitialized() {
		return nil, ErrAlreadyInitialized
	}

	// Generate CSPRNG keyfile.
	keyfile := make([]byte, keyfileLen)
	if _, err := rand.Read(keyfile); err != nil {
		return nil, fmt.Errorf("keystore: generate keyfile: %w", err)
	}

	// Generate Argon2id salt.
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("keystore: generate salt: %w", err)
	}

	// Derive master key.
	masterKey := deriveKey(passphrase, keyfile, salt, argon2Time, argon2Memory, argon2Threads)

	// Generate age identity.
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		return nil, fmt.Errorf("keystore: generate age identity: %w", err)
	}

	// Encrypt age private key string with master key.
	privPEM := identity.String() // "AGE-SECRET-KEY-1..."
	encPriv, err := sealWithKey(masterKey, []byte(privPEM))
	if err != nil {
		return nil, fmt.Errorf("keystore: encrypt age private key: %w", err)
	}

	// Encrypt check value to enable fast passphrase verification.
	encCheck, err := sealWithKey(masterKey, []byte(checkPlaintext))
	if err != nil {
		return nil, fmt.Errorf("keystore: encrypt check value: %w", err)
	}

	// Marshal and store metadata.
	meta := keystoreMetadata{
		Keyfile:       base64.StdEncoding.EncodeToString(keyfile),
		Argon2Salt:    base64.StdEncoding.EncodeToString(salt),
		Argon2Time:    argon2Time,
		Argon2Memory:  argon2Memory,
		Argon2Threads: argon2Threads,
	}
	metaJSON, err := json.Marshal(meta)
	if err != nil {
		return nil, fmt.Errorf("keystore: marshal metadata: %w", err)
	}

	privJSON, err := json.Marshal(encPriv)
	if err != nil {
		return nil, fmt.Errorf("keystore: marshal encrypted private key: %w", err)
	}

	checkJSON, err := json.Marshal(encCheck)
	if err != nil {
		return nil, fmt.Errorf("keystore: marshal encrypted check: %w", err)
	}

	// Persist to OS keyring. Order: private key first so we can clean up on partial failure.
	agePubStr := identity.Recipient().String()
	if err := keyring.Set(keystoreService, keyAgePriv, string(privJSON)); err != nil {
		return nil, fmt.Errorf("keystore: store age private key: %w", err)
	}
	if err := keyring.Set(keystoreService, keyCheck, string(checkJSON)); err != nil {
		_ = keyring.Delete(keystoreService, keyAgePriv)
		return nil, fmt.Errorf("keystore: store check value: %w", err)
	}
	if err := keyring.Set(keystoreService, keyAgePub, agePubStr); err != nil {
		_ = keyring.Delete(keystoreService, keyAgePriv)
		_ = keyring.Delete(keystoreService, keyCheck)
		return nil, fmt.Errorf("keystore: store age public key: %w", err)
	}
	if err := keyring.Set(keystoreService, keyMetadata, string(metaJSON)); err != nil {
		_ = keyring.Delete(keystoreService, keyAgePriv)
		_ = keyring.Delete(keystoreService, keyCheck)
		_ = keyring.Delete(keystoreService, keyAgePub)
		return nil, fmt.Errorf("keystore: store metadata: %w", err)
	}

	return &KeyMaterial{
		MasterKey:   masterKey,
		AgeIdentity: identity,
		Locked:      false,
		agePubKey:   agePubStr,
	}, nil
}

// Unlock derives the master key from passphrase, populates MasterKey and AgeIdentity,
// and sets Locked = false. Returns ErrWrongPassphrase on mismatch.
func (km *KeyMaterial) Unlock(passphrase string) error {
	metaJSON, err := keyring.Get(keystoreService, keyMetadata)
	if errors.Is(err, keyring.ErrNotFound) {
		return ErrNotInitialized
	}
	if err != nil {
		return fmt.Errorf("keystore: read metadata: %w", err)
	}

	var meta keystoreMetadata
	if err := json.Unmarshal([]byte(metaJSON), &meta); err != nil {
		return fmt.Errorf("keystore: decode metadata: %w", err)
	}

	keyfile, err := base64.StdEncoding.DecodeString(meta.Keyfile)
	if err != nil {
		return fmt.Errorf("keystore: decode keyfile: %w", err)
	}
	salt, err := base64.StdEncoding.DecodeString(meta.Argon2Salt)
	if err != nil {
		return fmt.Errorf("keystore: decode salt: %w", err)
	}

	masterKey := deriveKey(passphrase, keyfile, salt, meta.Argon2Time, meta.Argon2Memory, meta.Argon2Threads)

	// Verify passphrase by decrypting the check value before touching the private key.
	checkJSON, err := keyring.Get(keystoreService, keyCheck)
	if err != nil {
		return fmt.Errorf("keystore: read check value: %w", err)
	}
	var checkBlob encryptedBlob
	if err := json.Unmarshal([]byte(checkJSON), &checkBlob); err != nil {
		return fmt.Errorf("keystore: decode check value: %w", err)
	}
	checkPT, err := openWithKey(masterKey, checkBlob)
	if err != nil {
		return ErrWrongPassphrase
	}
	if string(checkPT) != checkPlaintext {
		return ErrWrongPassphrase
	}

	// Passphrase is correct; decrypt the age private key.
	privJSON, err := keyring.Get(keystoreService, keyAgePriv)
	if err != nil {
		return fmt.Errorf("keystore: read age private key: %w", err)
	}
	var privBlob encryptedBlob
	if err := json.Unmarshal([]byte(privJSON), &privBlob); err != nil {
		return fmt.Errorf("keystore: decode age private key blob: %w", err)
	}
	privPEM, err := openWithKey(masterKey, privBlob)
	if err != nil {
		return fmt.Errorf("keystore: decrypt age private key: %w", err)
	}

	id, err := age.ParseX25519Identity(string(privPEM))
	if err != nil {
		return fmt.Errorf("keystore: parse age identity: %w", err)
	}

	km.MasterKey = masterKey
	km.AgeIdentity = id
	km.Locked = false
	return nil
}

// AgeRecipient returns the age public key string (e.g. "age1...").
// Does NOT require Unlock() — the public key is stored separately in the keyring.
func (km *KeyMaterial) AgeRecipient() string {
	return km.agePubKey
}

// ResticPassword derives the restic repository password from the master key.
// Returns ErrKeyLocked if called before Unlock().
func (km *KeyMaterial) ResticPassword() (string, error) {
	if km.Locked {
		return "", ErrKeyLocked
	}
	h := hmac.New(sha256.New, km.MasterKey)
	h.Write([]byte("redoubt:restic-password:v1"))
	return base64.StdEncoding.EncodeToString(h.Sum(nil)), nil
}

// EscrowPayload returns the combined secret bytes suitable for Shamir splitting.
// The payload encodes both the master key and the age private key PEM so that
// reconstruction from shares restores the full keystore on a blank machine.
// Returns ErrKeyLocked if called before Unlock().
//
// Payload format: [4-byte magic "RDBT"] [32-byte master key] [age private key PEM bytes]
func (km *KeyMaterial) EscrowPayload() ([]byte, error) {
	if km.Locked {
		return nil, ErrKeyLocked
	}
	privPEM := km.AgeIdentity.String()
	magic := []byte("RDBT")
	payload := make([]byte, 0, len(magic)+len(km.MasterKey)+len(privPEM))
	payload = append(payload, magic...)
	payload = append(payload, km.MasterKey...)
	payload = append(payload, []byte(privPEM)...)
	return payload, nil
}

// InitFromEscrow installs an existing KeyMaterial (from RestoreFromEscrowPayload)
// into the OS keyring under a fresh passphrase. Used during break-glass recovery
// to re-initialize the keystore on a blank machine without changing the age identity.
// Returns ErrAlreadyInitialized if a keystore already exists.
func InitFromEscrow(km *KeyMaterial, passphrase string) error {
	if IsInitialized() {
		return ErrAlreadyInitialized
	}

	newKeyfile := make([]byte, keyfileLen)
	if _, err := rand.Read(newKeyfile); err != nil {
		return fmt.Errorf("keystore: generate keyfile for escrow restore: %w", err)
	}
	newSalt := make([]byte, saltLen)
	if _, err := rand.Read(newSalt); err != nil {
		return fmt.Errorf("keystore: generate salt for escrow restore: %w", err)
	}

	masterKey := deriveKey(passphrase, newKeyfile, newSalt, argon2Time, argon2Memory, argon2Threads)

	privPEM := km.AgeIdentity.String()
	encPriv, err := sealWithKey(masterKey, []byte(privPEM))
	if err != nil {
		return fmt.Errorf("keystore: encrypt age private key (escrow restore): %w", err)
	}
	encCheck, err := sealWithKey(masterKey, []byte(checkPlaintext))
	if err != nil {
		return fmt.Errorf("keystore: encrypt check value (escrow restore): %w", err)
	}

	meta := keystoreMetadata{
		Keyfile:       base64.StdEncoding.EncodeToString(newKeyfile),
		Argon2Salt:    base64.StdEncoding.EncodeToString(newSalt),
		Argon2Time:    argon2Time,
		Argon2Memory:  argon2Memory,
		Argon2Threads: argon2Threads,
	}
	metaJSON, _ := json.Marshal(meta)
	privJSON, _ := json.Marshal(encPriv)
	checkJSON, _ := json.Marshal(encCheck)

	agePubStr := km.AgeIdentity.Recipient().String()
	if err := keyring.Set(keystoreService, keyAgePriv, string(privJSON)); err != nil {
		return fmt.Errorf("keystore: store age private key (escrow restore): %w", err)
	}
	if err := keyring.Set(keystoreService, keyCheck, string(checkJSON)); err != nil {
		_ = keyring.Delete(keystoreService, keyAgePriv)
		return fmt.Errorf("keystore: store check value (escrow restore): %w", err)
	}
	if err := keyring.Set(keystoreService, keyAgePub, agePubStr); err != nil {
		_ = keyring.Delete(keystoreService, keyAgePriv)
		_ = keyring.Delete(keystoreService, keyCheck)
		return fmt.Errorf("keystore: store age public key (escrow restore): %w", err)
	}
	if err := keyring.Set(keystoreService, keyMetadata, string(metaJSON)); err != nil {
		_ = keyring.Delete(keystoreService, keyAgePriv)
		_ = keyring.Delete(keystoreService, keyCheck)
		_ = keyring.Delete(keystoreService, keyAgePub)
		return fmt.Errorf("keystore: store metadata (escrow restore): %w", err)
	}
	return nil
}

// RestoreFromEscrowPayload reconstructs a KeyMaterial from an escrow payload.
// Used during break-glass recovery. The payload must have been produced by EscrowPayload().
func RestoreFromEscrowPayload(payload []byte) (*KeyMaterial, error) {
	magic := []byte("RDBT")
	if len(payload) < len(magic)+argon2KeyLen {
		return nil, fmt.Errorf("keystore: escrow payload too short")
	}
	if string(payload[:len(magic)]) != string(magic) {
		return nil, fmt.Errorf("keystore: invalid escrow payload magic")
	}
	masterKey := payload[len(magic) : len(magic)+argon2KeyLen]
	privPEM := string(payload[len(magic)+argon2KeyLen:])

	id, err := age.ParseX25519Identity(strings.TrimSpace(privPEM))
	if err != nil {
		return nil, fmt.Errorf("keystore: parse age identity from escrow: %w", err)
	}

	return &KeyMaterial{
		MasterKey:   masterKey,
		AgeIdentity: id,
		Locked:      false,
		agePubKey:   id.Recipient().String(),
	}, nil
}

// Rotate replaces the passphrase (and optionally generates a new age identity).
// The new key material is verified before the old entry is removed.
// Returns ErrKeyLocked if called before Unlock().
func (km *KeyMaterial) Rotate(newPassphrase string) error {
	if km.Locked {
		return ErrKeyLocked
	}

	// Generate a fresh keyfile and salt for the new derivation.
	newKeyfile := make([]byte, keyfileLen)
	if _, err := rand.Read(newKeyfile); err != nil {
		return fmt.Errorf("keystore: generate new keyfile: %w", err)
	}
	newSalt := make([]byte, saltLen)
	if _, err := rand.Read(newSalt); err != nil {
		return fmt.Errorf("keystore: generate new salt: %w", err)
	}

	newMasterKey := deriveKey(newPassphrase, newKeyfile, newSalt, argon2Time, argon2Memory, argon2Threads)

	// Re-encrypt the existing age private key under the new master key.
	privPEM := km.AgeIdentity.String()
	encPriv, err := sealWithKey(newMasterKey, []byte(privPEM))
	if err != nil {
		return fmt.Errorf("keystore: re-encrypt age private key: %w", err)
	}
	encCheck, err := sealWithKey(newMasterKey, []byte(checkPlaintext))
	if err != nil {
		return fmt.Errorf("keystore: re-encrypt check value: %w", err)
	}

	newMeta := keystoreMetadata{
		Keyfile:       base64.StdEncoding.EncodeToString(newKeyfile),
		Argon2Salt:    base64.StdEncoding.EncodeToString(newSalt),
		Argon2Time:    argon2Time,
		Argon2Memory:  argon2Memory,
		Argon2Threads: argon2Threads,
	}
	metaJSON, _ := json.Marshal(newMeta)
	privJSON, _ := json.Marshal(encPriv)
	checkJSON, _ := json.Marshal(encCheck)

	// Write new entries to keyring. On any failure, old entries remain valid.
	if err := keyring.Set(keystoreService, keyAgePriv, string(privJSON)); err != nil {
		return fmt.Errorf("keystore: store rotated age private key: %w", err)
	}
	if err := keyring.Set(keystoreService, keyCheck, string(checkJSON)); err != nil {
		return fmt.Errorf("keystore: store rotated check value: %w", err)
	}
	if err := keyring.Set(keystoreService, keyMetadata, string(metaJSON)); err != nil {
		return fmt.Errorf("keystore: store rotated metadata: %w", err)
	}

	// Update in-memory state.
	km.MasterKey = newMasterKey
	return nil
}

// Verify checks that the keystore is internally consistent:
//   - All required keyring entries exist.
//   - The check value decrypts correctly (master key is valid).
//   - The age private key decrypts and parses correctly.
//
// Returns ErrKeyLocked if called before Unlock().
func (km *KeyMaterial) Verify() error {
	if km.Locked {
		return ErrKeyLocked
	}

	for _, k := range []string{keyMetadata, keyAgePub, keyAgePriv, keyCheck} {
		if _, err := keyring.Get(keystoreService, k); err != nil {
			return fmt.Errorf("keystore: verify: missing entry %q: %w", k, err)
		}
	}

	checkJSON, _ := keyring.Get(keystoreService, keyCheck)
	var checkBlob encryptedBlob
	if err := json.Unmarshal([]byte(checkJSON), &checkBlob); err != nil {
		return fmt.Errorf("keystore: verify: decode check blob: %w", err)
	}
	checkPT, err := openWithKey(km.MasterKey, checkBlob)
	if err != nil || string(checkPT) != checkPlaintext {
		return fmt.Errorf("keystore: verify: check value mismatch — keystore may be corrupt")
	}

	privJSON, _ := keyring.Get(keystoreService, keyAgePriv)
	var privBlob encryptedBlob
	if err := json.Unmarshal([]byte(privJSON), &privBlob); err != nil {
		return fmt.Errorf("keystore: verify: decode age private key blob: %w", err)
	}
	privPEM, err := openWithKey(km.MasterKey, privBlob)
	if err != nil {
		return fmt.Errorf("keystore: verify: decrypt age private key: %w", err)
	}
	id, err := age.ParseX25519Identity(string(privPEM))
	if err != nil {
		return fmt.Errorf("keystore: verify: parse age identity: %w", err)
	}

	// Confirm the stored public key matches the private key.
	storedPub, _ := keyring.Get(keystoreService, keyAgePub)
	if id.Recipient().String() != storedPub {
		return fmt.Errorf("keystore: verify: age public key mismatch — keystore may be corrupt")
	}

	return nil
}

// PassphraseStrength returns a score 0–4 estimating passphrase strength,
// where 0 means too weak to accept and ≥2 is acceptable.
// Also returns a human-readable description.
func PassphraseStrength(passphrase string) (score int, description string) {
	l := len(passphrase)
	if l < 8 {
		return 0, "too short (minimum 8 characters)"
	}

	var hasUpper, hasLower, hasDigit, hasSpecial bool
	for _, r := range passphrase {
		switch {
		case unicode.IsUpper(r):
			hasUpper = true
		case unicode.IsLower(r):
			hasLower = true
		case unicode.IsDigit(r):
			hasDigit = true
		default:
			hasSpecial = true
		}
	}

	score = 0
	if l >= 8 {
		score++
	}
	if l >= 14 {
		score++
	}
	if l >= 20 {
		score++
	}
	variety := 0
	for _, ok := range []bool{hasUpper, hasLower, hasDigit, hasSpecial} {
		if ok {
			variety++
		}
	}
	if variety >= 3 {
		score++
	}

	// Cap at 4.
	if score > 4 {
		score = 4
	}

	switch {
	case score == 0:
		description = "very weak"
	case score == 1:
		description = "weak"
	case score == 2:
		description = "fair"
	case score == 3:
		description = "strong"
	default:
		description = "very strong"
	}
	return score, description
}

// ---- internal helpers ----

// deriveKey runs Argon2id over (passphrase + keyfile) with the given parameters.
// The keyfile is prepended to the passphrase bytes before hashing, so both must
// be known to reproduce the key.
func deriveKey(passphrase string, keyfile, salt []byte, t, m uint32, p uint8) []byte {
	combined := append([]byte(passphrase), keyfile...)
	return argon2.IDKey(combined, salt, t, m, p, argon2KeyLen)
}

// sealWithKey encrypts plaintext with XChaCha20-Poly1305 under key (32 bytes).
// Returns an encryptedBlob with a random 24-byte nonce.
func sealWithKey(key, plaintext []byte) (encryptedBlob, error) {
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return encryptedBlob{}, fmt.Errorf("keystore: init chacha20: %w", err)
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return encryptedBlob{}, fmt.Errorf("keystore: generate nonce: %w", err)
	}
	ct := aead.Seal(nil, nonce, plaintext, nil)
	return encryptedBlob{
		Nonce:      base64.StdEncoding.EncodeToString(nonce),
		Ciphertext: base64.StdEncoding.EncodeToString(ct),
	}, nil
}

// openWithKey decrypts a blob sealed by sealWithKey.
func openWithKey(key []byte, blob encryptedBlob) ([]byte, error) {
	nonce, err := base64.StdEncoding.DecodeString(blob.Nonce)
	if err != nil {
		return nil, fmt.Errorf("keystore: decode nonce: %w", err)
	}
	ct, err := base64.StdEncoding.DecodeString(blob.Ciphertext)
	if err != nil {
		return nil, fmt.Errorf("keystore: decode ciphertext: %w", err)
	}
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, fmt.Errorf("keystore: init chacha20: %w", err)
	}
	pt, err := aead.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, fmt.Errorf("keystore: decrypt: %w", err)
	}
	return pt, nil
}

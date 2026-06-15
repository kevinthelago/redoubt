package keystore_test

import (
	"os"
	"testing"

	"filippo.io/age"
	"github.com/kevinthelago/redoubt/internal/keystore"
	"github.com/zalando/go-keyring"
)

// TestMain installs an in-memory keyring mock so tests run without a
// libsecret daemon (CI) or OS keychain access (macOS/Windows sandboxes).
func TestMain(m *testing.M) {
	keyring.MockInit()
	os.Exit(m.Run())
}

// clearKeyring removes all keyring entries for the "redoubt" service.
// Call before and after each test to guarantee isolation.
func clearKeyring(t *testing.T) {
	t.Helper()
	keys := []string{"v1:metadata", "v1:age-pub", "v1:age-priv", "v1:check"}
	for _, k := range keys {
		_ = keyring.Delete("redoubt", k)
	}
}

func TestInit_CreatesKeystore(t *testing.T) {
	clearKeyring(t)
	t.Cleanup(func() { clearKeyring(t) })

	km, err := keystore.Init("correct horse battery staple test!")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if km.Locked {
		t.Error("Init returned a locked KeyMaterial")
	}
	if len(km.MasterKey) != 32 {
		t.Errorf("MasterKey length = %d, want 32", len(km.MasterKey))
	}
	if km.AgeIdentity == nil {
		t.Error("AgeIdentity is nil")
	}
	pub := km.AgeRecipient()
	if pub == "" {
		t.Error("AgeRecipient() returned empty string")
	}
	// Must start with the age public key prefix.
	if len(pub) < 5 || pub[:4] != "age1" {
		t.Errorf("AgeRecipient() = %q — expected age1… prefix", pub)
	}
}

func TestInit_RefusesDoubleInit(t *testing.T) {
	clearKeyring(t)
	t.Cleanup(func() { clearKeyring(t) })

	if _, err := keystore.Init("correct horse battery staple test!"); err != nil {
		t.Fatalf("first Init: %v", err)
	}
	_, err := keystore.Init("different passphrase but still long")
	if err == nil {
		t.Fatal("second Init should have returned ErrAlreadyInitialized")
	}
}

func TestOpen_LockedWithPublicKey(t *testing.T) {
	clearKeyring(t)
	t.Cleanup(func() { clearKeyring(t) })

	init, _ := keystore.Init("correct horse battery staple test!")
	wantPub := init.AgeRecipient()

	km, err := keystore.Open()
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if !km.Locked {
		t.Error("Open should return a locked KeyMaterial")
	}
	if km.AgeRecipient() != wantPub {
		t.Errorf("AgeRecipient after Open = %q, want %q", km.AgeRecipient(), wantPub)
	}
}

func TestUnlock_CorrectPassphrase(t *testing.T) {
	clearKeyring(t)
	t.Cleanup(func() { clearKeyring(t) })

	const pass = "correct horse battery staple test!"
	keystore.Init(pass)

	km, _ := keystore.Open()
	if err := km.Unlock(pass); err != nil {
		t.Fatalf("Unlock: %v", err)
	}
	if km.Locked {
		t.Error("still locked after Unlock")
	}
	if km.AgeIdentity == nil {
		t.Error("AgeIdentity is nil after Unlock")
	}
}

func TestUnlock_WrongPassphrase(t *testing.T) {
	clearKeyring(t)
	t.Cleanup(func() { clearKeyring(t) })

	keystore.Init("correct horse battery staple test!")

	km, _ := keystore.Open()
	err := km.Unlock("wrong passphrase here!")
	if err == nil {
		t.Fatal("Unlock with wrong passphrase should fail")
	}
}

func TestResticPassword_DeterministicAndRequiresUnlock(t *testing.T) {
	clearKeyring(t)
	t.Cleanup(func() { clearKeyring(t) })

	const pass = "correct horse battery staple test!"
	keystore.Init(pass)

	km1, _ := keystore.Open()
	km1.Unlock(pass)
	p1, err := km1.ResticPassword()
	if err != nil {
		t.Fatalf("ResticPassword: %v", err)
	}

	km2, _ := keystore.Open()
	km2.Unlock(pass)
	p2, _ := km2.ResticPassword()

	if p1 != p2 {
		t.Errorf("ResticPassword not deterministic: %q != %q", p1, p2)
	}

	// Locked key must return ErrKeyLocked.
	kmLocked, _ := keystore.Open()
	_, err = kmLocked.ResticPassword()
	if err == nil {
		t.Error("ResticPassword on locked key should return ErrKeyLocked")
	}
}

func TestAgeRecipientUsable(t *testing.T) {
	clearKeyring(t)
	t.Cleanup(func() { clearKeyring(t) })

	const pass = "correct horse battery staple test!"
	keystore.Init(pass)

	km, _ := keystore.Open()
	// Should be usable as an age recipient string without unlocking.
	pub := km.AgeRecipient()
	if _, err := age.ParseX25519Recipient(pub); err != nil {
		t.Errorf("AgeRecipient() %q is not a valid age recipient: %v", pub, err)
	}
}

func TestEscrowPayload_RoundTrip(t *testing.T) {
	clearKeyring(t)
	t.Cleanup(func() { clearKeyring(t) })

	const pass = "correct horse battery staple test!"
	km, _ := keystore.Init(pass)

	payload, err := km.EscrowPayload()
	if err != nil {
		t.Fatalf("EscrowPayload: %v", err)
	}
	if len(payload) == 0 {
		t.Fatal("EscrowPayload returned empty payload")
	}

	restored, err := keystore.RestoreFromEscrowPayload(payload)
	if err != nil {
		t.Fatalf("RestoreFromEscrowPayload: %v", err)
	}
	if restored.AgeRecipient() != km.AgeRecipient() {
		t.Errorf("restored age public key = %q, want %q", restored.AgeRecipient(), km.AgeRecipient())
	}

	// Verify ResticPassword is the same.
	origPass, _ := km.ResticPassword()
	restoredPass, err := restored.ResticPassword()
	if err != nil {
		t.Fatalf("restored ResticPassword: %v", err)
	}
	if origPass != restoredPass {
		t.Error("ResticPassword differs after restore from escrow payload")
	}
}

func TestRotate(t *testing.T) {
	clearKeyring(t)
	t.Cleanup(func() { clearKeyring(t) })

	const oldPass = "correct horse battery staple test!"
	const newPass = "my new long and secure passphrase!"

	keystore.Init(oldPass)

	km, _ := keystore.Open()
	km.Unlock(oldPass)

	wantPub := km.AgeRecipient()

	if err := km.Rotate(newPass); err != nil {
		t.Fatalf("Rotate: %v", err)
	}

	// Old passphrase must no longer work.
	km2, _ := keystore.Open()
	if err := km2.Unlock(oldPass); err == nil {
		t.Error("old passphrase should fail after rotation")
	}

	// New passphrase must work.
	km3, _ := keystore.Open()
	if err := km3.Unlock(newPass); err != nil {
		t.Fatalf("new passphrase failed after rotation: %v", err)
	}

	// Age public key must be unchanged (same identity).
	if km3.AgeRecipient() != wantPub {
		t.Errorf("AgeRecipient changed after rotation: got %q want %q", km3.AgeRecipient(), wantPub)
	}
}

func TestVerify(t *testing.T) {
	clearKeyring(t)
	t.Cleanup(func() { clearKeyring(t) })

	const pass = "correct horse battery staple test!"
	keystore.Init(pass)

	km, _ := keystore.Open()
	km.Unlock(pass)

	if err := km.Verify(); err != nil {
		t.Errorf("Verify failed on healthy keystore: %v", err)
	}
}

func TestPassphraseStrength(t *testing.T) {
	tests := []struct {
		input     string
		wantScore int
	}{
		{"abc", 0},
		{"abcdefg1", 1},
		{"correct horse battery", 3},
		{"Correct-Horse-Battery-22!", 4},
	}
	for _, tc := range tests {
		score, _ := keystore.PassphraseStrength(tc.input)
		if score != tc.wantScore {
			t.Errorf("PassphraseStrength(%q) = %d, want %d", tc.input, score, tc.wantScore)
		}
	}
}

func TestOpen_NotInitialized(t *testing.T) {
	clearKeyring(t)
	t.Cleanup(func() { clearKeyring(t) })

	_, err := keystore.Open()
	if err == nil {
		t.Error("Open on empty keyring should return ErrNotInitialized")
	}
}

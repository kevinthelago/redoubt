package age_test

import (
	"bytes"
	"crypto/rand"
	"io"
	"strings"
	"testing"

	"filippo.io/age"
	redoubtage "github.com/kevinthelago/redoubt/internal/age"
)

func mustGenerate(t *testing.T) *age.X25519Identity {
	t.Helper()
	id, err := redoubtage.GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity: %v", err)
	}
	return id
}

func TestSealUnseal_SmallPayload(t *testing.T) {
	id := mustGenerate(t)
	plaintext := []byte("hello, redoubt!")

	var ciphertext bytes.Buffer
	if err := redoubtage.Seal(&ciphertext, bytes.NewReader(plaintext), id.Recipient()); err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if ciphertext.Len() == 0 {
		t.Fatal("Seal produced empty ciphertext")
	}

	var recovered bytes.Buffer
	if err := redoubtage.Unseal(&recovered, &ciphertext, id); err != nil {
		t.Fatalf("Unseal: %v", err)
	}
	if !bytes.Equal(recovered.Bytes(), plaintext) {
		t.Errorf("round-trip mismatch: got %q, want %q", recovered.Bytes(), plaintext)
	}
}

func TestSealUnseal_EmptyPayload(t *testing.T) {
	id := mustGenerate(t)

	var ciphertext bytes.Buffer
	if err := redoubtage.Seal(&ciphertext, strings.NewReader(""), id.Recipient()); err != nil {
		t.Fatalf("Seal empty: %v", err)
	}

	var recovered bytes.Buffer
	if err := redoubtage.Unseal(&recovered, &ciphertext, id); err != nil {
		t.Fatalf("Unseal empty: %v", err)
	}
	if recovered.Len() != 0 {
		t.Errorf("expected empty plaintext, got %d bytes", recovered.Len())
	}
}

// TestSealUnseal_LargePayload_ByteExact verifies byte-exact round-trip over a
// 4 MiB payload to exercise the streaming path.
func TestSealUnseal_LargePayload_ByteExact(t *testing.T) {
	id := mustGenerate(t)

	plaintext := make([]byte, 4<<20)
	if _, err := io.ReadFull(rand.Reader, plaintext); err != nil {
		t.Fatalf("rand.Read: %v", err)
	}

	var ciphertext bytes.Buffer
	if err := redoubtage.Seal(&ciphertext, bytes.NewReader(plaintext), id.Recipient()); err != nil {
		t.Fatalf("Seal large: %v", err)
	}

	var recovered bytes.Buffer
	if err := redoubtage.Unseal(&recovered, &ciphertext, id); err != nil {
		t.Fatalf("Unseal large: %v", err)
	}
	if !bytes.Equal(recovered.Bytes(), plaintext) {
		t.Errorf("large payload round-trip: %d bytes recovered, want %d", recovered.Len(), len(plaintext))
	}
}

func TestUnseal_WrongIdentity_ReturnsError(t *testing.T) {
	sender := mustGenerate(t)
	attacker := mustGenerate(t)

	var ciphertext bytes.Buffer
	if err := redoubtage.Seal(&ciphertext, strings.NewReader("secret"), sender.Recipient()); err != nil {
		t.Fatalf("Seal: %v", err)
	}

	var out bytes.Buffer
	if err := redoubtage.Unseal(&out, &ciphertext, attacker); err == nil {
		t.Fatal("Unseal with wrong identity: expected error, got nil")
	}
}

func TestSealUnseal_MultipleRecipients(t *testing.T) {
	alice := mustGenerate(t)
	bob := mustGenerate(t)

	plaintext := []byte("shared secret")
	var ciphertext bytes.Buffer
	if err := redoubtage.Seal(&ciphertext, bytes.NewReader(plaintext), alice.Recipient(), bob.Recipient()); err != nil {
		t.Fatalf("Seal multi-recipient: %v", err)
	}

	for _, tc := range []struct {
		name string
		id   age.Identity
	}{
		{"alice", alice},
		{"bob", bob},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ct := bytes.NewReader(ciphertext.Bytes())
			var out bytes.Buffer
			if err := redoubtage.Unseal(&out, ct, tc.id); err != nil {
				t.Fatalf("Unseal (%s): %v", tc.name, err)
			}
			if !bytes.Equal(out.Bytes(), plaintext) {
				t.Errorf("%s: byte-exact mismatch", tc.name)
			}
		})
	}
}

// TestCiphertext_DifferentFromPlaintext is a sanity check that the sealed
// output does not contain the plaintext verbatim.
func TestCiphertext_DifferentFromPlaintext(t *testing.T) {
	id := mustGenerate(t)
	plaintext := []byte("plaintext must not appear in ciphertext stream")

	var ct bytes.Buffer
	if err := redoubtage.Seal(&ct, bytes.NewReader(plaintext), id.Recipient()); err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if bytes.Contains(ct.Bytes(), plaintext) {
		t.Error("ciphertext contains plaintext verbatim — encryption failed")
	}
}

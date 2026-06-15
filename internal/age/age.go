// Package age wraps filippo.io/age for streaming seal/unseal of arbitrary data.
//
// Design constraints:
//   - No plaintext is written to disk beyond the explicit target writer.
//   - Encryption and decryption are fully streamed; no full-copy buffering.
package age

import (
	"fmt"
	"io"

	"filippo.io/age"
)

// GenerateIdentity creates a new X25519 identity (private/public key pair).
func GenerateIdentity() (*age.X25519Identity, error) {
	id, err := age.GenerateX25519Identity()
	if err != nil {
		return nil, fmt.Errorf("age: generate identity: %w", err)
	}
	return id, nil
}

// Seal encrypts all bytes from src to the given recipients and streams the
// ciphertext to dst.  Returns after the ciphertext is fully written and
// finalised.  No intermediate plaintext buffer is held in memory.
func Seal(dst io.Writer, src io.Reader, recipients ...age.Recipient) error {
	w, err := age.Encrypt(dst, recipients...)
	if err != nil {
		return fmt.Errorf("age: encrypt init: %w", err)
	}
	if _, err := io.Copy(w, src); err != nil {
		return fmt.Errorf("age: encrypt copy: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("age: encrypt close: %w", err)
	}
	return nil
}

// Unseal decrypts src using the first matching identity and streams the
// plaintext to dst.  Returns an error if no identity can decrypt the file.
func Unseal(dst io.Writer, src io.Reader, identities ...age.Identity) error {
	r, err := age.Decrypt(src, identities...)
	if err != nil {
		return fmt.Errorf("age: decrypt init: %w", err)
	}
	if _, err := io.Copy(dst, r); err != nil {
		return fmt.Errorf("age: decrypt copy: %w", err)
	}
	return nil
}

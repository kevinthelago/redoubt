// Package escrow implements Shamir's Secret Sharing for Redoubt's break-glass
// recovery path.
//
// The secret is split into N shares, any M (threshold ≤ N) of which can
// reconstruct it. The underlying primitive is corvus-ch/shamir, a GF(2^8)
// implementation of the same algorithm used in the HashiCorp Vault SDK.
//
// The Split call performs an immediate reconstruction verification before
// returning. Escrow is only considered complete when that round-trip succeeds.
package escrow

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"

	"github.com/corvus-ch/shamir"
)

// Sentinel errors.
var (
	ErrInsufficientShares = errors.New("escrow: insufficient shares to reconstruct")
	ErrShareMismatch      = errors.New("escrow: share checksum mismatch — share may be corrupted")
	ErrVerifyFailed       = errors.New("escrow: reconstruction verification failed — escrow aborted")
)

// Share is a single Shamir share.
type Share struct {
	Index    int    // 1-based share index (matches the GF(2^8) x-coordinate byte)
	Data     []byte // raw share data bytes
	Checksum string // SHA-256 hex of Data; verified by Reconstruct before use
}

// EscrowResult is returned from a successful Split.
// Verified is always true on return; Split returns ErrVerifyFailed instead of
// setting it to false.
type EscrowResult struct {
	Shares    []Share
	Threshold int
	Total     int
	Verified  bool
}

// Split splits secret into total Shamir shares, any threshold of which are
// required to reconstruct the original. Performs an immediate in-memory
// verification before returning. Returns ErrVerifyFailed if verification fails.
func Split(secret []byte, threshold, total int) (*EscrowResult, error) {
	if threshold < 2 {
		return nil, fmt.Errorf("escrow: threshold must be ≥ 2, got %d", threshold)
	}
	if total < threshold {
		return nil, fmt.Errorf("escrow: total (%d) must be ≥ threshold (%d)", total, threshold)
	}
	if total > 255 {
		return nil, fmt.Errorf("escrow: total must be ≤ 255, got %d", total)
	}
	if len(secret) == 0 {
		return nil, fmt.Errorf("escrow: secret must not be empty")
	}

	// corvus-ch/shamir.Split(secret, parts, threshold) returns map[byte][]byte
	rawMap, err := shamir.Split(secret, total, threshold)
	if err != nil {
		return nil, fmt.Errorf("escrow: shamir split: %w", err)
	}

	// Convert map to ordered slice of Share, sorted by index for determinism.
	shares := make([]Share, 0, len(rawMap))
	for idx, data := range rawMap {
		shares = append(shares, Share{
			Index:    int(idx),
			Data:     data,
			Checksum: checksum(data),
		})
	}
	sort.Slice(shares, func(i, j int) bool { return shares[i].Index < shares[j].Index })

	// Verify: reconstruct from the first `threshold` shares and compare.
	verifyShares := make([]Share, threshold)
	copy(verifyShares, shares[:threshold])
	reconstructed, err := Reconstruct(verifyShares)
	if err != nil {
		return nil, ErrVerifyFailed
	}
	if !equal(reconstructed, secret) {
		return nil, ErrVerifyFailed
	}

	return &EscrowResult{
		Shares:    shares,
		Threshold: threshold,
		Total:     len(shares),
		Verified:  true,
	}, nil
}

// Reconstruct combines threshold-or-more shares to recover the original secret.
// Shares may arrive in any order. Each share's Checksum is validated before use.
// Returns ErrInsufficientShares if the library cannot reconstruct (fewer than
// threshold valid shares). Returns ErrShareMismatch if any checksum fails.
func Reconstruct(shares []Share) ([]byte, error) {
	if len(shares) == 0 {
		return nil, ErrInsufficientShares
	}

	// Validate checksums and build the map expected by corvus-ch/shamir.
	parts := make(map[byte][]byte, len(shares))
	for _, s := range shares {
		if s.Checksum != checksum(s.Data) {
			return nil, fmt.Errorf("%w: share %d", ErrShareMismatch, s.Index)
		}
		parts[byte(s.Index)] = s.Data
	}

	secret, err := shamir.Combine(parts)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInsufficientShares, err)
	}
	return secret, nil
}

// checksum returns the full hex-encoded SHA-256 of data.
func checksum(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// equal compares two byte slices in constant time.
func equal(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	diff := byte(0)
	for i := range a {
		diff |= a[i] ^ b[i]
	}
	return diff == 0
}

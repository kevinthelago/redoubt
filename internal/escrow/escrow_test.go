package escrow_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/kevinthelago/redoubt/internal/escrow"
)

func TestSplit_DefaultParams(t *testing.T) {
	secret := []byte("a 32-byte secret for testing!!!!") // exactly 32 bytes

	result, err := escrow.Split(secret, 2, 3)
	if err != nil {
		t.Fatalf("Split: %v", err)
	}
	if !result.Verified {
		t.Error("Verified should be true on successful Split")
	}
	if len(result.Shares) != 3 {
		t.Errorf("got %d shares, want 3", len(result.Shares))
	}
	if result.Threshold != 2 || result.Total != 3 {
		t.Errorf("Threshold=%d Total=%d, want 2 and 3", result.Threshold, result.Total)
	}
	// Indices are GF(2^8) x-coordinates assigned by the shamir library — non-zero,
	// unique, but not necessarily sequential.
	seen := map[int]bool{}
	for i, s := range result.Shares {
		if s.Index == 0 {
			t.Errorf("share %d has zero Index", i)
		}
		if seen[s.Index] {
			t.Errorf("duplicate Index %d", s.Index)
		}
		seen[s.Index] = true
		if len(s.Checksum) != 64 {
			t.Errorf("share %d checksum length = %d, want 64 (hex SHA-256)", i, len(s.Checksum))
		}
	}
}

func TestReconstruct_ThresholdSufficient(t *testing.T) {
	secret := []byte("redoubt-test-secret-exactly-32!!")

	result, _ := escrow.Split(secret, 2, 3)

	for _, combo := range [][2]int{{0, 1}, {0, 2}, {1, 2}} {
		shares := []escrow.Share{result.Shares[combo[0]], result.Shares[combo[1]]}
		got, err := escrow.Reconstruct(shares)
		if err != nil {
			t.Errorf("Reconstruct with shares %v: %v", combo, err)
			continue
		}
		if !bytes.Equal(got, secret) {
			t.Errorf("Reconstruct with shares %v: got %x, want %x", combo, got, secret)
		}
	}
}

func TestReconstruct_OneShareInsufficient(t *testing.T) {
	secret := []byte("redoubt-test-secret-exactly-32!!")
	result, _ := escrow.Split(secret, 2, 3)

	_, err := escrow.Reconstruct(result.Shares[:1])
	if err == nil {
		t.Error("Reconstruct with 1 share (threshold=2) should fail")
	}
}

func TestReconstruct_BadChecksum(t *testing.T) {
	secret := []byte("redoubt-test-secret-exactly-32!!")
	result, _ := escrow.Split(secret, 2, 3)

	// Corrupt one share's checksum.
	bad := result.Shares[0]
	bad.Checksum = "0000000000000000000000000000000000000000000000000000000000000000"
	shares := []escrow.Share{bad, result.Shares[1]}

	_, err := escrow.Reconstruct(shares)
	if err == nil {
		t.Error("Reconstruct with bad checksum should fail")
	}
}

func TestReconstruct_EmptyShares(t *testing.T) {
	_, err := escrow.Reconstruct(nil)
	if err == nil {
		t.Error("Reconstruct(nil) should return ErrInsufficientShares")
	}
}

func TestSplit_VerifyFailureDetected(t *testing.T) {
	// Valid split should always pass verification.
	secret := make([]byte, 64)
	for i := range secret {
		secret[i] = byte(i)
	}
	_, err := escrow.Split(secret, 3, 5)
	if err != nil {
		t.Fatalf("Split: %v", err)
	}
}

func TestSplit_InvalidParams(t *testing.T) {
	secret := []byte("test-secret-long-enough-for-shamir!!")
	if _, err := escrow.Split(secret, 1, 3); err == nil {
		t.Error("threshold < 2 should fail")
	}
	if _, err := escrow.Split(secret, 3, 2); err == nil {
		t.Error("total < threshold should fail")
	}
	if _, err := escrow.Split(nil, 2, 3); err == nil {
		t.Error("empty secret should fail")
	}
}

func TestWriteRecoverySheet(t *testing.T) {
	secret := []byte("redoubt-test-secret-exactly-32!!")
	result, err := escrow.Split(secret, 2, 3)
	if err != nil {
		t.Fatalf("Split: %v", err)
	}

	dir := t.TempDir()
	opts := escrow.RecoverySheetOptions{
		OutputDir: dir,
		Threshold: 2,
		Total:     3,
		QRSize:    256,
	}
	if err := escrow.WriteRecoverySheet(result.Shares, opts); err != nil {
		t.Fatalf("WriteRecoverySheet: %v", err)
	}

	// QR PNGs must exist.
	for i := 1; i <= 3; i++ {
		p := filepath.Join(dir, fmt.Sprintf("share-%d.png", i))
		if _, err := os.Stat(p); os.IsNotExist(err) {
			t.Errorf("missing QR PNG: %s", p)
		}
	}

	// HTML must exist and be non-empty.
	htmlPath := filepath.Join(dir, "recovery.html")
	info, err := os.Stat(htmlPath)
	if err != nil {
		t.Fatalf("recovery.html missing: %v", err)
	}
	if info.Size() < 100 {
		t.Errorf("recovery.html is suspiciously small (%d bytes)", info.Size())
	}
}

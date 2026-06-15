package restore_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	goage "filippo.io/age"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kevinthelago/redoubt/internal/escrow"
	"github.com/kevinthelago/redoubt/internal/restore"
)

// breakGlassEscrow delegates to the real escrow.Reconstruct so tests exercise
// the corvus-ch/shamir GF(2^8) implementation end-to-end.
// The payload stored by escrow.Split is the age identity string itself.
type breakGlassEscrow struct {
	threshold int
	total     int
}

func (e *breakGlassEscrow) Info() (int, int) { return e.threshold, e.total }

// Reconstruct converts [index||data] raw shares back to escrow.Share and
// reconstructs the payload. Interprets the payload bytes as an age identity
// string (the test uses the identity string as the escrow payload directly).
func (e *breakGlassEscrow) Reconstruct(_ context.Context, rawShares [][]byte) (goage.Identity, error) {
	shares := make([]escrow.Share, len(rawShares))
	for i, raw := range rawShares {
		if len(raw) < 2 {
			return nil, escrow.ErrInsufficientShares
		}
		data := raw[1:]
		h := sha256.Sum256(data)
		shares[i] = escrow.Share{
			Index:    int(raw[0]),
			Data:     data,
			Checksum: hex.EncodeToString(h[:]),
		}
	}
	payload, err := escrow.Reconstruct(shares)
	if err != nil {
		return nil, err
	}
	identity, err := goage.ParseX25519Identity(strings.TrimSpace(string(payload)))
	if err != nil {
		return nil, err
	}
	return identity, nil
}

// encodeShare encodes [index||data] as base64 for stdin simulation.
func encodeShare(s escrow.Share) string {
	raw := append([]byte{byte(s.Index)}, s.Data...)
	return base64.StdEncoding.EncodeToString(raw)
}

// TestBreakGlassRoundTrip is the crown-jewel recovery test:
//  1. Generate a fresh age identity.
//  2. Shamir-split the identity string as an escrow payload (2-of-3 threshold).
//  3. Simulate the keystore being unavailable.
//  4. Feed 2 shares through the pipeline's break-glass stdin path.
//  5. Confirm the pipeline unseals a sealed file (proving the reconstructed key works).
func TestBreakGlassRoundTrip(t *testing.T) {
	// ── 1. Generate identity and split ────────────────────────────────────
	original, err := goage.GenerateX25519Identity()
	require.NoError(t, err)

	threshold, total := 2, 3
	result, err := escrow.Split([]byte(original.String()), threshold, total)
	require.NoError(t, err)
	require.Len(t, result.Shares, total)

	// ── 2. Encode shares for the simulated stdin ───────────────────────────
	// Feed shares 0 and 1 (any two of three work — use first two for simplicity).
	stdinInput := encodeShare(result.Shares[0]) + "\n" + encodeShare(result.Shares[1]) + "\n"

	// ── 3. Set up pipeline with unavailable keystore ───────────────────────
	dir := t.TempDir()
	sealedPath := filepath.Join(dir, "secret.txt.age")
	require.NoError(t, os.WriteFile(sealedPath, []byte("sealed-placeholder"), 0600))

	var reconstructedIdentity goage.Identity
	unsealer := &mockUnsealer{
		unsealFn: func(id goage.Identity, src, dst string) error {
			reconstructedIdentity = id
			if err := os.WriteFile(dst, []byte("plain"), 0600); err != nil {
				return err
			}
			return os.Remove(src)
		},
	}

	backend := &mockBackend{
		restoreFn: func(_ context.Context, _, _ string, _ []string, _ bool) error {
			return nil // files already written above
		},
	}

	in := bytes.NewBufferString(stdinInput)
	out := &bytes.Buffer{}

	p := newTestPipeline(t,
		backend,
		unsealer,
		&mockKeys{err: restore.ErrKeystoreUnavailable},
		&breakGlassEscrow{threshold: threshold, total: total},
		&mockBrowser{},
		in,
		out,
	)

	// ── 4. Run and assert ─────────────────────────────────────────────────
	res, err := p.Run(context.Background(), restore.Options{
		SnapshotID: "latest",
		TargetDir:  dir,
		Overwrite:  true, // dir has secret.txt.age pre-written
		NoVerify:   true,
	})
	require.NoError(t, err, "break-glass restore must succeed with threshold shares")

	assert.True(t, res.UsedBreakGlass, "result must flag break-glass was used")
	assert.Equal(t, 1, res.SealedFiles, "the sealed file must have been unsealed")
	assert.NotNil(t, reconstructedIdentity, "unseal must have been called with reconstructed identity")

	outputStr := out.String()
	assert.Contains(t, outputStr, "break-glass")
	assert.Contains(t, outputStr, "Key reconstructed")
}

// TestBreakGlassInsufficientShares verifies a clear "cannot decrypt" error
// (not a crash or an internal Shamir error) when too few shares are given.
func TestBreakGlassInsufficientShares(t *testing.T) {
	threshold, total := 2, 3
	insufficientEscrow := &mockEscrow{
		threshold: threshold,
		total:     total,
		err:       escrow.ErrInsufficientShares,
	}

	// Feed exactly threshold lines so collectShares reads them all.
	stdinInput := base64.StdEncoding.EncodeToString([]byte{1, 0}) + "\n" +
		base64.StdEncoding.EncodeToString([]byte{2, 0}) + "\n"

	in := bytes.NewBufferString(stdinInput)
	out := &bytes.Buffer{}
	dir := t.TempDir()

	p := newTestPipeline(t,
		&mockBackend{},
		&mockUnsealer{},
		&mockKeys{err: restore.ErrKeystoreUnavailable},
		insufficientEscrow,
		&mockBrowser{},
		in,
		out,
	)

	_, err := p.Run(context.Background(), restore.Options{
		SnapshotID: "latest",
		TargetDir:  dir,
		NoVerify:   true,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot decrypt",
		"insufficient shares must yield 'cannot decrypt', not an internal error")
}

// TestBreakGlassWrongShares verifies that structurally valid but content-wrong
// shares produce "cannot decrypt" — not a crash or an internal error.
func TestBreakGlassWrongShares(t *testing.T) {
	threshold, total := 2, 3
	esc := &breakGlassEscrow{threshold: threshold, total: total}

	// Minimal well-formed raw shares that will reconstruct garbage.
	// Index 1 and 2, data bytes all zero.
	fakeShare := func(idx byte) string {
		raw := []byte{idx, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0} // index + 16 zero bytes
		return base64.StdEncoding.EncodeToString(raw)
	}
	stdinInput := fakeShare(1) + "\n" + fakeShare(2) + "\n"

	in := bytes.NewBufferString(stdinInput)
	out := &bytes.Buffer{}
	dir := t.TempDir()

	p := newTestPipeline(t,
		&mockBackend{},
		&mockUnsealer{},
		&mockKeys{err: restore.ErrKeystoreUnavailable},
		esc,
		&mockBrowser{},
		in,
		out,
	)

	_, err := p.Run(context.Background(), restore.Options{
		SnapshotID: "latest",
		TargetDir:  dir,
		NoVerify:   true,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot decrypt",
		"wrong shares must yield 'cannot decrypt', not a panic or internal error")
}

// TestShamirAllPairsReconstruct verifies that any 2-of-3 share pair produces
// the identical secret (the fundamental correctness property of SSS).
func TestShamirAllPairsReconstruct(t *testing.T) {
	original, err := goage.GenerateX25519Identity()
	require.NoError(t, err)

	result, err := escrow.Split([]byte(original.String()), 2, 3)
	require.NoError(t, err)

	pairs := [][2]int{{0, 1}, {0, 2}, {1, 2}}
	for _, pair := range pairs {
		shares := []escrow.Share{result.Shares[pair[0]], result.Shares[pair[1]]}
		payload, err := escrow.Reconstruct(shares)
		require.NoError(t, err, "pair %v should reconstruct", pair)

		reconstructed, err := goage.ParseX25519Identity(strings.TrimSpace(string(payload)))
		require.NoError(t, err, "reconstructed payload must parse as age identity")
		assert.Equal(t, original.String(), reconstructed.String(),
			"reconstructed identity must match original for pair %v", pair)
	}
}

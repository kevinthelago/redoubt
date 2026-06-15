package restore

import (
	"bufio"
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	goage "filippo.io/age"
)

// collectShares interactively collects Shamir shares from the operator and
// reconstructs the master age identity. It shows a threshold indicator so the
// operator knows exactly how many shares are needed.
func (p *Pipeline) collectShares(ctx context.Context) (goage.Identity, error) {
	threshold, total := p.escrow.Info()

	fmt.Fprintf(p.out, "\nRequired: %d of %d shares\n", threshold, total)
	fmt.Fprintln(p.out, "Paste each share (base64) and press Enter. Shares can be read from a")
	fmt.Fprintln(p.out, "USB drive or printed recovery sheet.")
	fmt.Fprintln(p.out)

	scanner := bufio.NewScanner(p.in)
	shares := make([][]byte, 0, threshold)

	for i := 0; i < threshold; i++ {
		fmt.Fprintf(p.out, "Enter share %d/%d: ", i+1, threshold)
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return nil, fmt.Errorf("reading share: %w", err)
			}
			return nil, fmt.Errorf("unexpected end of input while reading shares")
		}
		raw := strings.TrimSpace(scanner.Text())
		if raw == "" {
			return nil, fmt.Errorf("share %d/%d is empty", i+1, threshold)
		}
		shareBytes, err := base64.StdEncoding.DecodeString(raw)
		if err != nil {
			return nil, fmt.Errorf("share %d/%d: invalid base64: %w", i+1, threshold, err)
		}
		shares = append(shares, shareBytes)
		fmt.Fprintf(p.out, "Share %d accepted.\n", i+1)
	}

	fmt.Fprintln(p.out, "\nReconstructing key from shares...")
	identity, err := p.escrow.Reconstruct(ctx, shares)
	if err != nil {
		// Keep the message human-readable; never reveal what was wrong with the
		// shares beyond "cannot decrypt" — same message for wrong share and
		// insufficient shares, to avoid oracle attacks.
		return nil, fmt.Errorf("cannot decrypt: %w", err)
	}
	fmt.Fprintln(p.out, "Key reconstructed successfully.")
	return identity, nil
}

// EncodeShare base64-encodes a raw share for display / storage.
func EncodeShare(raw []byte) string {
	return base64.StdEncoding.EncodeToString(raw)
}

// DecodeShare base64-decodes a share string into raw bytes.
func DecodeShare(s string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(strings.TrimSpace(s))
}

// Package escrow — recovery.go generates the printable break-glass recovery sheet.
//
// Each share is rendered as:
//   - A QR code PNG (for scanning)
//   - A base64 text representation (for manual entry)
//
// A companion HTML file bundles all shares into a single printable page with
// embedded QR codes and step-by-step restore instructions.
package escrow

import (
	"encoding/base64"
	"fmt"
	"html/template"
	"io"
	"os"
	"path/filepath"
	"time"

	qrcode "github.com/skip2/go-qrcode"
)

// RecoverySheetOptions controls the output of WriteRecoverySheet.
type RecoverySheetOptions struct {
	// OutputDir is the directory where QR PNG files and the HTML sheet are written.
	// The directory must already exist.
	OutputDir string
	// Threshold is the minimum number of shares required to reconstruct.
	Threshold int
	// Total is the total number of shares generated.
	Total int
	// QRSize is the pixel size of each QR code image. Defaults to 512.
	QRSize int
}

// sheetShare is the per-share data passed to the HTML template.
type sheetShare struct {
	Index     int
	B64       string       // base64-encoded share data (the value users enter manually)
	Checksum  string       // full SHA-256 hex
	Short     string       // first 16 chars for quick visual check
	QRDataURI template.URL // inline PNG data URI; marked safe for template rendering
}

// sheetData is passed to the HTML template.
type sheetData struct {
	Threshold int
	Total     int
	Timestamp string
	Shares    []sheetShare
}

// WriteRecoverySheet generates per-share QR code PNGs and a single printable
// HTML recovery sheet in opts.OutputDir. The HTML file is named "recovery.html".
func WriteRecoverySheet(shares []Share, opts RecoverySheetOptions) error {
	if opts.QRSize == 0 {
		opts.QRSize = 512
	}

	data := sheetData{
		Threshold: opts.Threshold,
		Total:     opts.Total,
		Timestamp: time.Now().UTC().Format("2006-01-02 15:04 UTC"),
	}

	for pos, s := range shares {
		b64 := base64.StdEncoding.EncodeToString(s.Data)

		// Use sequential position (1-based) for filenames so the output is always
		// share-1.png, share-2.png, ... regardless of the internal Shamir x-coordinate.
		qrFilename := fmt.Sprintf("share-%d.png", pos+1)
		qrPath := filepath.Join(opts.OutputDir, qrFilename)
		if err := qrcode.WriteFile(b64, qrcode.Medium, opts.QRSize, qrPath); err != nil {
			return fmt.Errorf("escrow: write QR for share %d: %w", s.Index, err)
		}

		// Read PNG back for inline data URI embedding.
		pngBytes, err := os.ReadFile(qrPath)
		if err != nil {
			return fmt.Errorf("escrow: read QR PNG for share %d: %w", s.Index, err)
		}
		dataURI := template.URL("data:image/png;base64," + base64.StdEncoding.EncodeToString(pngBytes))

		data.Shares = append(data.Shares, sheetShare{
			Index:     pos + 1, // display number, 1-based sequential
			B64:       b64,
			Checksum:  s.Checksum,
			Short:     s.Checksum[:16],
			QRDataURI: dataURI,
		})
	}

	// Write HTML sheet.
	htmlPath := filepath.Join(opts.OutputDir, "recovery.html")
	f, err := os.OpenFile(htmlPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("escrow: create recovery sheet: %w", err)
	}
	defer f.Close()

	return renderSheet(f, data)
}

func renderSheet(w io.Writer, data sheetData) error {
	return sheetTmpl.Execute(w, data)
}

var sheetTmpl = template.Must(template.New("sheet").Parse(sheetHTML))

const sheetHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>Redoubt Break-Glass Recovery Sheet</title>
<style>
  body { font-family: monospace; max-width: 900px; margin: 0 auto; padding: 2em; color: #111; }
  h1 { font-size: 1.4em; border-bottom: 2px solid #333; padding-bottom: 0.3em; }
  .meta { background: #f5f5f5; padding: 1em; border: 1px solid #ccc; margin-bottom: 2em; }
  .share { border: 2px solid #555; padding: 1em; margin-bottom: 2em; page-break-inside: avoid; }
  .share h2 { margin: 0 0 0.5em; }
  .qr { text-align: center; margin: 1em 0; }
  .qr img { max-width: 300px; border: 1px solid #999; }
  .data { word-break: break-all; background: #eee; padding: 0.5em; font-size: 0.85em; }
  .checksum { font-size: 0.8em; color: #555; margin-top: 0.3em; }
  .steps { margin-top: 2em; padding: 1em; background: #fffbe6; border: 1px solid #e6c800; }
  .steps ol li { margin: 0.5em 0; }
  @media print { .steps { page-break-before: always; } }
</style>
</head>
<body>
<h1>Redoubt Break-Glass Recovery Sheet</h1>

<div class="meta">
  <strong>Generated:</strong> {{.Timestamp}}<br>
  <strong>Scheme:</strong> {{.Threshold}}-of-{{.Total}} Shamir Secret Sharing<br>
  <strong>STORE EACH SHARE IN A SEPARATE SECURE LOCATION.</strong>
  Any {{.Threshold}} of these {{.Total}} shares can reconstruct the master key.
</div>

{{range .Shares}}
<div class="share">
  <h2>Share {{.Index}} of {{len $.Shares}}</h2>
  <div class="qr">
    <img src="{{.QRDataURI}}" alt="QR code for share {{.Index}}">
    <p>Scan with a QR reader, or enter the text below manually.</p>
  </div>
  <div class="data">{{.B64}}</div>
  <div class="checksum">
    SHA-256 (first 16): <strong>{{.Short}}</strong><br>
    Full: {{.Checksum}}
  </div>
</div>
{{end}}

<div class="steps">
  <h2>Recovery Instructions</h2>
  <ol>
    <li>Obtain at least <strong>{{.Threshold}}</strong> of these {{.Total}} shares
        (scan QR codes or type the base64 text into a file).</li>
    <li>Install Redoubt on the recovery machine:
        <code>go install github.com/kevinthelago/redoubt/cmd/redoubt@latest</code></li>
    <li>For each share, save its base64 text to a file:
        <code>share-1.txt</code>, <code>share-2.txt</code>, etc.</li>
    <li>Run: <code>redoubt key reconstruct --shares share-1.txt,share-2.txt</code></li>
    <li>Redoubt will reconstruct the master key, prompt for a new passphrase,
        and re-initialize the OS keystore.</li>
    <li>Connect to the vault: <code>redoubt vault status</code></li>
    <li>Restore files: <code>redoubt restore --snapshot latest --target /tmp/restore</code></li>
  </ol>
  <p><em>
    The SHA-256 checksum lets you verify share integrity before reconstruction.
    A corrupted share will be rejected with a "checksum mismatch" error.
  </em></p>
</div>
</body>
</html>
`

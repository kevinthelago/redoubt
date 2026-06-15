package backup

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// sealSecretsAsset age-seals all files in a secrets-category asset into a
// mirrored directory tree under tmpDir. The mirrored paths carry a ".age"
// suffix so the restore stream knows to unseal them.
//
// Returns the sealed directory path; the caller owns cleanup.
// The original secret files are never read by this package — the Sealer
// implementation handles all I/O on the plaintext side.
func sealSecretsAsset(
	ctx context.Context,
	a TrackedAsset,
	tmpDir string,
	sealer Sealer,
	log Logger,
) (string, error) {
	sealedDir := filepath.Join(tmpDir, "sealed-"+sanitizeName(a.ID))
	if err := os.MkdirAll(sealedDir, 0o700); err != nil {
		return "", fmt.Errorf("create sealed directory for asset %q: %w", a.ID, err)
	}

	var count int
	for _, path := range a.Paths {
		n, err := sealPath(ctx, path, sealedDir, sealer)
		if err != nil {
			return "", fmt.Errorf("seal path for asset %q: %w", a.ID, err)
		}
		count += n
	}

	// Log count only — no paths, no content. Path names for secrets assets may
	// themselves be considered sensitive metadata.
	log.Info("sealed secrets asset", "asset", a.ID, "files", count)
	return sealedDir, nil
}

// sealPath seals one file or directory tree into sealedDir, maintaining the
// relative directory structure and appending ".age" to each sealed file name.
// Returns the count of sealed files.
func sealPath(ctx context.Context, src, sealedDir string, sealer Sealer) (int, error) {
	info, err := os.Stat(src)
	if err != nil {
		return 0, fmt.Errorf("stat: %w", err)
	}

	if !info.IsDir() {
		dst := filepath.Join(sealedDir, info.Name()+".age")
		if err := sealer.SealFile(ctx, src, dst); err != nil {
			return 0, fmt.Errorf("seal file: %w", err)
		}
		return 1, nil
	}

	var count int
	err = filepath.WalkDir(src, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return fmt.Errorf("compute relative path: %w", err)
		}

		dst := filepath.Join(sealedDir, rel+".age")
		if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
			return fmt.Errorf("create parent dir for sealed file: %w", err)
		}

		if err := sealer.SealFile(ctx, path, dst); err != nil {
			return fmt.Errorf("seal %s: %w", rel, err)
		}

		count++
		return nil
	})
	if err != nil {
		return count, err
	}

	return count, nil
}

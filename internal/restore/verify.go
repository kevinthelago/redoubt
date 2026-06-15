package restore

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	goage "filippo.io/age"
)

// verify compares the restored tree against the snapshot's file manifest.
// It is a host-side second pass after restic's built-in --verify, providing
// defence-in-depth: restic verifies content against the pack store; this check
// confirms every expected file actually landed on disk in the right shape.
//
// For files that were age-sealed in the snapshot (*.age) and have since been
// unsealed to their plain counterpart, we skip the size check for the plain
// version (its size will differ from the sealed copy).
func (p *Pipeline) verify(
	targetDir string,
	expectedFiles []FileEntry,
	_ goage.Identity, // reserved: future encrypted-hash verification
) ([]VerifyError, error) {
	p.log.Info("post-restore verification", "target", targetDir, "expected", len(expectedFiles))

	// Build index: relative path → entry.
	type entry struct {
		size int64
		kind string
	}
	index := make(map[string]entry, len(expectedFiles))
	for _, e := range expectedFiles {
		index[normalizePath(e.Path)] = entry{size: e.Size, kind: e.Type}
	}

	var errs []VerifyError

	err := filepath.WalkDir(targetDir, func(path string, d os.DirEntry, werr error) error {
		if werr != nil {
			errs = append(errs, VerifyError{Path: path, Message: werr.Error()})
			return nil
		}
		if d.IsDir() {
			return nil
		}

		rel, err := filepath.Rel(targetDir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)

		exp, inSnapshot := index[rel]
		if !inSnapshot {
			// Not in snapshot — could be the unsealed counterpart of a *.age entry.
			// If path+".age" is in the index, this is an unsealed secret; not an error.
			if _, hasSealedVersion := index[rel+".age"]; !hasSealedVersion {
				p.log.Debug("extra file in target not in snapshot", "path", rel)
			}
			return nil
		}

		// Size check for plain files only — skip for directories and symlinks.
		if exp.kind == "file" && exp.size > 0 {
			info, err := d.Info()
			if err != nil {
				errs = append(errs, VerifyError{Path: rel, Message: fmt.Sprintf("stat: %v", err)})
				return nil
			}
			// The unsealed version of a sealed file will have a different size
			// than the .age-encrypted original; skip the size check for it.
			if !strings.HasSuffix(rel, ".age") {
				if _, hasSealedVersion := index[rel+".age"]; hasSealedVersion {
					// This is the unsealed version of a sealed entry — skip size check.
					delete(index, rel) // mark visited so it doesn't appear as missing
					return nil
				}
			}
			if info.Size() != exp.size {
				errs = append(errs, VerifyError{
					Path:    rel,
					Message: fmt.Sprintf("size mismatch: expected %d B, got %d B", exp.size, info.Size()),
				})
			}
		}

		delete(index, rel) // mark as found
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk target: %w", err)
	}

	// Anything remaining in the index is missing from the target.
	for rel, exp := range index {
		if exp.kind == "file" {
			errs = append(errs, VerifyError{
				Path:    rel,
				Message: "file missing from target directory after restore",
			})
		}
	}

	if len(errs) == 0 {
		p.log.Info("verification passed")
	} else {
		p.log.Error("verification mismatches", slog.Int("count", len(errs)))
	}
	return errs, nil
}

// normalizePath converts OS separators to forward slashes and strips any
// leading slash so paths from restic (which uses Unix-style) compare equal
// to paths produced by filepath.Rel (which uses OS separators).
func normalizePath(p string) string {
	p = filepath.ToSlash(p)
	return strings.TrimPrefix(p, "/")
}

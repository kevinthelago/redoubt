package assets

import (
	"bufio"
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
)

// Rule is a single secret-detection rule.
type Rule struct {
	ID          string
	Description string
	re          *regexp.Regexp
}

// ScanResult reports a potential secret found in a file.
// The matched value is intentionally omitted — only location and rule are surfaced.
type ScanResult struct {
	AssetPath   string // the tracked asset path this result belongs to
	File        string // absolute path of the file containing the match
	Line        int
	RuleID      string
	Description string
}

// Scanner detects likely secrets in files using a set of rules.
type Scanner struct {
	rules []Rule
}

// DefaultScanner returns a Scanner loaded with the built-in gitleaks-compatible ruleset.
func DefaultScanner() *Scanner {
	return &Scanner{rules: builtinRules}
}

// ScanAssets runs the scanner across all non-secrets assets, applying ex to skip
// excluded paths. Assets tagged CategorySecrets are skipped entirely.
//
// Returns (nil, nil) when no assets match or no findings are detected.
func (s *Scanner) ScanAssets(items []Asset, ex *Excluder) ([]ScanResult, error) {
	var results []ScanResult
	for _, a := range items {
		if a.Category == CategorySecrets {
			continue
		}
		found, err := s.ScanAsset(a, ex)
		if err != nil {
			return nil, fmt.Errorf("scanning %s: %w", a.Path, err)
		}
		results = append(results, found...)
	}
	return results, nil
}

// ScanAsset scans a single asset's path for secret-like patterns.
// Directories are walked recursively. Excluded paths are skipped.
func (s *Scanner) ScanAsset(a Asset, ex *Excluder) ([]ScanResult, error) {
	info, err := os.Stat(a.Path)
	if err != nil {
		return nil, err
	}

	// Merge asset-local .redoubtignore into the global excluder.
	localEx, _ := LoadAssetExcludes(a.Path)
	effective := ex
	if localEx != nil && len(localEx.patterns) > 0 {
		if effective != nil {
			effective = effective.Merge(localEx)
		} else {
			effective = localEx
		}
	}

	var results []ScanResult

	if info.IsDir() {
		err = filepath.WalkDir(a.Path, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil || d.IsDir() {
				return nil
			}
			rel, _ := filepath.Rel(a.Path, path)
			rel = filepath.ToSlash(rel)
			if effective != nil && effective.IsExcluded(rel) {
				return nil
			}
			found, err := s.scanFile(path)
			if err != nil {
				return nil // skip unreadable files silently
			}
			for i := range found {
				found[i].AssetPath = a.Path
			}
			results = append(results, found...)
			return nil
		})
	} else {
		if effective == nil || !effective.IsExcluded(filepath.Base(a.Path)) {
			found, err := s.scanFile(a.Path)
			if err == nil {
				for i := range found {
					found[i].AssetPath = a.Path
				}
				results = append(results, found...)
			}
		}
	}

	return results, err
}

// scanFile checks a single file for secret-like patterns line by line.
// Binary files (detected by a null byte in the first 512 bytes) are silently skipped.
func (s *Scanner) scanFile(path string) ([]ScanResult, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// Binary detection.
	header := make([]byte, 512)
	n, _ := f.Read(header)
	if bytes.IndexByte(header[:n], 0) >= 0 {
		return nil, nil
	}
	if _, err := f.Seek(0, 0); err != nil {
		return nil, err
	}

	var results []ScanResult
	sc := bufio.NewScanner(f)
	lineNum := 0
	for sc.Scan() {
		lineNum++
		line := sc.Bytes()
		for _, rule := range s.rules {
			if rule.re.Match(line) {
				results = append(results, ScanResult{
					File:        path,
					Line:        lineNum,
					RuleID:      rule.ID,
					Description: rule.Description,
				})
				break // first matching rule per line
			}
		}
	}
	return results, sc.Err()
}

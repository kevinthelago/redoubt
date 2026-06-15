package assets

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// Excluder matches file paths against gitignore-style patterns.
// Patterns are applied in order; the last match wins.
// Lines beginning with # are comments; lines beginning with ! negate a prior match.
type Excluder struct {
	patterns []excludePattern
}

type excludePattern struct {
	raw    string
	negate bool
}

// NewExcluder compiles an Excluder from a slice of raw gitignore-style pattern lines.
func NewExcluder(lines []string) *Excluder {
	e := &Excluder{}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		neg := strings.HasPrefix(line, "!")
		if neg {
			line = line[1:]
		}
		e.patterns = append(e.patterns, excludePattern{raw: line, negate: neg})
	}
	return e
}

// LoadExcludeFile reads a gitignore-style file (e.g. .redoubtignore) and returns
// an Excluder for its contents. A missing file returns an empty Excluder.
func LoadExcludeFile(path string) (*Excluder, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Excluder{}, nil
		}
		return nil, err
	}
	defer f.Close()

	var lines []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return NewExcluder(lines), nil
}

// LoadDataDirExcludes loads the global .redoubtignore from dataDir.
// Errors loading the file are returned; a missing file is not an error.
func LoadDataDirExcludes(dataDir string) (*Excluder, error) {
	return LoadExcludeFile(filepath.Join(dataDir, ".redoubtignore"))
}

// LoadAssetExcludes loads a per-asset .redoubtignore located in the root of
// the asset path (if it is a directory).
func LoadAssetExcludes(assetPath string) (*Excluder, error) {
	return LoadExcludeFile(filepath.Join(assetPath, ".redoubtignore"))
}

// Merge returns a new Excluder that combines the patterns of both receivers.
// Patterns from other are appended after e, so they can override earlier ones.
func (e *Excluder) Merge(other *Excluder) *Excluder {
	merged := &Excluder{}
	merged.patterns = append(merged.patterns, e.patterns...)
	merged.patterns = append(merged.patterns, other.patterns...)
	return merged
}

// IsExcluded reports whether path matches the exclusion rules.
// path should be a slash-separated path relative to the asset root.
// The last matching pattern wins; a negation pattern un-excludes a path.
func (e *Excluder) IsExcluded(path string) bool {
	path = filepath.ToSlash(path)
	excluded := false
	for _, p := range e.patterns {
		if matchGlob(p.raw, path) {
			excluded = !p.negate
		}
	}
	return excluded
}

// matchGlob reports whether path matches the gitignore-style glob pat.
//
// Rules:
//   - If pat contains no slash, match against the base name only.
//   - ** matches any sequence of path segments (including none).
//   - * matches any sequence except /.
//   - ? matches any single character except /.
func matchGlob(pat, path string) bool {
	pat = filepath.ToSlash(pat)

	// No slash in pattern → match only the base name.
	if !strings.Contains(pat, "/") {
		base := path
		if idx := strings.LastIndex(path, "/"); idx >= 0 {
			base = path[idx+1:]
		}
		ok, _ := filepath.Match(pat, base)
		return ok
	}

	// Pattern ends with / → match directories (treat as prefix).
	if strings.HasSuffix(pat, "/") {
		return strings.HasPrefix(path+"/", pat) || strings.HasPrefix(path, pat)
	}

	// Handle ** with a recursive segment-by-segment match.
	if strings.Contains(pat, "**") {
		return matchDoubleGlob(pat, path)
	}

	ok, _ := filepath.Match(pat, path)
	return ok
}

// matchDoubleGlob handles patterns that contain **.
// It expands ** to match zero or more path segments.
func matchDoubleGlob(pat, path string) bool {
	// Split on ** and try all intermediate path lengths.
	parts := strings.SplitN(pat, "**", 2)
	prefix, suffix := parts[0], parts[1]

	suffix = strings.TrimPrefix(suffix, "/")

	segments := strings.Split(path, "/")
	for i := 0; i <= len(segments); i++ {
		head := strings.Join(segments[:i], "/")
		tail := strings.Join(segments[i:], "/")

		headOK := prefix == "" || strings.HasPrefix(head+"/", prefix) || head == strings.TrimSuffix(prefix, "/")
		if prefix == "" {
			headOK = true
		}
		var tailOK bool
		if suffix == "" {
			tailOK = true
		} else {
			tailOK, _ = filepath.Match(suffix, tail)
			if !tailOK {
				tailOK = tail == suffix
			}
		}
		if headOK && tailOK {
			return true
		}
	}
	return false
}

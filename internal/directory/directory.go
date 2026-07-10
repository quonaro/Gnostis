package directory

import (
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/quonaro/gnostis/internal/config"
)

// Directory holds effective indexing rules for a single root.
type Directory struct {
	config.Directory
	effectiveExtensions []string
	effectiveExcludes   []string
}

// FromConfig merges directory-specific settings with global defaults.
func FromConfig(idx config.Index, dir config.Directory) Directory {
	extensions := dir.Extensions
	if len(extensions) == 0 {
		extensions = idx.DefaultExtensions
	}

	excludes := make([]string, 0, len(idx.DefaultExcludePatterns)+len(dir.Exclude))
	excludes = append(excludes, idx.DefaultExcludePatterns...)
	excludes = append(excludes, dir.Exclude...)

	return Directory{
		Directory:           dir,
		effectiveExtensions: normalizeExtensions(extensions),
		effectiveExcludes:   excludes,
	}
}

// ShouldIndex reports whether a file should be indexed.
// relPath is relative to the directory root; sizeBytes is the file size.
func (d Directory) ShouldIndex(relPath string, sizeBytes int64) bool {
	lower := strings.ToLower(relPath)

	if len(d.Include) > 0 {
		matched := false
		for _, pattern := range d.Include {
			if matchPattern(pattern, lower) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	for _, pattern := range d.effectiveExcludes {
		if matchPattern(pattern, lower) {
			return false
		}
	}

	if !d.hasAllowedExtension(relPath) {
		return false
	}

	maxBytes := int64(d.MaxFileSizeMB) * 1024 * 1024
	if maxBytes > 0 && sizeBytes > maxBytes {
		return false
	}

	return true
}

func (d Directory) hasAllowedExtension(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	if ext == "" {
		return false
	}
	for _, allowed := range d.effectiveExtensions {
		if ext == allowed {
			return true
		}
	}
	return false
}

func matchPattern(pattern, path string) bool {
	matched, err := doublestar.Match(pattern, path)
	if err != nil {
		return false
	}
	return matched
}

func normalizeExtensions(exts []string) []string {
	out := make([]string, 0, len(exts))
	for _, ext := range exts {
		out = append(out, strings.ToLower(ext))
	}
	return out
}

package mcp

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

func (s *Server) isPathAllowed(path string) bool {
	if path == "" {
		return false
	}
	clean := filepath.Clean(path)
	for _, p := range s.projects {
		rel, err := filepath.Rel(p.Path, clean)
		if err != nil {
			continue
		}
		if rel == "." || !strings.HasPrefix(rel, "..") {
			return true
		}
	}
	return false
}

func (s *Server) resolvePath(project, path string) (string, error) {
	var base string
	if project != "" {
		for _, p := range s.projects {
			if p.Name == project {
				base = p.Path
				break
			}
		}
		if base == "" {
			return "", fmt.Errorf("project %q not found", project)
		}
	}

	if path == "" {
		if base == "" {
			return "", fmt.Errorf("project or path is required")
		}
		path = base
	} else if base != "" && !filepath.IsAbs(path) {
		path = filepath.Join(base, path)
	}

	clean := filepath.Clean(path)
	if !s.isPathAllowed(clean) {
		return "", fmt.Errorf("path %q is outside indexed projects", clean)
	}
	return clean, nil
}

func globFiles(root, pattern string) ([]string, error) {
	if pattern == "" {
		pattern = "*"
	}
	if !doublestar.ValidatePattern(pattern) {
		return nil, fmt.Errorf("invalid glob pattern: %s", pattern)
	}
	matches, err := doublestar.Glob(os.DirFS(root), pattern)
	if err != nil {
		return nil, fmt.Errorf("glob: %w", err)
	}
	// Prefix matches with root to return absolute paths.
	for i, m := range matches {
		matches[i] = filepath.Join(root, m)
	}
	return matches, nil
}

func isTextFile(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()

	buf := make([]byte, 512)
	n, err := f.Read(buf)
	if err != nil && !errors.Is(err, io.EOF) {
		return false
	}
	return !strings.Contains(string(buf[:n]), "\x00")
}

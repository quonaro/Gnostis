package mcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/mark3labs/mcp-go/mcp"
)

// Human-readable error reason codes used by toolError.
const (
	errReasonInvalidArgument  = "invalid_argument"
	errReasonProjectNotFound  = "project_not_found"
	errReasonPathNotAllowed   = "path_not_allowed"
	errReasonPathNotFound     = "path_not_found"
	errReasonNotConfigured    = "not_configured"
	errReasonNotFound         = "not_found"
	errReasonReadFailed       = "read_failed"
	errReasonInvalidRegex     = "invalid_regex"
	errReasonSearchFailed     = "search_failed"
	errReasonMemoryNotEnabled = "memory_not_enabled"
	errReasonIndexNotReady    = "index_not_ready"
)

func (s *Server) isPathAllowed(path string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.isPathAllowedLocked(path)
}

func (s *Server) isPathAllowedLocked(path string) bool {
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

func (s *Server) resolveAbsolutePath(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("path is required")
	}
	clean := filepath.Clean(path)
	if !filepath.IsAbs(clean) {
		return "", fmt.Errorf("path must be absolute: %s", clean)
	}
	return clean, nil
}

func (s *Server) resolvePath(project, path string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

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
	if !s.isPathAllowedLocked(clean) {
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

// toolErrorResponse is the JSON body returned by toolError.
type toolErrorResponse struct {
	Error      bool   `json:"error"`
	Reason     string `json:"reason"`
	Message    string `json:"message"`
	Suggestion string `json:"suggestion,omitempty"`
}

// toolError returns a structured, human-readable MCP error result.
func toolError(reason, message, suggestion string) *mcp.CallToolResult {
	data, _ := json.Marshal(toolErrorResponse{
		Error:      true,
		Reason:     reason,
		Message:    message,
		Suggestion: suggestion,
	})
	return mcp.NewToolResultError(string(data))
}

// toolErrorFromResolvePath maps a resolvePath error to a structured error.
func toolErrorFromResolvePath(err error) *mcp.CallToolResult {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "project") && strings.Contains(msg, "not found"):
		return toolError(errReasonProjectNotFound, msg, "use mcp2_list_projects to see available projects")
	case strings.Contains(msg, "outside indexed projects"):
		return toolError(errReasonPathNotAllowed, msg, "use fs_* tools for non-project paths or add the project to the index")
	default:
		return toolError(errReasonInvalidArgument, msg, "provide a valid project name or absolute path")
	}
}

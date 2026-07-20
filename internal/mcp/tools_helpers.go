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
	"github.com/quonaro/gnostis/internal/progress"
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

// resolvePathOrAbsolute tries resolvePath first, then falls back to
// resolveAbsolutePath when the path is not within an indexed project but is
// a valid absolute path. This avoids the chicken-and-egg problem where a
// project must be indexed before its paths can be used with project-scoped
// tools.
func (s *Server) resolvePathOrAbsolute(project, path string) (string, error) {
	resolved, err := s.resolvePath(project, path)
	if err == nil {
		return resolved, nil
	}
	msg := err.Error()
	isPathNotAllowed := strings.Contains(msg, "outside indexed projects")
	isProjectNotFound := strings.Contains(msg, "not found") && strings.Contains(msg, "project")
	if (isPathNotAllowed || isProjectNotFound) && filepath.IsAbs(filepath.Clean(path)) {
		if abs, absErr := s.resolveAbsolutePath(path); absErr == nil {
			return abs, nil
		}
	}
	return "", err
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

// indexNotReadyError checks if the index is still being built and returns an
// index_not_ready error. It returns nil when indexing is not in progress,
// allowing the caller to return normal (possibly empty) results.
func (s *Server) indexNotReadyError() *mcp.CallToolResult {
	if s.indexer == nil {
		return nil
	}
	pstate, err := s.indexer.ProgressState()
	if err != nil {
		return nil
	}
	if pstate.Status == progress.StatusRunning {
		return toolError(errReasonIndexNotReady,
			"index is still being built, no results yet",
			"use get_index_status to check progress and try again later")
	}
	return nil
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

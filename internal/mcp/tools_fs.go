package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

type listFilesArgs struct {
	Project     string `json:"project"`
	Path        string `json:"path"`
	Pattern     string `json:"pattern"`
	IncludeDirs bool   `json:"include_dirs"`
}

type directoryTreeArgs struct {
	Project string  `json:"project"`
	Path    string  `json:"path"`
	Depth   float64 `json:"depth"`
}

type getRecentChangesArgs struct {
	Project string  `json:"project"`
	Path    string  `json:"path"`
	Minutes float64 `json:"minutes"`
}

type fileEntry struct {
	Path string `json:"path"`
}

type treeEntry struct {
	Path     string      `json:"path"`
	Type     string      `json:"type"`
	Children []treeEntry `json:"children,omitempty"`
}

type recentChange struct {
	Path    string `json:"path"`
	ModTime string `json:"mod_time"`
}

func (s *Server) listFiles(ctx context.Context, request mcp.CallToolRequest, args listFilesArgs) (*mcp.CallToolResult, error) {
	slog.InfoContext(ctx, "mcp tool call", "tool", "list_files", "project", args.Project, "path", args.Path, "pattern", args.Pattern)

	root, err := s.resolvePath(args.Project, args.Path)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	files, err := globFiles(root, args.Pattern)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	if !args.IncludeDirs {
		filtered := make([]string, 0, len(files))
		for _, f := range files {
			info, statErr := os.Stat(f)
			if statErr != nil || info.IsDir() {
				continue
			}
			filtered = append(filtered, f)
		}
		files = filtered
	}

	entries := make([]fileEntry, len(files))
	for i, f := range files {
		entries[i] = fileEntry{Path: f}
	}

	data, err := json.Marshal(entries)
	if err != nil {
		return nil, fmt.Errorf("marshal files: %w", err)
	}
	return mcp.NewToolResultText(string(data)), nil
}

func (s *Server) directoryTree(ctx context.Context, request mcp.CallToolRequest, args directoryTreeArgs) (*mcp.CallToolResult, error) {
	slog.InfoContext(ctx, "mcp tool call", "tool", "directory_tree", "project", args.Project, "path", args.Path, "depth", args.Depth)

	root, err := s.resolvePath(args.Project, args.Path)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	depth := int(args.Depth)
	if depth <= 0 {
		depth = 3
	}

	tree, err := buildTree(root, depth)
	if err != nil {
		return nil, fmt.Errorf("build tree: %w", err)
	}

	data, err := json.Marshal(tree)
	if err != nil {
		return nil, fmt.Errorf("marshal tree: %w", err)
	}
	return mcp.NewToolResultText(string(data)), nil
}

func buildTree(root string, depth int) (treeEntry, error) {
	info, err := os.Stat(root)
	if err != nil {
		return treeEntry{}, err
	}
	entry := treeEntry{Path: root, Type: "dir"}
	if depth <= 0 || !info.IsDir() {
		return entry, nil
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return entry, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, e := range entries {
		childPath := filepath.Join(root, e.Name())
		if e.IsDir() {
			child, err := buildTree(childPath, depth-1)
			if err == nil {
				entry.Children = append(entry.Children, child)
			}
		} else {
			entry.Children = append(entry.Children, treeEntry{Path: childPath, Type: "file"})
		}
	}
	return entry, nil
}

func listFilesTool() mcp.Tool {
	return mcp.NewTool("list_files",
		mcp.WithDescription("List files in a project directory"),
		mcp.WithString("project", mcp.Description("Project name")),
		mcp.WithString("path", mcp.Description("Relative path within the project")),
		mcp.WithString("pattern", mcp.Description("Glob pattern, e.g. *.go"), mcp.DefaultString("*")),
		mcp.WithBoolean("include_dirs", mcp.Description("Include directories in results"), mcp.DefaultBool(false)),
	)
}

func directoryTreeTool() mcp.Tool {
	return mcp.NewTool("directory_tree",
		mcp.WithDescription("Return the directory tree up to a given depth"),
		mcp.WithString("project", mcp.Description("Project name")),
		mcp.WithString("path", mcp.Description("Relative path within the project")),
		mcp.WithNumber("depth", mcp.Description("Maximum depth"), mcp.DefaultNumber(3)),
	)
}

func getRecentChangesTool() mcp.Tool {
	return mcp.NewTool("get_recent_changes",
		mcp.WithDescription("List files modified within the last N minutes"),
		mcp.WithString("project", mcp.Description("Project name")),
		mcp.WithString("path", mcp.Description("Relative path within the project")),
		mcp.WithNumber("minutes", mcp.Description("Time window in minutes"), mcp.DefaultNumber(60)),
	)
}

func (s *Server) getRecentChanges(ctx context.Context, request mcp.CallToolRequest, args getRecentChangesArgs) (*mcp.CallToolResult, error) {
	slog.InfoContext(ctx, "mcp tool call", "tool", "get_recent_changes", "project", args.Project, "path", args.Path, "minutes", args.Minutes)

	root, err := s.resolvePath(args.Project, args.Path)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	minutes := int(args.Minutes)
	if minutes <= 0 {
		minutes = 60
	}
	cutoff := time.Now().Add(-time.Duration(minutes) * time.Minute)

	var changes []recentChange
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() {
			return nil
		}
		info, walkErr := d.Info()
		if walkErr != nil || !info.ModTime().After(cutoff) {
			return nil
		}
		changes = append(changes, recentChange{
			Path:    path,
			ModTime: info.ModTime().Format(time.RFC3339),
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk recent changes: %w", err)
	}

	data, err := json.Marshal(changes)
	if err != nil {
		return nil, fmt.Errorf("marshal changes: %w", err)
	}
	return mcp.NewToolResultText(string(data)), nil
}

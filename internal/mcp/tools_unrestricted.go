package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

type fsReadArgs struct {
	Path      string  `json:"path"`
	StartLine float64 `json:"start_line"`
	EndLine   float64 `json:"end_line"`
}

type fsGrepArgs struct {
	Query string  `json:"query"`
	Path  string  `json:"path"`
	Regex bool    `json:"regex"`
	TopK  float64 `json:"top_k"`
}

type fsListArgs struct {
	Path        string `json:"path"`
	Pattern     string `json:"pattern"`
	IncludeDirs bool   `json:"include_dirs"`
}

type fsTreeArgs struct {
	Path  string  `json:"path"`
	Depth float64 `json:"depth"`
}

func fsReadTool() mcp.Tool {
	return mcp.NewTool("fs_read",
		mcp.WithDescription("Read a file or a range of lines from any absolute path"),
		mcp.WithString("path", mcp.Required(), mcp.Description("Absolute file path")),
		mcp.WithNumber("start_line", mcp.Description("First line (1-based)")),
		mcp.WithNumber("end_line", mcp.Description("Last line (1-based)")),
	)
}

func (s *Server) fsRead(ctx context.Context, request mcp.CallToolRequest, args fsReadArgs) (*mcp.CallToolResult, error) {
	slog.InfoContext(ctx, "mcp tool call", "tool", "fs_read", "path", args.Path, "start_line", args.StartLine, "end_line", args.EndLine)
	if args.Path == "" {
		return toolError(errReasonInvalidArgument, "path is required", "provide an absolute file path"), nil
	}

	clean, err := s.resolveAbsolutePath(args.Path)
	if err != nil {
		return toolError(errReasonInvalidArgument, err.Error(), "provide an absolute file path"), nil
	}

	content, err := os.ReadFile(clean)
	if err != nil {
		slog.ErrorContext(ctx, "fs_read failed", "path", clean, "error", err)
		if os.IsNotExist(err) {
			return toolError(errReasonPathNotFound, fmt.Sprintf("file not found: %s", clean), "check the path"), nil
		}
		return toolError(errReasonReadFailed, fmt.Sprintf("read file: %v", err), "check file permissions"), nil
	}

	lines := strings.Split(string(content), "\n")
	start := int(args.StartLine)
	end := int(args.EndLine)
	if start <= 0 {
		start = 1
	}
	if end <= 0 || end > len(lines) {
		end = len(lines)
	}
	if start > end {
		return toolError(errReasonInvalidArgument, "start_line must be <= end_line", "adjust line range"), nil
	}

	out := strings.Join(lines[start-1:end], "\n")
	return mcp.NewToolResultText(out), nil
}

func fsGrepTool() mcp.Tool {
	return mcp.NewTool("fs_grep",
		mcp.WithDescription("Search file contents by substring or regex in any absolute path"),
		mcp.WithString("query", mcp.Required(), mcp.Description("Text or regex to search")),
		mcp.WithString("path", mcp.Required(), mcp.Description("Absolute file or directory path")),
		mcp.WithBoolean("regex", mcp.Description("Treat query as regex"), mcp.DefaultBool(false)),
		mcp.WithNumber("top_k", mcp.Description("Maximum number of matches"), mcp.DefaultNumber(20)),
	)
}

func (s *Server) fsGrep(ctx context.Context, request mcp.CallToolRequest, args fsGrepArgs) (*mcp.CallToolResult, error) {
	slog.InfoContext(ctx, "mcp tool call", "tool", "fs_grep", "query", args.Query, "path", args.Path)
	if args.Query == "" {
		return toolError(errReasonInvalidArgument, "query is required", "provide a non-empty search query"), nil
	}

	root, err := s.resolveAbsolutePath(args.Path)
	if err != nil {
		return toolError(errReasonInvalidArgument, err.Error(), "provide an absolute file or directory path"), nil
	}

	topK := int(args.TopK)
	if topK <= 0 {
		topK = 20
	}

	var re *regexp.Regexp
	if args.Regex {
		re, err = regexp.Compile(args.Query)
		if err != nil {
			return toolError(errReasonInvalidRegex, fmt.Sprintf("invalid regex: %v", err), "fix the regular expression"), nil
		}
	}

	var matches []grepMatch
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() {
			return nil
		}
		if !isTextFile(path) {
			return nil
		}
		info, walkErr := d.Info()
		if walkErr != nil || info.Size() > 1<<20 {
			return nil
		}
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		lines := strings.Split(string(content), "\n")
		for i, line := range lines {
			matched := (re != nil && re.MatchString(line)) || (re == nil && strings.Contains(line, args.Query))
			if matched {
				matches = append(matches, grepMatch{Path: path, Line: i + 1, Content: line})
				if len(matches) >= topK {
					return errGrepStop
				}
			}
		}
		return nil
	})
	if err != nil && err != errGrepStop {
		slog.ErrorContext(ctx, "fs_grep failed", "root", root, "error", err)
		return toolError(errReasonReadFailed, err.Error(), "check the path and permissions"), nil
	}

	data, err := json.Marshal(matches)
	if err != nil {
		return toolError(errReasonSearchFailed, err.Error(), "internal error marshalling matches"), nil
	}
	return mcp.NewToolResultText(string(data)), nil
}

func fsListTool() mcp.Tool {
	return mcp.NewTool("fs_list",
		mcp.WithDescription("List files in any absolute directory path"),
		mcp.WithString("path", mcp.Required(), mcp.Description("Absolute directory path")),
		mcp.WithString("pattern", mcp.Description("Glob pattern, e.g. *.go"), mcp.DefaultString("*")),
		mcp.WithBoolean("include_dirs", mcp.Description("Include directories in results"), mcp.DefaultBool(false)),
	)
}

func (s *Server) fsList(ctx context.Context, request mcp.CallToolRequest, args fsListArgs) (*mcp.CallToolResult, error) {
	slog.InfoContext(ctx, "mcp tool call", "tool", "fs_list", "path", args.Path, "pattern", args.Pattern)
	if args.Path == "" {
		return toolError(errReasonInvalidArgument, "path is required", "provide an absolute directory path"), nil
	}

	root, err := s.resolveAbsolutePath(args.Path)
	if err != nil {
		return toolError(errReasonInvalidArgument, err.Error(), "provide an absolute directory path"), nil
	}

	files, err := globFiles(root, args.Pattern)
	if err != nil {
		return toolError(errReasonInvalidArgument, err.Error(), "check the glob pattern"), nil
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
		return toolError(errReasonSearchFailed, err.Error(), "internal error marshalling file list"), nil
	}
	return mcp.NewToolResultText(string(data)), nil
}

func fsTreeTool() mcp.Tool {
	return mcp.NewTool("fs_tree",
		mcp.WithDescription("Return the directory tree up to a given depth for any absolute path"),
		mcp.WithString("path", mcp.Required(), mcp.Description("Absolute directory path")),
		mcp.WithNumber("depth", mcp.Description("Maximum depth"), mcp.DefaultNumber(3)),
	)
}

func (s *Server) fsTree(ctx context.Context, request mcp.CallToolRequest, args fsTreeArgs) (*mcp.CallToolResult, error) {
	slog.InfoContext(ctx, "mcp tool call", "tool", "fs_tree", "path", args.Path, "depth", args.Depth)
	if args.Path == "" {
		return toolError(errReasonInvalidArgument, "path is required", "provide an absolute directory path"), nil
	}

	root, err := s.resolveAbsolutePath(args.Path)
	if err != nil {
		return toolError(errReasonInvalidArgument, err.Error(), "provide an absolute directory path"), nil
	}

	depth := int(args.Depth)
	if depth <= 0 {
		depth = 3
	}

	tree, err := buildTree(root, depth)
	if err != nil {
		slog.ErrorContext(ctx, "fs_tree failed", "root", root, "error", err)
		if os.IsNotExist(err) {
			return toolError(errReasonPathNotFound, fmt.Sprintf("path not found: %s", root), "check the directory path"), nil
		}
		return toolError(errReasonReadFailed, err.Error(), "check the path and permissions"), nil
	}

	data, err := json.Marshal(tree)
	if err != nil {
		return toolError(errReasonSearchFailed, err.Error(), "internal error marshalling tree"), nil
	}
	return mcp.NewToolResultText(string(data)), nil
}

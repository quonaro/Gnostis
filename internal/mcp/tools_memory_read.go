package mcp

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

type memoryListArgs struct {
	Provider string `json:"provider,omitempty"`
}

type memoryListResult struct {
	Path     string `json:"path"`
	Provider string `json:"provider"`
}

func memoryListTool() mcp.Tool {
	return mcp.NewTool("memory_list",
		mcp.WithDescription("List indexed memory files."),
		mcp.WithString("provider", mcp.Description("Memory provider to filter by, e.g. cascade or cursor")),
	)
}

func (s *Server) memoryList(ctx context.Context, _ mcp.CallToolRequest, args memoryListArgs) (*mcp.CallToolResult, error) {
	slog.InfoContext(ctx, "mcp tool call", "tool", "memory_list")

	if s.memoryManager == nil {
		return toolError(errReasonMemoryNotEnabled, "memory is not enabled", "enable a memory provider in the Gnostis configuration or run `lota service install`"), nil
	}

	dataDir := s.memoryManager.DataDir()
	entries, err := os.ReadDir(dataDir)
	if err != nil {
		if os.IsNotExist(err) {
			return mcp.NewToolResultText("[]"), nil
		}
		slog.ErrorContext(ctx, "memory_list failed", "dir", dataDir, "error", err)
		return toolError(errReasonReadFailed, err.Error(), "check memory directory permissions"), nil
	}

	providerFilter := args.Provider
	if providerFilter == "" {
		providerFilter = defaultMemoryProvider
	}

	var results []memoryListResult
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		path := filepath.Join(dataDir, entry.Name())
		content, err := os.ReadFile(path)
		if err != nil {
			slog.WarnContext(ctx, "read memory file", "path", path, "error", err)
			continue
		}
		provider := extractProvider(string(content))
		if providerFilter != "" && provider != providerFilter {
			continue
		}
		results = append(results, memoryListResult{Path: path, Provider: provider})
	}

	data, err := json.Marshal(results)
	if err != nil {
		return toolError(errReasonSearchFailed, err.Error(), "internal error marshalling memory list"), nil
	}
	return mcp.NewToolResultText(string(data)), nil
}

type memoryReadArgs struct {
	Path string `json:"path"`
}

func memoryReadTool() mcp.Tool {
	return mcp.NewTool("memory_read",
		mcp.WithDescription("Read a specific memory Markdown file."),
		mcp.WithString("path", mcp.Required(), mcp.Description("Absolute path to the memory file")),
	)
}

func (s *Server) memoryRead(ctx context.Context, _ mcp.CallToolRequest, args memoryReadArgs) (*mcp.CallToolResult, error) {
	slog.InfoContext(ctx, "mcp tool call", "tool", "memory_read", "path", args.Path)

	if strings.TrimSpace(args.Path) == "" {
		return toolError(errReasonInvalidArgument, "path is required", "provide an absolute memory file path"), nil
	}
	if s.memoryManager == nil {
		return toolError(errReasonMemoryNotEnabled, "memory is not enabled", "enable a memory provider in the Gnostis configuration or run `lota service install`"), nil
	}

	clean := filepath.Clean(args.Path)
	if !strings.HasPrefix(clean, s.memoryManager.DataDir()) {
		return toolError(errReasonPathNotAllowed, "path is outside memory directory", "provide a path inside the memory data directory"), nil
	}

	content, err := os.ReadFile(clean)
	if err != nil {
		if os.IsNotExist(err) {
			return toolError(errReasonNotFound, "file not found", "check the memory file path"), nil
		}
		slog.ErrorContext(ctx, "memory_read failed", "path", clean, "error", err)
		return toolError(errReasonReadFailed, err.Error(), "check file permissions"), nil
	}

	return mcp.NewToolResultText(string(content)), nil
}

type rebuildMemoryResult struct {
	Chunks int `json:"chunks"`
}

func rebuildMemoryTool() mcp.Tool {
	return mcp.NewTool("rebuild_memory",
		mcp.WithDescription("Clear and rebuild the memory (dialogue/note) index."),
	)
}

func (s *Server) rebuildMemory(ctx context.Context, _ mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, error) {
	slog.InfoContext(ctx, "mcp tool call", "tool", "rebuild_memory")

	if s.memoryManager == nil {
		return toolError(errReasonMemoryNotEnabled, "memory is not enabled", "enable a memory provider in the Gnostis configuration or run `lota service install`"), nil
	}

	if err := s.memoryManager.Rebuild(ctx); err != nil {
		slog.ErrorContext(ctx, "rebuild_memory failed", "error", err)
		return toolError(errReasonSearchFailed, err.Error(), "try again later"), nil
	}

	res := rebuildMemoryResult{Chunks: s.memoryManager.Store().Count()}
	data, err := json.Marshal(res)
	if err != nil {
		return toolError(errReasonSearchFailed, err.Error(), "internal error marshalling rebuild memory result"), nil
	}
	return mcp.NewToolResultText(string(data)), nil
}

func extractProvider(content string) string {
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(line, "- **Provider:**") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(strings.ReplaceAll(parts[1], "**", ""))
			}
		}
	}
	return defaultMemoryProvider
}

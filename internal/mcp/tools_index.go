package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"

	"github.com/mark3labs/mcp-go/mcp"
)

type reindexFilesArgs struct {
	Paths []string `json:"paths"`
}

type reindexFilesResult struct {
	Reindexed []string `json:"reindexed"`
}

func reindexFilesTool() mcp.Tool {
	return mcp.NewTool("reindex_files",
		mcp.WithDescription("Reindex specific files or directories so they are searchable again"),
		mcp.WithArray("paths", mcp.Required(), mcp.Description("Absolute file or directory paths to reindex")),
	)
}

func (s *Server) reindexFiles(ctx context.Context, request mcp.CallToolRequest, args reindexFilesArgs) (*mcp.CallToolResult, error) {
	slog.InfoContext(ctx, "mcp tool call", "tool", "reindex_files", "paths", args.Paths)
	if len(args.Paths) == 0 {
		return toolError(errReasonInvalidArgument, "paths is required", "provide at least one absolute path"), nil
	}
	if s.indexer == nil {
		return toolError(errReasonNotConfigured, "reindexing is not configured", "check the Gnostis configuration"), nil
	}

	cleanPaths := make([]string, 0, len(args.Paths))
	for _, p := range args.Paths {
		clean, err := s.resolveAbsolutePath(p)
		if err != nil {
			return toolError(errReasonInvalidArgument, err.Error(), "provide an absolute path"), nil
		}

		if _, err := os.Stat(clean); err != nil {
			if os.IsNotExist(err) {
				return toolError(errReasonPathNotFound, fmt.Sprintf("path not found: %s", clean), "check the path"), nil
			}
			return toolError(errReasonReadFailed, fmt.Sprintf("stat %s: %v", clean, err), "check permissions"), nil
		}

		if !s.isPathAllowed(clean) {
			return toolError(errReasonPathNotAllowed, fmt.Sprintf("path %s is outside indexed projects", clean), "use fs_* tools or add the project to the index"), nil
		}

		cleanPaths = append(cleanPaths, clean)
	}

	if err := s.indexer.ReindexFiles(ctx, cleanPaths); err != nil {
		slog.ErrorContext(ctx, "reindex_files failed", "paths", cleanPaths, "error", err)
		return toolError(errReasonSearchFailed, err.Error(), "try again later or check the index status"), nil
	}

	data, err := json.Marshal(reindexFilesResult{Reindexed: cleanPaths})
	if err != nil {
		return toolError(errReasonSearchFailed, err.Error(), "internal error marshalling result"), nil
	}
	return mcp.NewToolResultText(string(data)), nil
}

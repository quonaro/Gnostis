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
		return mcp.NewToolResultError("paths is required"), nil
	}
	if s.reindexer == nil {
		return mcp.NewToolResultError("reindexing is not configured"), nil
	}

	cleanPaths := make([]string, 0, len(args.Paths))
	for _, p := range args.Paths {
		clean, err := s.resolveAbsolutePath(p)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		if _, err := os.Stat(clean); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("stat %s: %v", clean, err)), nil
		}

		cleanPaths = append(cleanPaths, clean)
	}

	if err := s.reindexer.ReindexFiles(ctx, cleanPaths); err != nil {
		slog.ErrorContext(ctx, "reindex_files failed", "paths", cleanPaths, "error", err)
		return nil, fmt.Errorf("reindex files: %w", err)
	}

	data, err := json.Marshal(reindexFilesResult{Reindexed: cleanPaths})
	if err != nil {
		return nil, fmt.Errorf("marshal reindex result: %w", err)
	}
	return mcp.NewToolResultText(string(data)), nil
}

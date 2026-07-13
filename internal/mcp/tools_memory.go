package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

const defaultMemoryProvider = "cascade"

type memorySearchArgs struct {
	Query    string `json:"query"`
	Provider string `json:"provider,omitempty"`
	TopK     int    `json:"top_k,omitempty"`
}

type memorySearchResult struct {
	Path      string  `json:"path"`
	Provider  string  `json:"provider"`
	Score     float32 `json:"score"`
	Content   string  `json:"content,omitempty"`
	StartLine int     `json:"start_line"`
	EndLine   int     `json:"end_line"`
}

func memorySearchTool() mcp.Tool {
	return mcp.NewTool("memory_search",
		mcp.WithDescription("Semantic search over indexed memory (chat dialogues and saved notes)."),
		mcp.WithString("query", mcp.Required(), mcp.Description("Natural language search query")),
		mcp.WithString("provider", mcp.Description("Memory provider to restrict the search, e.g. cascade or cursor")),
		mcp.WithNumber("top_k", mcp.Description("Number of results"), mcp.DefaultNumber(10)),
	)
}

func (s *Server) memorySearch(ctx context.Context, _ mcp.CallToolRequest, args memorySearchArgs) (*mcp.CallToolResult, error) {
	slog.InfoContext(ctx, "mcp tool call", "tool", "memory_search", "query", args.Query)

	if strings.TrimSpace(args.Query) == "" {
		return toolError(errReasonInvalidArgument, "query is required", "provide a non-empty search query"), nil
	}
	if s.memoryManager == nil {
		return toolError(errReasonMemoryNotEnabled, "memory is not enabled", "enable a memory provider in the Gnostis configuration or run `lota service install`"), nil
	}

	providerID := args.Provider
	if providerID == "" {
		providerID = defaultMemoryProvider
	}

	vectors, err := s.memoryManager.Provider().Embed(ctx, []string{args.Query})
	if err != nil {
		slog.ErrorContext(ctx, "memory_search failed", "error", err)
		return toolError(errReasonSearchFailed, err.Error(), "try again later"), nil
	}
	if len(vectors) == 0 {
		return toolError(errReasonSearchFailed, "empty query embedding", "try again with a different query"), nil
	}

	topK := args.TopK
	if topK <= 0 {
		topK = 10
	}

	filters := map[string]string{"project_id": "memory-" + providerID}
	raw, err := s.memoryManager.Store().Query(ctx, vectors[0], topK*2, filters)
	if err != nil {
		slog.ErrorContext(ctx, "memory_search failed", "error", err)
		return toolError(errReasonSearchFailed, err.Error(), "try again later"), nil
	}

	results := make([]memorySearchResult, 0, len(raw))
	seen := make(map[string]bool)
	for _, r := range raw {
		path := r.Metadata["path"]
		if seen[path] {
			continue
		}
		seen[path] = true

		content := r.Content
		if len(content) > 800 {
			content = content[:800] + "..."
		}

		startLine := 0
		endLine := 0
		_, _ = fmt.Sscanf(r.Metadata["start_line"], "%d", &startLine)
		_, _ = fmt.Sscanf(r.Metadata["end_line"], "%d", &endLine)

		results = append(results, memorySearchResult{
			Path:      path,
			Provider:  providerID,
			Score:     r.Similarity,
			Content:   content,
			StartLine: startLine,
			EndLine:   endLine,
		})
		if len(results) >= topK {
			break
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	data, err := json.Marshal(results)
	if err != nil {
		return toolError(errReasonSearchFailed, err.Error(), "internal error marshalling memory search results"), nil
	}
	return mcp.NewToolResultText(string(data)), nil
}

type memoryWriteArgs struct {
	Title    string   `json:"title"`
	Content  string   `json:"content"`
	Tags     []string `json:"tags,omitempty"`
	Provider string   `json:"provider,omitempty"`
}

type memoryWriteResult struct {
	Path string `json:"path"`
}

func memoryWriteTool() mcp.Tool {
	return mcp.NewTool("memory_write",
		mcp.WithDescription("Persist a note, summary, or fact to memory so it can be retrieved later via semantic search."),
		mcp.WithString("title", mcp.Required(), mcp.Description("Short title for the note")),
		mcp.WithString("content", mcp.Required(), mcp.Description("Body text to save")),
		mcp.WithArray("tags", mcp.Description("Optional tags for filtering")),
		mcp.WithString("provider", mcp.Description("Memory provider to associate the note with"), mcp.DefaultString(defaultMemoryProvider)),
	)
}

func (s *Server) memoryWrite(ctx context.Context, _ mcp.CallToolRequest, args memoryWriteArgs) (*mcp.CallToolResult, error) {
	slog.InfoContext(ctx, "mcp tool call", "tool", "memory_write", "title", args.Title)

	if strings.TrimSpace(args.Title) == "" {
		return toolError(errReasonInvalidArgument, "title is required", "provide a non-empty title for the note"), nil
	}
	if strings.TrimSpace(args.Content) == "" {
		return toolError(errReasonInvalidArgument, "content is required", "provide non-empty note content"), nil
	}
	if s.memoryManager == nil {
		return toolError(errReasonMemoryNotEnabled, "memory is not enabled", "enable a memory provider in the Gnostis configuration or run `lota service install`"), nil
	}

	providerID := args.Provider
	if providerID == "" {
		providerID = defaultMemoryProvider
	}

	path, err := s.memoryManager.WriteNote(ctx, args.Title, args.Content, args.Tags, providerID)
	if err != nil {
		slog.ErrorContext(ctx, "memory_write failed", "error", err)
		return toolError(errReasonSearchFailed, err.Error(), "try again later"), nil
	}

	res := memoryWriteResult{Path: path}
	data, err := json.Marshal(res)
	if err != nil {
		return toolError(errReasonSearchFailed, err.Error(), "internal error marshalling memory write result"), nil
	}
	return mcp.NewToolResultText(string(data)), nil
}

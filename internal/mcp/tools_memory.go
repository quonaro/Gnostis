package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
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
		return mcp.NewToolResultError("query is required"), nil
	}
	if s.memoryManager == nil {
		return mcp.NewToolResultError("memory is not enabled"), nil
	}

	providerID := args.Provider
	if providerID == "" {
		providerID = defaultMemoryProvider
	}

	vectors, err := s.memoryManager.Provider().Embed(ctx, []string{args.Query})
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}
	if len(vectors) == 0 {
		return nil, fmt.Errorf("empty query embedding")
	}

	topK := args.TopK
	if topK <= 0 {
		topK = 10
	}

	filters := map[string]string{"project_id": "memory-" + providerID}
	raw, err := s.memoryManager.Store().Query(ctx, vectors[0], topK*2, filters)
	if err != nil {
		return nil, fmt.Errorf("query memory store: %w", err)
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
		return nil, fmt.Errorf("marshal memory search results: %w", err)
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
		return mcp.NewToolResultError("title is required"), nil
	}
	if strings.TrimSpace(args.Content) == "" {
		return mcp.NewToolResultError("content is required"), nil
	}
	if s.memoryManager == nil {
		return mcp.NewToolResultError("memory is not enabled"), nil
	}

	providerID := args.Provider
	if providerID == "" {
		providerID = defaultMemoryProvider
	}

	path, err := s.memoryManager.WriteNote(ctx, args.Title, args.Content, args.Tags, providerID)
	if err != nil {
		return nil, fmt.Errorf("write memory note: %w", err)
	}

	res := memoryWriteResult{Path: path}
	data, err := json.Marshal(res)
	if err != nil {
		return nil, fmt.Errorf("marshal memory write result: %w", err)
	}
	return mcp.NewToolResultText(string(data)), nil
}

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
		return mcp.NewToolResultError("memory is not enabled"), nil
	}

	dataDir := s.memoryManager.DataDir()
	entries, err := os.ReadDir(dataDir)
	if err != nil {
		if os.IsNotExist(err) {
			return mcp.NewToolResultText("[]"), nil
		}
		return nil, fmt.Errorf("read memory dir: %w", err)
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
		return nil, fmt.Errorf("marshal memory list: %w", err)
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
		return mcp.NewToolResultError("path is required"), nil
	}
	if s.memoryManager == nil {
		return mcp.NewToolResultError("memory is not enabled"), nil
	}

	clean := filepath.Clean(args.Path)
	if !strings.HasPrefix(clean, s.memoryManager.DataDir()) {
		return mcp.NewToolResultError("path is outside memory directory"), nil
	}

	content, err := os.ReadFile(clean)
	if err != nil {
		if os.IsNotExist(err) {
			return mcp.NewToolResultError("file not found"), nil
		}
		return nil, fmt.Errorf("read memory file: %w", err)
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
		return mcp.NewToolResultError("memory is not enabled"), nil
	}

	if err := s.memoryManager.Rebuild(ctx); err != nil {
		return nil, fmt.Errorf("rebuild memory: %w", err)
	}

	res := rebuildMemoryResult{Chunks: s.memoryManager.Store().Count()}
	data, err := json.Marshal(res)
	if err != nil {
		return nil, fmt.Errorf("marshal rebuild memory result: %w", err)
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

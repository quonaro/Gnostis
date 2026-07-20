package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/quonaro/gnostis/internal/search"
	"github.com/quonaro/gnostis/internal/symbol"
)

type searchCodebaseArgs struct {
	Query          string  `json:"query"`
	Project        string  `json:"project"`
	Path           string  `json:"path"`
	Language       string  `json:"language"`
	TopK           float64 `json:"top_k"`
	IncludeContent bool    `json:"include_content"`
}

type findSymbolArgs struct {
	Name     string `json:"name"`
	Project  string `json:"project"`
	Language string `json:"language"`
}

type getFileContextArgs struct {
	Path      string  `json:"path"`
	StartLine float64 `json:"start_line"`
	EndLine   float64 `json:"end_line"`
}

type listProjectsArgs struct{}

type searchResultItem struct {
	ID        string  `json:"id"`
	ProjectID string  `json:"project"`
	Path      string  `json:"path"`
	Language  string  `json:"language"`
	Symbol    string  `json:"symbol"`
	Signature string  `json:"signature"`
	StartLine int     `json:"start_line"`
	EndLine   int     `json:"end_line"`
	Score     float32 `json:"score"`
	Content   string  `json:"content,omitempty"`
}

func (s *Server) searchCodebase(ctx context.Context, request mcp.CallToolRequest, args searchCodebaseArgs) (*mcp.CallToolResult, error) {
	slog.InfoContext(ctx, "mcp tool call", "tool", "search_codebase", "query", args.Query, "project", args.Project, "path", args.Path, "language", args.Language)
	if args.Query == "" {
		return toolError(errReasonInvalidArgument, "query is required", "provide a non-empty search query"), nil
	}

	filters := map[string]string{}
	if args.Project != "" {
		filters["project_id"] = args.Project
	}
	if args.Language != "" {
		filters["language"] = strings.ToLower(args.Language)
	}

	if args.Path != "" {
		prefix, err := s.resolvePathOrAbsolute(args.Project, args.Path)
		if err != nil {
			return toolErrorFromResolvePath(err), nil
		}
		filters["path"] = prefix
	}

	topK := int(args.TopK)
	if topK <= 0 {
		topK = 10
	}

	results, err := s.engine.Search(ctx, args.Query, filters, topK)
	if err != nil {
		slog.ErrorContext(ctx, "search_codebase failed", "query", args.Query, "error", err)
		return toolError(errReasonSearchFailed, err.Error(), "try again later or check the index status"), nil
	}
	slog.DebugContext(ctx, "search_codebase results", "count", len(results))

	if len(results) == 0 {
		if notReady := s.indexNotReadyError(); notReady != nil {
			return notReady, nil
		}
	}

	items := make([]searchResultItem, len(results))
	for i, r := range results {
		items[i] = searchResultItem{
			ID:        r.ID,
			ProjectID: r.ProjectID,
			Path:      r.Path,
			Language:  r.Language,
			Symbol:    r.Symbol,
			Signature: r.Signature,
			StartLine: r.StartLine,
			EndLine:   r.EndLine,
			Score:     r.Score,
		}
		if args.IncludeContent {
			items[i].Content = r.Content
		}
	}

	data, err := json.Marshal(items)
	if err != nil {
		return nil, fmt.Errorf("marshal results: %w", err)
	}

	return mcp.NewToolResultText(string(data)), nil
}

func (s *Server) findSymbol(ctx context.Context, request mcp.CallToolRequest, args findSymbolArgs) (*mcp.CallToolResult, error) {
	slog.InfoContext(ctx, "mcp tool call", "tool", "find_symbol", "name", args.Name, "project", args.Project, "language", args.Language)
	if args.Name == "" {
		return toolError(errReasonInvalidArgument, "name is required", "provide a symbol name to find"), nil
	}

	var matched []symbol.Location
	if s.symbols != nil {
		matched = s.symbols.Lookup(args.Name)
		if len(matched) == 0 {
			matched = s.symbols.SearchFuzzy(args.Name)
		}
	}

	matched = filterSymbolLocations(matched, args.Project, args.Language)

	// If the symbol index has no match, fall back to semantic search.
	if len(matched) == 0 {
		items, err := s.findSymbolSemantic(ctx, args)
		if err != nil {
			return toolError(errReasonSearchFailed, err.Error(), "try again later or check the index status"), nil
		}
		matched = items
	}

	items := make([]searchResultItem, len(matched))
	for i, loc := range matched {
		items[i] = symbolLocationToItem(loc)
	}

	data, err := json.Marshal(items)
	if err != nil {
		return toolError(errReasonSearchFailed, err.Error(), "internal error marshalling results"), nil
	}

	return mcp.NewToolResultText(string(data)), nil
}

func (s *Server) findSymbolSemantic(ctx context.Context, args findSymbolArgs) ([]symbol.Location, error) {
	filters := map[string]string{}
	if args.Project != "" {
		filters["project_id"] = args.Project
	}
	if args.Language != "" {
		filters["language"] = strings.ToLower(args.Language)
	}

	query := fmt.Sprintf("function or type named %s", args.Name)
	results, err := s.engine.Search(ctx, query, filters, 10)
	if err != nil {
		slog.ErrorContext(ctx, "find_symbol failed", "name", args.Name, "error", err)
		return nil, fmt.Errorf("search symbol: %w", err)
	}
	slog.DebugContext(ctx, "find_symbol semantic results", "count", len(results))

	nameLower := strings.ToLower(args.Name)
	var matched []symbol.Location
	for _, r := range results {
		if strings.EqualFold(r.Symbol, args.Name) || strings.Contains(strings.ToLower(r.Symbol), nameLower) {
			matched = append(matched, searchResultToSymbolLocation(r))
		}
	}
	return matched, nil
}

func filterSymbolLocations(locs []symbol.Location, project, language string) []symbol.Location {
	if project == "" && language == "" {
		return locs
	}
	filtered := make([]symbol.Location, 0, len(locs))
	for _, loc := range locs {
		if project != "" && loc.ProjectID != project {
			continue
		}
		if language != "" && !strings.EqualFold(loc.Language, language) {
			continue
		}
		filtered = append(filtered, loc)
	}
	return filtered
}

func symbolLocationToItem(loc symbol.Location) searchResultItem {
	return searchResultItem{
		ProjectID: loc.ProjectID,
		Path:      loc.Path,
		Language:  loc.Language,
		Symbol:    loc.Symbol,
		Signature: loc.Signature,
		StartLine: loc.StartLine,
		EndLine:   loc.EndLine,
		Score:     1.0,
	}
}

func searchResultToSymbolLocation(r search.Result) symbol.Location {
	return symbol.Location{
		ProjectID: r.ProjectID,
		Path:      r.Path,
		Language:  r.Language,
		Symbol:    r.Symbol,
		Signature: r.Signature,
		StartLine: r.StartLine,
		EndLine:   r.EndLine,
	}
}

func (s *Server) getFileContext(ctx context.Context, request mcp.CallToolRequest, args getFileContextArgs) (*mcp.CallToolResult, error) {
	slog.InfoContext(ctx, "mcp tool call", "tool", "get_file_context", "path", args.Path, "start_line", args.StartLine, "end_line", args.EndLine)
	if args.Path == "" {
		return toolError(errReasonInvalidArgument, "path is required", "provide an absolute file path"), nil
	}

	clean, err := s.resolvePathOrAbsolute("", args.Path)
	if err != nil {
		return toolErrorFromResolvePath(err), nil
	}

	content, err := os.ReadFile(clean)
	if err != nil {
		slog.ErrorContext(ctx, "get_file_context failed", "path", clean, "error", err)
		if os.IsNotExist(err) {
			return toolError(errReasonPathNotFound, fmt.Sprintf("file not found: %s", clean), "check the path or use fs_read for non-project files"), nil
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

func (s *Server) listProjects(ctx context.Context, request mcp.CallToolRequest, args listProjectsArgs) (*mcp.CallToolResult, error) {
	slog.InfoContext(ctx, "mcp tool call", "tool", "list_projects")
	type projectItem struct {
		Name string `json:"name"`
		Path string `json:"path"`
	}

	s.mu.RLock()
	items := make([]projectItem, len(s.projects))
	for i, p := range s.projects {
		items[i] = projectItem{Name: p.Name, Path: p.Path}
	}
	s.mu.RUnlock()

	data, err := json.Marshal(items)
	if err != nil {
		return toolError(errReasonSearchFailed, err.Error(), "internal error marshalling project list"), nil
	}

	return mcp.NewToolResultText(string(data)), nil
}

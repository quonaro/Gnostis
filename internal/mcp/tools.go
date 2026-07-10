package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/quonaro/gnostis/internal/search"
)

type searchCodebaseArgs struct {
	Query          string  `json:"query"`
	Project        string  `json:"project"`
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
	if args.Query == "" {
		return mcp.NewToolResultError("query is required"), nil
	}

	filters := map[string]string{}
	if args.Project != "" {
		filters["project_id"] = args.Project
	}
	if args.Language != "" {
		filters["language"] = strings.ToLower(args.Language)
	}

	topK := int(args.TopK)
	if topK <= 0 {
		topK = 10
	}

	results, err := s.engine.Search(ctx, args.Query, filters, topK)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
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
	if args.Name == "" {
		return mcp.NewToolResultError("name is required"), nil
	}

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
		return nil, fmt.Errorf("search symbol: %w", err)
	}

	var matched []search.Result
	nameLower := strings.ToLower(args.Name)
	for _, r := range results {
		if strings.EqualFold(r.Symbol, args.Name) || strings.Contains(strings.ToLower(r.Symbol), nameLower) {
			matched = append(matched, r)
		}
	}

	items := make([]searchResultItem, len(matched))
	for i, r := range matched {
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
			Content:   r.Content,
		}
	}

	data, err := json.Marshal(items)
	if err != nil {
		return nil, fmt.Errorf("marshal results: %w", err)
	}

	return mcp.NewToolResultText(string(data)), nil
}

func (s *Server) getFileContext(ctx context.Context, request mcp.CallToolRequest, args getFileContextArgs) (*mcp.CallToolResult, error) {
	if args.Path == "" {
		return mcp.NewToolResultError("path is required"), nil
	}

	content, err := os.ReadFile(args.Path)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
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
		return mcp.NewToolResultError("start_line must be <= end_line"), nil
	}

	out := strings.Join(lines[start-1:end], "\n")
	return mcp.NewToolResultText(out), nil
}

func (s *Server) listProjects(ctx context.Context, request mcp.CallToolRequest, args listProjectsArgs) (*mcp.CallToolResult, error) {
	type projectItem struct {
		Name string `json:"name"`
		Path string `json:"path"`
	}

	items := make([]projectItem, len(s.projects))
	for i, p := range s.projects {
		items[i] = projectItem{Name: p.Name, Path: p.Path}
	}

	data, err := json.Marshal(items)
	if err != nil {
		return nil, fmt.Errorf("marshal projects: %w", err)
	}

	return mcp.NewToolResultText(string(data)), nil
}

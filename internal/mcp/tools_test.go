package mcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/quonaro/gnostis/internal/project"
	"github.com/quonaro/gnostis/internal/search"
)

type mockSearcher struct {
	results []search.Result
	err     error
}

func (m *mockSearcher) Search(ctx context.Context, query string, filters map[string]string, topK int) ([]search.Result, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.results, nil
}

func TestSearchCodebase_EmptyQuery(t *testing.T) {
	srv := New("test", "1.0.0", &mockSearcher{}, nil)
	req := mcp.CallToolRequest{}

	res, err := srv.searchCodebase(context.Background(), req, searchCodebaseArgs{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil || !res.IsError {
		t.Fatalf("expected error result, got %+v", res)
	}
	assertTextEquals(t, res, "query is required")
}

func TestSearchCodebase_Results(t *testing.T) {
	mock := &mockSearcher{results: []search.Result{
		{
			ID:        "chunk-1",
			ProjectID: "proj",
			Path:      "/foo/bar.go",
			Language:  "go",
			Symbol:    "Bar",
			Signature: "func Bar()",
			StartLine: 10,
			EndLine:   20,
			Score:     0.9,
			Content:   "func Bar() {}",
		},
	}}
	srv := New("test", "1.0.0", mock, nil)
	req := mcp.CallToolRequest{}
	args := searchCodebaseArgs{Query: "find bar", TopK: 5, IncludeContent: true}

	res, err := srv.searchCodebase(context.Background(), req, args)
	if err != nil {
		t.Fatalf("searchCodebase: %v", err)
	}

	items := extractResultItems(t, res)
	if len(items) != 1 {
		t.Fatalf("expected 1 result, got %d", len(items))
	}
	if items[0].Content == "" {
		t.Errorf("expected content to be included")
	}
	if items[0].ProjectID != "proj" {
		t.Errorf("project = %q, want proj", items[0].ProjectID)
	}
}

func TestFindSymbol_Match(t *testing.T) {
	mock := &mockSearcher{results: []search.Result{
		{
			ID:        "chunk-1",
			ProjectID: "proj",
			Path:      "/foo/bar.go",
			Language:  "go",
			Symbol:    "Bar",
			Signature: "func Bar()",
			StartLine: 10,
			EndLine:   20,
			Score:     0.9,
			Content:   "func Bar() {}",
		},
		{
			ID:        "chunk-2",
			ProjectID: "proj",
			Path:      "/foo/baz.go",
			Language:  "go",
			Symbol:    "Baz",
			Signature: "func Baz()",
			StartLine: 5,
			EndLine:   15,
			Score:     0.8,
			Content:   "func Baz() {}",
		},
	}}
	srv := New("test", "1.0.0", mock, nil)
	req := mcp.CallToolRequest{}
	args := findSymbolArgs{Name: "Bar"}

	res, err := srv.findSymbol(context.Background(), req, args)
	if err != nil {
		t.Fatalf("findSymbol: %v", err)
	}

	items := extractResultItems(t, res)
	if len(items) != 1 {
		t.Fatalf("expected 1 match, got %d", len(items))
	}
	if items[0].Symbol != "Bar" {
		t.Errorf("symbol = %q, want Bar", items[0].Symbol)
	}
}

func TestGetFileContext(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.go")
	data := "line1\nline2\nline3\nline4\nline5\n"
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	srv := New("test", "1.0.0", &mockSearcher{}, nil)
	req := mcp.CallToolRequest{}
	args := getFileContextArgs{Path: path, StartLine: 2, EndLine: 4}

	res, err := srv.getFileContext(context.Background(), req, args)
	if err != nil {
		t.Fatalf("getFileContext: %v", err)
	}

	got := extractText(t, res)
	want := "line2\nline3\nline4"
	if got != want {
		t.Errorf("content = %q, want %q", got, want)
	}
}

func TestListProjects(t *testing.T) {
	projects := []project.Project{
		{Name: "foo", Path: "/projects/foo"},
		{Name: "bar", Path: "/projects/bar"},
	}
	srv := New("test", "1.0.0", &mockSearcher{}, projects)
	req := mcp.CallToolRequest{}

	res, err := srv.listProjects(context.Background(), req, listProjectsArgs{})
	if err != nil {
		t.Fatalf("listProjects: %v", err)
	}

	var got []struct {
		Name string `json:"name"`
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(extractText(t, res)), &got); err != nil {
		t.Fatalf("unmarshal projects: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(got))
	}
	if got[0].Name != "foo" || got[1].Name != "bar" {
		t.Errorf("unexpected projects: %+v", got)
	}
}

func assertTextEquals(t *testing.T, res *mcp.CallToolResult, want string) {
	t.Helper()
	got := extractText(t, res)
	if got != want {
		t.Errorf("result text = %q, want %q", got, want)
	}
}

func extractText(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	if len(res.Content) == 0 {
		t.Fatal("empty result content")
	}
	text, ok := mcp.AsTextContent(res.Content[0])
	if !ok {
		t.Fatalf("expected text content, got %T", res.Content[0])
	}
	return text.Text
}

func extractResultItems(t *testing.T, res *mcp.CallToolResult) []searchResultItem {
	t.Helper()
	var items []searchResultItem
	if err := json.Unmarshal([]byte(extractText(t, res)), &items); err != nil {
		t.Fatalf("unmarshal results: %v", err)
	}
	return items
}

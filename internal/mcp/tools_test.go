package mcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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
	srv := New("test", "1.0.0", &mockSearcher{}, nil, nil, nil, nil)
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
	srv := New("test", "1.0.0", mock, nil, nil, nil, nil)
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
	srv := New("test", "1.0.0", mock, nil, nil, nil, nil)
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

	srv := New("test", "1.0.0", &mockSearcher{}, nil, nil, nil, []project.Project{{Name: "test", Path: dir}})
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

func TestGetFileContext_MissingFile(t *testing.T) {
	dir := t.TempDir()
	srv := New("test", "1.0.0", &mockSearcher{}, nil, nil, nil, []project.Project{{Name: "test", Path: dir}})

	res, err := srv.getFileContext(context.Background(), mcp.CallToolRequest{}, getFileContextArgs{Path: filepath.Join(dir, "missing.go")})
	if err != nil {
		t.Fatalf("getFileContext returned internal error: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected error result for missing file")
	}
}

func TestListProjects(t *testing.T) {
	projects := []project.Project{
		{Name: "foo", Path: "/projects/foo"},
		{Name: "bar", Path: "/projects/bar"},
	}
	srv := New("test", "1.0.0", &mockSearcher{}, nil, nil, nil, projects)
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

func TestListFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("a"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.md"), []byte("b"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	srv := New("test", "1.0.0", &mockSearcher{}, nil, nil, nil, []project.Project{{Name: "test", Path: dir}})
	res, err := srv.listFiles(context.Background(), mcp.CallToolRequest{}, listFilesArgs{Project: "test", Pattern: "*.go"})
	if err != nil {
		t.Fatalf("listFiles: %v", err)
	}
	var files []fileEntry
	if err := json.Unmarshal([]byte(extractText(t, res)), &files); err != nil {
		t.Fatalf("unmarshal files: %v", err)
	}
	if len(files) != 1 || !strings.HasSuffix(files[0].Path, "a.go") {
		t.Errorf("unexpected files: %+v", files)
	}
}

func TestListFiles_NoDirsByDefault(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("a"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	srv := New("test", "1.0.0", &mockSearcher{}, nil, nil, nil, []project.Project{{Name: "test", Path: dir}})
	res, err := srv.listFiles(context.Background(), mcp.CallToolRequest{}, listFilesArgs{Project: "test", Pattern: "*"})
	if err != nil {
		t.Fatalf("listFiles: %v", err)
	}
	var files []fileEntry
	if err := json.Unmarshal([]byte(extractText(t, res)), &files); err != nil {
		t.Fatalf("unmarshal files: %v", err)
	}
	if len(files) != 1 || !strings.HasSuffix(files[0].Path, "a.go") {
		t.Errorf("expected only a.go, got: %+v", files)
	}
}

func TestListFiles_IncludeDirs(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("a"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	srv := New("test", "1.0.0", &mockSearcher{}, nil, nil, nil, []project.Project{{Name: "test", Path: dir}})
	res, err := srv.listFiles(context.Background(), mcp.CallToolRequest{}, listFilesArgs{Project: "test", Pattern: "*", IncludeDirs: true})
	if err != nil {
		t.Fatalf("listFiles: %v", err)
	}
	var files []fileEntry
	if err := json.Unmarshal([]byte(extractText(t, res)), &files); err != nil {
		t.Fatalf("unmarshal files: %v", err)
	}
	if len(files) != 2 {
		t.Errorf("expected 2 entries, got: %+v", files)
	}
}

func TestListFiles_MissingProjectAndPath(t *testing.T) {
	srv := New("test", "1.0.0", &mockSearcher{}, nil, nil, nil, []project.Project{{Name: "test", Path: "/tmp/test"}})
	res, err := srv.listFiles(context.Background(), mcp.CallToolRequest{}, listFilesArgs{})
	if err != nil {
		t.Fatalf("listFiles: %v", err)
	}
	if res == nil || !res.IsError {
		t.Fatalf("expected error result, got %+v", res)
	}
	assertTextEquals(t, res, "project or path is required")
}

func TestGrep_MissingProjectAndPath(t *testing.T) {
	srv := New("test", "1.0.0", &mockSearcher{}, nil, nil, nil, []project.Project{{Name: "test", Path: "/tmp/test"}})
	res, err := srv.grep(context.Background(), mcp.CallToolRequest{}, grepArgs{Query: "foo"})
	if err != nil {
		t.Fatalf("grep: %v", err)
	}
	if res == nil || !res.IsError {
		t.Fatalf("expected error result, got %+v", res)
	}
	assertTextEquals(t, res, "project or path is required")
}

func TestGrep(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("func Foo() {}\nfunc Bar() {}\n"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	srv := New("test", "1.0.0", &mockSearcher{}, nil, nil, nil, []project.Project{{Name: "test", Path: dir}})
	res, err := srv.grep(context.Background(), mcp.CallToolRequest{}, grepArgs{Project: "test", Query: "Foo"})
	if err != nil {
		t.Fatalf("grep: %v", err)
	}
	var matches []grepMatch
	if err := json.Unmarshal([]byte(extractText(t, res)), &matches); err != nil {
		t.Fatalf("unmarshal matches: %v", err)
	}
	if len(matches) != 1 || matches[0].Line != 1 {
		t.Errorf("unexpected matches: %+v", matches)
	}
}

func TestGrep_Regex(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("func Foo() {}\nfunc Bar() {}\n"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	srv := New("test", "1.0.0", &mockSearcher{}, nil, nil, nil, []project.Project{{Name: "test", Path: dir}})
	res, err := srv.grep(context.Background(), mcp.CallToolRequest{}, grepArgs{Project: "test", Query: `func [A-Z][a-z]+`, Regex: true})
	if err != nil {
		t.Fatalf("grep: %v", err)
	}
	var matches []grepMatch
	if err := json.Unmarshal([]byte(extractText(t, res)), &matches); err != nil {
		t.Fatalf("unmarshal matches: %v", err)
	}
	if len(matches) != 2 {
		t.Errorf("expected 2 regex matches, got %d", len(matches))
	}
}

func TestDirectoryTree(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub", "file.go"), []byte("x"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	srv := New("test", "1.0.0", &mockSearcher{}, nil, nil, nil, []project.Project{{Name: "test", Path: dir}})
	res, err := srv.directoryTree(context.Background(), mcp.CallToolRequest{}, directoryTreeArgs{Project: "test", Depth: 2})
	if err != nil {
		t.Fatalf("directoryTree: %v", err)
	}
	var tree treeEntry
	if err := json.Unmarshal([]byte(extractText(t, res)), &tree); err != nil {
		t.Fatalf("unmarshal tree: %v", err)
	}
	if tree.Type != "dir" || len(tree.Children) != 1 {
		t.Errorf("unexpected tree: %+v", tree)
	}
}

func TestGetRecentChanges(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("x"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	srv := New("test", "1.0.0", &mockSearcher{}, nil, nil, nil, []project.Project{{Name: "test", Path: dir}})
	res, err := srv.getRecentChanges(context.Background(), mcp.CallToolRequest{}, getRecentChangesArgs{Project: "test", Minutes: 60})
	if err != nil {
		t.Fatalf("getRecentChanges: %v", err)
	}
	var changes []recentChange
	if err := json.Unmarshal([]byte(extractText(t, res)), &changes); err != nil {
		t.Fatalf("unmarshal changes: %v", err)
	}
	if len(changes) != 1 {
		t.Errorf("expected 1 recent change, got %d", len(changes))
	}
}

func TestQueryDocumentation(t *testing.T) {
	mock := &mockSearcher{results: []search.Result{{
		ID:        "doc-1",
		ProjectID: "proj",
		Path:      "/docs/README.md",
		Language:  "markdown",
		Symbol:    "",
		Score:     0.95,
		Content:   "docs",
	}}}
	srv := New("test", "1.0.0", mock, nil, nil, nil, nil)
	res, err := srv.queryDocumentation(context.Background(), mcp.CallToolRequest{}, queryDocumentationArgs{Query: "how to run"})
	if err != nil {
		t.Fatalf("queryDocumentation: %v", err)
	}
	items := extractResultItems(t, res)
	if len(items) != 1 || items[0].Language != "markdown" {
		t.Errorf("unexpected documentation results: %+v", items)
	}
}

func TestGetFileContext_OutsideProject(t *testing.T) {
	srv := New("test", "1.0.0", &mockSearcher{}, nil, nil, nil, []project.Project{{Name: "test", Path: "/tmp"}})
	res, err := srv.getFileContext(context.Background(), mcp.CallToolRequest{}, getFileContextArgs{Path: "/etc/passwd"})
	if err != nil {
		t.Fatalf("getFileContext: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected error result for path outside project")
	}
}

func assertTextEquals(t *testing.T, res *mcp.CallToolResult, want string) {
	t.Helper()
	got := extractText(t, res)
	if !strings.Contains(got, want) {
		t.Errorf("result text = %q, want substring %q", got, want)
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

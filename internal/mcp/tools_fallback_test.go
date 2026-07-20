package mcp

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/quonaro/gnostis/internal/project"
)

func TestDirectoryTree_FallbackToAbsolutePath(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	srv := New("test", "1.0.0", &mockSearcher{}, nil, nil, nil, []project.Project{
		{Name: "other", Path: "/some/other/path"},
	})

	res, err := srv.directoryTree(context.Background(), mcp.CallToolRequest{}, directoryTreeArgs{
		Path:  dir,
		Depth: 1,
	})
	if err != nil {
		t.Fatalf("directoryTree: %v", err)
	}
	if res.IsError {
		t.Fatalf("expected success with fallback, got error: %s", extractText(t, res))
	}
}

func TestListFiles_FallbackToAbsolutePath(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package main"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	srv := New("test", "1.0.0", &mockSearcher{}, nil, nil, nil, []project.Project{
		{Name: "other", Path: "/some/other/path"},
	})

	res, err := srv.listFiles(context.Background(), mcp.CallToolRequest{}, listFilesArgs{
		Path:    dir,
		Pattern: "*.go",
	})
	if err != nil {
		t.Fatalf("listFiles: %v", err)
	}
	if res.IsError {
		t.Fatalf("expected success with fallback, got error: %s", extractText(t, res))
	}
}

func TestGetRecentChanges_FallbackToAbsolutePath(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "recent.go"), []byte("package main"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	srv := New("test", "1.0.0", &mockSearcher{}, nil, nil, nil, []project.Project{
		{Name: "other", Path: "/some/other/path"},
	})

	res, err := srv.getRecentChanges(context.Background(), mcp.CallToolRequest{}, getRecentChangesArgs{
		Path:    dir,
		Minutes: 60,
	})
	if err != nil {
		t.Fatalf("getRecentChanges: %v", err)
	}
	if res.IsError {
		t.Fatalf("expected success with fallback, got error: %s", extractText(t, res))
	}
}

func TestGrep_FallbackToAbsolutePath(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "match.go"), []byte("package main\n\nfunc Foo() {}"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	srv := New("test", "1.0.0", &mockSearcher{}, nil, nil, nil, []project.Project{
		{Name: "other", Path: "/some/other/path"},
	})

	res, err := srv.grep(context.Background(), mcp.CallToolRequest{}, grepArgs{
		Query: "Foo",
		Path:  dir,
	})
	if err != nil {
		t.Fatalf("grep: %v", err)
	}
	if res.IsError {
		t.Fatalf("expected success with fallback, got error: %s", extractText(t, res))
	}
}

func TestGetFileContext_FallbackToAbsolutePath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ctx.txt")
	if err := os.WriteFile(path, []byte("line1\nline2\nline3\n"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	srv := New("test", "1.0.0", &mockSearcher{}, nil, nil, nil, []project.Project{
		{Name: "other", Path: "/some/other/path"},
	})

	res, err := srv.getFileContext(context.Background(), mcp.CallToolRequest{}, getFileContextArgs{
		Path:      path,
		StartLine: 1,
		EndLine:   2,
	})
	if err != nil {
		t.Fatalf("getFileContext: %v", err)
	}
	if res.IsError {
		t.Fatalf("expected success with fallback, got error: %s", extractText(t, res))
	}
	if extractText(t, res) != "line1\nline2" {
		t.Errorf("unexpected content: %q", extractText(t, res))
	}
}

func TestResolvePathOrAbsolute_NoFallbackForRelative(t *testing.T) {
	srv := New("test", "1.0.0", &mockSearcher{}, nil, nil, nil, []project.Project{
		{Name: "other", Path: "/some/other/path"},
	})

	_, err := srv.resolvePathOrAbsolute("", "relative/path")
	if err == nil {
		t.Fatal("expected error for relative path without project")
	}
}

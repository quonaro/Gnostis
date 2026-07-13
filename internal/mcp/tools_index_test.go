package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/quonaro/gnostis/internal/project"
)

type mockReindexer struct {
	paths []string
	err   error
}

func (m *mockReindexer) ReindexFiles(ctx context.Context, paths []string) error {
	m.paths = append(m.paths, paths...)
	return m.err
}

func TestReindexFiles_NoPaths(t *testing.T) {
	srv := New("test", "1.0.0", &mockSearcher{}, nil, &mockReindexer{}, nil)
	res, err := srv.reindexFiles(context.Background(), mcp.CallToolRequest{}, reindexFilesArgs{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected error result for empty paths")
	}
	assertTextEquals(t, res, "paths is required")
}

func TestReindexFiles_NotConfigured(t *testing.T) {
	srv := New("test", "1.0.0", &mockSearcher{}, nil, nil, nil)
	res, err := srv.reindexFiles(context.Background(), mcp.CallToolRequest{}, reindexFilesArgs{Paths: []string{"/foo"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected error result when reindexer is not configured")
	}
}

func TestReindexFiles_RelativePath(t *testing.T) {
	srv := New("test", "1.0.0", &mockSearcher{}, nil, &mockReindexer{}, []project.Project{{Name: "test", Path: "/tmp"}})
	res, err := srv.reindexFiles(context.Background(), mcp.CallToolRequest{}, reindexFilesArgs{Paths: []string{"relative/path.go"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected error result for relative path")
	}
}

func TestReindexFiles_OutsideProject(t *testing.T) {
	dir := t.TempDir()
	outsideDir := t.TempDir()
	path := filepath.Join(outsideDir, "sample.go")
	if err := os.WriteFile(path, []byte("package main"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	mock := &mockReindexer{}
	srv := New("test", "1.0.0", &mockSearcher{}, nil, mock, []project.Project{{Name: "test", Path: dir}})
	res, err := srv.reindexFiles(context.Background(), mcp.CallToolRequest{}, reindexFilesArgs{Paths: []string{path}})
	if err != nil {
		t.Fatalf("reindexFiles: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected error result for path outside project")
	}
}

func TestReindexFiles_Directory(t *testing.T) {
	dir := t.TempDir()
	mock := &mockReindexer{}
	srv := New("test", "1.0.0", &mockSearcher{}, nil, mock, []project.Project{{Name: "test", Path: dir}})
	res, err := srv.reindexFiles(context.Background(), mcp.CallToolRequest{}, reindexFilesArgs{Paths: []string{dir}})
	if err != nil {
		t.Fatalf("reindexFiles: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error result: %v", res.Content)
	}

	var got reindexFilesResult
	if err := json.Unmarshal([]byte(extractText(t, res)), &got); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if len(got.Reindexed) != 1 || got.Reindexed[0] != dir {
		t.Errorf("unexpected reindexed paths: %+v", got)
	}
	if len(mock.paths) != 1 || mock.paths[0] != dir {
		t.Errorf("unexpected paths passed to reindexer: %+v", mock.paths)
	}
}

func TestReindexFiles_OK(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.go")
	if err := os.WriteFile(path, []byte("package main"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	mock := &mockReindexer{}
	srv := New("test", "1.0.0", &mockSearcher{}, nil, mock, []project.Project{{Name: "test", Path: dir}})
	res, err := srv.reindexFiles(context.Background(), mcp.CallToolRequest{}, reindexFilesArgs{Paths: []string{path}})
	if err != nil {
		t.Fatalf("reindexFiles: %v", err)
	}

	var got reindexFilesResult
	if err := json.Unmarshal([]byte(extractText(t, res)), &got); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if len(got.Reindexed) != 1 || got.Reindexed[0] != path {
		t.Errorf("unexpected reindexed paths: %+v", got)
	}
	if len(mock.paths) != 1 || mock.paths[0] != path {
		t.Errorf("unexpected paths passed to reindexer: %+v", mock.paths)
	}
}

func TestReindexFiles_ReindexerError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.go")
	if err := os.WriteFile(path, []byte("package main"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	mock := &mockReindexer{err: errors.New("boom")}
	srv := New("test", "1.0.0", &mockSearcher{}, nil, mock, []project.Project{{Name: "test", Path: dir}})
	_, err := srv.reindexFiles(context.Background(), mcp.CallToolRequest{}, reindexFilesArgs{Paths: []string{path}})
	if err == nil {
		t.Fatal("expected error from reindexer")
	}
}

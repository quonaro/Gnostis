package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/quonaro/gnostis/internal/discover"
	"github.com/quonaro/gnostis/internal/progress"
	"github.com/quonaro/gnostis/internal/project"
	"github.com/quonaro/gnostis/internal/stats"
)

type mockIndexer struct {
	paths          []string
	err            error
	discoverResult discover.Result
	discoverCalled bool
	progressState  progress.State
}

func (m *mockIndexer) Status() ([]string, int)     { return nil, 0 }
func (m *mockIndexer) Info() (string, string, int) { return "", "", 0 }
func (m *mockIndexer) ProgressState() (progress.State, error) {
	if m.progressState.Status != "" {
		return m.progressState, nil
	}
	return progress.State{JobID: "job-1", Status: progress.StatusIdle}, nil
}
func (m *mockIndexer) ProjectStats(context.Context) (map[string]stats.Project, error) {
	return nil, nil
}
func (m *mockIndexer) StartRebuildProject(context.Context, string) (string, error) {
	return "job-1", nil
}
func (m *mockIndexer) StartRebuildIndex(context.Context) (string, error) { return "job-1", nil }
func (m *mockIndexer) DiscoverProjects(context.Context, string, discover.Options) (discover.Result, error) {
	m.discoverCalled = true
	return m.discoverResult, nil
}
func (m *mockIndexer) AddProject(context.Context, string, string) error { return nil }
func (m *mockIndexer) RemoveProject(context.Context, string) error      { return nil }

func (m *mockIndexer) ReindexFiles(ctx context.Context, paths []string) error {
	m.paths = append(m.paths, paths...)
	return m.err
}

func TestReindexFiles_NoPaths(t *testing.T) {
	srv := New("test", "1.0.0", &mockSearcher{}, nil, &mockIndexer{}, nil, nil)
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
	srv := New("test", "1.0.0", &mockSearcher{}, nil, nil, nil, nil)
	res, err := srv.reindexFiles(context.Background(), mcp.CallToolRequest{}, reindexFilesArgs{Paths: []string{"/foo"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected error result when indexer is not configured")
	}
}

func TestReindexFiles_RelativePath(t *testing.T) {
	srv := New("test", "1.0.0", &mockSearcher{}, nil, &mockIndexer{}, nil, []project.Project{{Name: "test", Path: "/tmp"}})
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

	mock := &mockIndexer{}
	srv := New("test", "1.0.0", &mockSearcher{}, nil, mock, nil, []project.Project{{Name: "test", Path: dir}})
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
	mock := &mockIndexer{}
	srv := New("test", "1.0.0", &mockSearcher{}, nil, mock, nil, []project.Project{{Name: "test", Path: dir}})
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
		t.Errorf("unexpected paths passed to indexer: %+v", mock.paths)
	}
}

func TestReindexFiles_OK(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.go")
	if err := os.WriteFile(path, []byte("package main"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	mock := &mockIndexer{}
	srv := New("test", "1.0.0", &mockSearcher{}, nil, mock, nil, []project.Project{{Name: "test", Path: dir}})
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
		t.Errorf("unexpected paths passed to indexer: %+v", mock.paths)
	}
}

func TestReindexFiles_IndexerError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.go")
	if err := os.WriteFile(path, []byte("package main"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	mock := &mockIndexer{err: errors.New("boom")}
	srv := New("test", "1.0.0", &mockSearcher{}, nil, mock, nil, []project.Project{{Name: "test", Path: dir}})
	_, err := srv.reindexFiles(context.Background(), mcp.CallToolRequest{}, reindexFilesArgs{Paths: []string{path}})
	if err == nil {
		t.Fatal("expected error from indexer")
	}
}

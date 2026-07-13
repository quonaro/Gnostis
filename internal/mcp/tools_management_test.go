package mcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/quonaro/gnostis/internal/discover"
	"github.com/quonaro/gnostis/internal/progress"
	"github.com/quonaro/gnostis/internal/project"
)

func TestGetIndexStatus(t *testing.T) {
	mock := &mockIndexer{}
	srv := New("test", "1.0.0", &mockSearcher{}, nil, mock, nil)
	res, err := srv.getIndexStatus(context.Background(), mcp.CallToolRequest{}, getIndexStatusArgs{})
	if err != nil {
		t.Fatalf("getIndexStatus: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error result: %v", res.Content)
	}
	text := extractText(t, res)
	if text == "" {
		t.Fatal("expected non-empty status")
	}
}

func TestGetIndexJob(t *testing.T) {
	mock := &mockIndexer{}
	srv := New("test", "1.0.0", &mockSearcher{}, nil, mock, nil)
	res, err := srv.getIndexJob(context.Background(), mcp.CallToolRequest{}, getIndexJobArgs{JobID: "job-1"})
	if err != nil {
		t.Fatalf("getIndexJob: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error result: %v", res.Content)
	}

	var state map[string]interface{}
	if err := json.Unmarshal([]byte(extractText(t, res)), &state); err != nil {
		t.Fatalf("unmarshal job state: %v", err)
	}
	if state["job_id"] != "job-1" {
		t.Errorf("job_id = %v, want job-1", state["job_id"])
	}
}

func TestRebuildProject(t *testing.T) {
	mock := &mockIndexer{}
	srv := New("test", "1.0.0", &mockSearcher{}, nil, mock, nil)
	res, err := srv.rebuildProject(context.Background(), mcp.CallToolRequest{}, rebuildProjectArgs{Project: "foo"})
	if err != nil {
		t.Fatalf("rebuildProject: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error result: %v", res.Content)
	}
	if extractText(t, res) != `{"job_id":"job-1"}` {
		t.Errorf("unexpected result: %s", extractText(t, res))
	}
}

func TestRebuildIndex(t *testing.T) {
	mock := &mockIndexer{}
	srv := New("test", "1.0.0", &mockSearcher{}, nil, mock, nil)
	res, err := srv.rebuildIndex(context.Background(), mcp.CallToolRequest{}, rebuildIndexArgs{})
	if err != nil {
		t.Fatalf("rebuildIndex: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error result: %v", res.Content)
	}
	if extractText(t, res) != `{"job_id":"job-1"}` {
		t.Errorf("unexpected result: %s", extractText(t, res))
	}
}

func TestDiscoverProjects(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	mock := &mockIndexer{}
	srv := New("test", "1.0.0", &mockSearcher{}, nil, mock, []project.Project{{Name: "test", Path: dir}})
	res, err := srv.discoverProjects(context.Background(), mcp.CallToolRequest{}, discoverProjectsArgs{Path: dir, Depth: 1})
	if err != nil {
		t.Fatalf("discoverProjects: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error result: %v", res.Content)
	}
	if !mock.discoverCalled {
		t.Fatal("expected DiscoverProjects to be called")
	}
}

func TestAddProject(t *testing.T) {
	mock := &mockIndexer{}
	srv := New("test", "1.0.0", &mockSearcher{}, nil, mock, nil)
	res, err := srv.addProject(context.Background(), mcp.CallToolRequest{}, addProjectArgs{Path: "/tmp/foo", Name: "foo"})
	if err != nil {
		t.Fatalf("addProject: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error result: %v", res.Content)
	}
	if extractText(t, res) != `{"added":true}` {
		t.Errorf("unexpected result: %s", extractText(t, res))
	}
}

func TestRemoveProject(t *testing.T) {
	mock := &mockIndexer{}
	srv := New("test", "1.0.0", &mockSearcher{}, nil, mock, nil)
	res, err := srv.removeProject(context.Background(), mcp.CallToolRequest{}, removeProjectArgs{Name: "foo", Confirm: true})
	if err != nil {
		t.Fatalf("removeProject: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error result: %v", res.Content)
	}
	if extractText(t, res) != `{"removed":true}` {
		t.Errorf("unexpected result: %s", extractText(t, res))
	}
}

func TestDiscoverProjectsResult(t *testing.T) {
	dir := t.TempDir()
	mock := &mockIndexer{
		discoverResult: discover.Result{
			New:          []discover.Project{{Name: "new", Path: "/new"}},
			AlreadyAdded: []discover.Project{{Name: "old", Path: "/old"}},
		},
	}
	srv := New("test", "1.0.0", &mockSearcher{}, nil, mock, nil)
	res, err := srv.discoverProjects(context.Background(), mcp.CallToolRequest{}, discoverProjectsArgs{Path: dir, Depth: 1})
	if err != nil {
		t.Fatalf("discoverProjects: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error result: %v", res.Content)
	}

	var result discover.Result
	if err := json.Unmarshal([]byte(extractText(t, res)), &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if len(result.New) != 1 || result.New[0].Name != "new" {
		t.Errorf("unexpected new projects: %+v", result.New)
	}
	if len(result.AlreadyAdded) != 1 || result.AlreadyAdded[0].Name != "old" {
		t.Errorf("unexpected already added projects: %+v", result.AlreadyAdded)
	}
}

func TestGetIndexStatusETA(t *testing.T) {
	now := time.Now().UTC()
	mock := &mockIndexer{
		progressState: progress.State{
			JobID:       "job-eta",
			Status:      progress.StatusRunning,
			Phase:       progress.PhaseEmbedding,
			Project:     "test",
			StartedAt:   now.Add(-2 * time.Minute),
			UpdatedAt:   now,
			TotalChunks: 1000,
			DoneChunks:  100,
		},
	}
	srv := New("test", "1.0.0", &mockSearcher{}, nil, mock, nil)
	res, err := srv.getIndexStatus(context.Background(), mcp.CallToolRequest{}, getIndexStatusArgs{})
	if err != nil {
		t.Fatalf("getIndexStatus: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error result: %v", res.Content)
	}

	var result indexStatusResult
	if err := json.Unmarshal([]byte(extractText(t, res)), &result); err != nil {
		t.Fatalf("unmarshal status: %v", err)
	}
	if result.ETA == "" {
		t.Errorf("expected non-empty ETA, got empty")
	}
	if result.ETASeconds <= 0 {
		t.Errorf("expected positive ETA seconds, got %d", result.ETASeconds)
	}
	if result.Progress.Project != "test" {
		t.Errorf("progress project = %q, want test", result.Progress.Project)
	}
}

package mcp

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/quonaro/gnostis/internal/progress"
)

func TestQueryDocumentation_IndexNotReady(t *testing.T) {
	mock := &mockIndexer{
		progressState: progress.State{
			JobID:  "job-1",
			Status: progress.StatusRunning,
			Phase:  progress.PhaseEmbedding,
		},
	}
	srv := New("test", "1.0.0", &mockSearcher{}, nil, mock, nil, nil)

	res, err := srv.queryDocumentation(context.Background(), mcp.CallToolRequest{}, queryDocumentationArgs{
		Query: "test query",
	})
	if err != nil {
		t.Fatalf("queryDocumentation: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected error result when index is not ready")
	}
	assertTextEquals(t, res, "index is still being built")
}

func TestQueryDocumentation_EmptyResults_IndexIdle(t *testing.T) {
	mock := &mockIndexer{}
	srv := New("test", "1.0.0", &mockSearcher{}, nil, mock, nil, nil)

	res, err := srv.queryDocumentation(context.Background(), mcp.CallToolRequest{}, queryDocumentationArgs{
		Query: "test query",
	})
	if err != nil {
		t.Fatalf("queryDocumentation: %v", err)
	}
	if res.IsError {
		t.Fatalf("expected empty results, not error: %s", extractText(t, res))
	}
	if extractText(t, res) != "[]" {
		t.Errorf("expected empty array, got %s", extractText(t, res))
	}
}

func TestSearchCodebase_IndexNotReady(t *testing.T) {
	mock := &mockIndexer{
		progressState: progress.State{
			JobID:  "job-1",
			Status: progress.StatusRunning,
			Phase:  progress.PhaseEmbedding,
		},
	}
	srv := New("test", "1.0.0", &mockSearcher{}, nil, mock, nil, nil)

	res, err := srv.searchCodebase(context.Background(), mcp.CallToolRequest{}, searchCodebaseArgs{
		Query: "test query",
	})
	if err != nil {
		t.Fatalf("searchCodebase: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected error result when index is not ready")
	}
	assertTextEquals(t, res, "index is still being built")
}

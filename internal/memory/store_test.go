package memory

import (
	"context"
	"testing"

	"github.com/quonaro/gnostis/internal/chunker"
)

func TestStoreAddAndQuery(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	s, err := NewStore(ctx, dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	chunks := []chunker.Chunk{
		{ID: "c1", ProjectID: "memory-cascade", Path: "/tmp/a.md", Content: "hello world", FileHash: "h1", StartLine: 1, EndLine: 1},
	}
	vectors := [][]float32{{1, 0, 0, 0}}
	if err := s.AddChunks(ctx, chunks, vectors); err != nil {
		t.Fatalf("AddChunks: %v", err)
	}

	if got := s.Count(); got != 1 {
		t.Fatalf("Count = %d, want 1", got)
	}

	results, err := s.Query(ctx, []float32{1, 0, 0, 0}, 10, nil)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("Query results = %d, want 1", len(results))
	}
	if results[0].Content != "hello world" {
		t.Errorf("Content = %q, want %q", results[0].Content, "hello world")
	}
}

func TestStoreDeleteByPath(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	s, err := NewStore(ctx, dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	chunks := []chunker.Chunk{
		{ID: "c1", ProjectID: "memory-cascade", Path: "/tmp/a.md", Content: "hello", FileHash: "h1", StartLine: 1, EndLine: 1},
	}
	if err := s.AddChunks(ctx, chunks, [][]float32{{1, 0, 0, 0}}); err != nil {
		t.Fatalf("AddChunks: %v", err)
	}
	if err := s.DeleteByPath(ctx, "/tmp/a.md"); err != nil {
		t.Fatalf("DeleteByPath: %v", err)
	}
	if got := s.Count(); got != 0 {
		t.Fatalf("Count after delete = %d, want 0", got)
	}
}

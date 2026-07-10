package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/quonaro/gnostis/internal/chunker"
)

func TestStore_GetFileHash(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	s, err := New(ctx, dir)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	hash, err := s.GetFileHash(ctx, "/tmp/foo.go")
	if err != nil {
		t.Fatalf("get file hash: %v", err)
	}
	if hash != "" {
		t.Fatalf("expected empty hash for unknown path, got %q", hash)
	}

	chunks := []chunker.Chunk{
		{ID: "c1", Path: "/tmp/foo.go", FileHash: "abc", Content: "func main() {}"},
	}
	embeddings := [][]float32{{0.1, 0.2, 0.3}}
	if err := s.AddChunks(ctx, chunks, embeddings); err != nil {
		t.Fatalf("add chunks: %v", err)
	}

	hash, err = s.GetFileHash(ctx, "/tmp/foo.go")
	if err != nil {
		t.Fatalf("get file hash after add: %v", err)
	}
	if hash != "abc" {
		t.Fatalf("expected hash %q, got %q", "abc", hash)
	}

	if err := s.DeleteByPath(ctx, "/tmp/foo.go"); err != nil {
		t.Fatalf("delete by path: %v", err)
	}

	hash, err = s.GetFileHash(ctx, "/tmp/foo.go")
	if err != nil {
		t.Fatalf("get file hash after delete: %v", err)
	}
	if hash != "" {
		t.Fatalf("expected empty hash after delete, got %q", hash)
	}
}

func TestStore_HashPersistence(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	s, err := New(ctx, dir)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	chunks := []chunker.Chunk{
		{ID: "c1", Path: "/tmp/bar.go", FileHash: "xyz", Content: "package bar"},
	}
	if err := s.AddChunks(ctx, chunks, [][]float32{{0.1, 0.2, 0.3}}); err != nil {
		t.Fatalf("add chunks: %v", err)
	}

	s2, err := New(ctx, dir)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}

	hash, err := s2.GetFileHash(ctx, "/tmp/bar.go")
	if err != nil {
		t.Fatalf("get file hash after reopen: %v", err)
	}
	if hash != "xyz" {
		t.Fatalf("expected hash %q after reopen, got %q", "xyz", hash)
	}

	if _, err := os.Stat(filepath.Join(dir, hashFileName)); err != nil {
		t.Fatalf("hash file should exist: %v", err)
	}
}

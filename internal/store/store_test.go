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

func TestStore_AddChunks_DimensionMismatch(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	s, err := New(ctx, dir)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	chunks := []chunker.Chunk{
		{ID: "c1", Path: "/tmp/a.go", FileHash: "h1", Content: "package a"},
		{ID: "c2", Path: "/tmp/b.go", FileHash: "h2", Content: "package b"},
	}
	if err := s.AddChunks(ctx, chunks, [][]float32{{0.1, 0.2}, {0.3, 0.4, 0.5}}); err == nil {
		t.Fatalf("expected dimension mismatch error")
	}
}

func TestStore_AddChunks_DimensionMismatchAgainstExisting(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	s, err := New(ctx, dir)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	if err := s.AddChunks(ctx, []chunker.Chunk{
		{ID: "c1", Path: "/tmp/a.go", FileHash: "h1", Content: "package a"},
	}, [][]float32{{0.1, 0.2, 0.3}}); err != nil {
		t.Fatalf("add initial chunks: %v", err)
	}

	if err := s.AddChunks(ctx, []chunker.Chunk{
		{ID: "c2", Path: "/tmp/b.go", FileHash: "h2", Content: "package b"},
	}, [][]float32{{0.1, 0.2}}); err == nil {
		t.Fatalf("expected dimension mismatch against existing store")
	}
}

func TestStore_Query_DimensionMismatch(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	s, err := New(ctx, dir)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	if err := s.AddChunks(ctx, []chunker.Chunk{
		{ID: "c1", Path: "/tmp/a.go", FileHash: "h1", Content: "package a"},
	}, [][]float32{{0.1, 0.2, 0.3}}); err != nil {
		t.Fatalf("add chunks: %v", err)
	}

	_, err = s.Query(ctx, []float32{0.1, 0.2}, 1, nil)
	if err == nil {
		t.Fatalf("expected query dimension mismatch error")
	}
}

func TestStore_CountByProject(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	s, err := New(ctx, dir)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	chunks := []chunker.Chunk{
		{ID: "c1", ProjectID: "alpha", Path: "/tmp/a.go", FileHash: "h1", Content: "package a"},
		{ID: "c2", ProjectID: "alpha", Path: "/tmp/b.go", FileHash: "h2", Content: "package b"},
		{ID: "c3", ProjectID: "beta", Path: "/tmp/c.go", FileHash: "h3", Content: "package c"},
	}
	if err := s.AddChunks(ctx, chunks, [][]float32{{0.1, 0.2}, {0.2, 0.3}, {0.3, 0.4}}); err != nil {
		t.Fatalf("add chunks: %v", err)
	}

	alpha, err := s.CountByProject(ctx, "alpha")
	if err != nil {
		t.Fatalf("count alpha: %v", err)
	}
	if alpha != 2 {
		t.Fatalf("expected alpha count 2, got %d", alpha)
	}

	beta, err := s.CountByProject(ctx, "beta")
	if err != nil {
		t.Fatalf("count beta: %v", err)
	}
	if beta != 1 {
		t.Fatalf("expected beta count 1, got %d", beta)
	}

	gamma, err := s.CountByProject(ctx, "gamma")
	if err != nil {
		t.Fatalf("count gamma: %v", err)
	}
	if gamma != 0 {
		t.Fatalf("expected gamma count 0, got %d", gamma)
	}
}

func TestStore_DimPersistence(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	s, err := New(ctx, dir)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	if err := s.AddChunks(ctx, []chunker.Chunk{
		{ID: "c1", Path: "/tmp/a.go", FileHash: "h1", Content: "package a"},
	}, [][]float32{{0.1, 0.2, 0.3, 0.4}}); err != nil {
		t.Fatalf("add chunks: %v", err)
	}

	s2, err := New(ctx, dir)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}

	_, err = s2.Query(ctx, []float32{0.1, 0.2, 0.3}, 1, nil)
	if err == nil {
		t.Fatalf("expected persisted dimension mismatch error after reopen")
	}
}

package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	chromem "github.com/philippgille/chromem-go"

	"github.com/quonaro/gnostis/internal/chunker"
)

const (
	collectionName = "memory_chunks"
	hashFileName   = "file_hashes.json"
	dimFileName    = "embedding_dim.json"
)

// Store persists memory chunks in a dedicated chromem-go collection separate
// from the project code index.
type Store struct {
	mu       sync.RWMutex
	col      *chromem.Collection
	hashFile string
	dimFile  string
	hashes   map[string]string
	dim      int
}

// NewStore opens or creates a persistent chromem-go database for memory.
func NewStore(ctx context.Context, dataDir string) (*Store, error) {
	slog.InfoContext(ctx, "opening memory store", "data_dir", dataDir)
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("create memory data dir: %w", err)
	}

	db, err := chromem.NewPersistentDB(dataDir, false)
	if err != nil {
		return nil, fmt.Errorf("open chromem db: %w", err)
	}

	embedFn := func(ctx context.Context, text string) ([]float32, error) {
		return nil, fmt.Errorf("embeddings must be provided explicitly")
	}

	col, err := db.GetOrCreateCollection(collectionName, nil, embedFn)
	if err != nil {
		return nil, fmt.Errorf("create memory collection: %w", err)
	}

	s := &Store{
		col:      col,
		hashFile: filepath.Join(dataDir, hashFileName),
		dimFile:  filepath.Join(dataDir, dimFileName),
		hashes:   make(map[string]string),
	}
	if err := s.loadHashes(); err != nil {
		return nil, fmt.Errorf("load memory file hashes: %w", err)
	}
	if err := s.loadDim(); err != nil {
		return nil, fmt.Errorf("load memory embedding dimension: %w", err)
	}

	return s, nil
}

// AddChunks stores chunks with their precomputed embeddings.
func (s *Store) AddChunks(ctx context.Context, chunks []chunker.Chunk, embeddings [][]float32) error {
	if len(chunks) == 0 {
		return nil
	}
	if len(chunks) != len(embeddings) {
		return fmt.Errorf("chunks (%d) and embeddings (%d) length mismatch", len(chunks), len(embeddings))
	}

	dim, err := validateEmbeddings(embeddings)
	if err != nil {
		return fmt.Errorf("validate embeddings: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.dim != 0 && s.dim != dim {
		return fmt.Errorf("embedding dimension mismatch: expected %d, got %d; clear the memory data directory and restart after changing the embedding model", s.dim, dim)
	}

	ids := make([]string, len(chunks))
	contents := make([]string, len(chunks))
	metadatas := make([]map[string]string, len(chunks))

	for i, ch := range chunks {
		ids[i] = ch.ID
		contents[i] = ch.Content
		metadatas[i] = chunkMetadata(ch)
		if ch.FileHash != "" {
			s.hashes[ch.Path] = ch.FileHash
		}
	}

	if err := s.col.Add(ctx, ids, embeddings, metadatas, contents); err != nil {
		return fmt.Errorf("add memory chunks: %w", err)
	}
	if s.dim == 0 {
		s.dim = dim
		if err := s.saveDim(); err != nil {
			return fmt.Errorf("save memory embedding dimension: %w", err)
		}
	}
	if err := s.saveHashes(); err != nil {
		return fmt.Errorf("save memory file hashes: %w", err)
	}

	return nil
}

// Query searches the memory store with a precomputed embedding and optional metadata filters.
func (s *Store) Query(ctx context.Context, embedding []float32, n int, filters map[string]string) ([]chromem.Result, error) {
	s.mu.RLock()
	count := s.col.Count()
	if count == 0 {
		s.mu.RUnlock()
		return nil, nil
	}
	if n > count {
		n = count
	}
	if s.dim != 0 && len(embedding) != s.dim {
		s.mu.RUnlock()
		return nil, fmt.Errorf("embedding dimension mismatch: query has %d dimensions but the memory store was indexed with %d dimensions; clear the memory data directory and restart after changing the embedding model", len(embedding), s.dim)
	}
	results, err := s.col.QueryEmbedding(ctx, embedding, n, filters, nil)
	s.mu.RUnlock()
	if err != nil {
		return nil, fmt.Errorf("query memory store: %w", err)
	}
	return results, nil
}

// DeleteByPath removes all memory chunks belonging to a file.
func (s *Store) DeleteByPath(ctx context.Context, path string) error {
	return s.DeleteByPaths(ctx, []string{path})
}

// DeleteByPaths removes all memory chunks for the given paths in a single batch.
func (s *Store) DeleteByPaths(ctx context.Context, paths []string) error {
	if len(paths) == 0 {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, path := range paths {
		if err := s.col.Delete(ctx, map[string]string{"path": path}, nil); err != nil {
			return fmt.Errorf("delete memory path %s: %w", path, err)
		}
		delete(s.hashes, path)
	}
	if err := s.saveHashes(); err != nil {
		return fmt.Errorf("save memory file hashes: %w", err)
	}
	return nil
}

// GetFileHash returns the stored hash for a memory file path.
func (s *Store) GetFileHash(_ context.Context, path string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.hashes[path], nil
}

// Paths returns all memory file paths currently tracked by the store.
func (s *Store) Paths() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	paths := make([]string, 0, len(s.hashes))
	for p := range s.hashes {
		paths = append(paths, p)
	}
	return paths
}

// Count returns the number of stored memory chunks.
func (s *Store) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.col.Count()
}

func validateEmbeddings(vectors [][]float32) (int, error) {
	if len(vectors) == 0 {
		return 0, fmt.Errorf("no embeddings")
	}
	dim := len(vectors[0])
	if dim == 0 {
		return 0, fmt.Errorf("empty embedding vector")
	}
	for i, v := range vectors {
		if len(v) != dim {
			return 0, fmt.Errorf("embedding at index %d has length %d, expected %d", i, len(v), dim)
		}
	}
	return dim, nil
}

func chunkMetadata(ch chunker.Chunk) map[string]string {
	return map[string]string{
		"project_id": ch.ProjectID,
		"path":       ch.Path,
		"file_hash":  ch.FileHash,
		"language":   ch.Language,
		"symbol":     ch.Symbol,
		"signature":  ch.Signature,
		"start_line": strconv.Itoa(ch.StartLine),
		"end_line":   strconv.Itoa(ch.EndLine),
	}
}

func (s *Store) loadHashes() error {
	data, err := os.ReadFile(s.hashFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read memory hash file: %w", err)
	}
	if err := json.Unmarshal(data, &s.hashes); err != nil {
		return fmt.Errorf("parse memory hash file: %w", err)
	}
	return nil
}

func (s *Store) saveHashes() error {
	data, err := json.Marshal(s.hashes)
	if err != nil {
		return fmt.Errorf("marshal memory hashes: %w", err)
	}
	if err := os.WriteFile(s.hashFile, data, 0o600); err != nil {
		return fmt.Errorf("write memory hash file: %w", err)
	}
	return nil
}

func (s *Store) loadDim() error {
	data, err := os.ReadFile(s.dimFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read memory dimension file: %w", err)
	}
	var dim int
	if err := json.Unmarshal(data, &dim); err != nil {
		return fmt.Errorf("parse memory dimension file: %w", err)
	}
	s.dim = dim
	return nil
}

func (s *Store) saveDim() error {
	data, err := json.Marshal(s.dim)
	if err != nil {
		return fmt.Errorf("marshal memory dimension: %w", err)
	}
	if err := os.WriteFile(s.dimFile, data, 0o600); err != nil {
		return fmt.Errorf("write memory dimension file: %w", err)
	}
	return nil
}

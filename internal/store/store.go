package store

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	chromem "github.com/philippgille/chromem-go"

	"github.com/quonaro/gnostis/internal/chunker"
)

const collectionName = "code_chunks"
const hashFileName = "file_hashes.json"
const dimFileName = "embedding_dim.json"

// VectorStore is the storage interface consumed by the search engine and indexer.
// It is implemented by the default chromem-backed Store and can be implemented by
// other backends in the future (FAISS, Qdrant, Postgres, etc.).
type VectorStore interface {
	AddChunks(ctx context.Context, chunks []chunker.Chunk, embeddings [][]float32) error
	Query(ctx context.Context, embedding []float32, n int, filters map[string]string) ([]chromem.Result, error)
	DeleteByPath(ctx context.Context, path string) error
	DeleteByPaths(ctx context.Context, paths []string) error
	GetFileHash(ctx context.Context, path string) (string, error)
	Paths() []string
	Count() int
	CountByProject(ctx context.Context, projectID string) (int, error)
}

// Store persists chunks in chromem-go and tracks file hashes for incremental indexing.
// Store methods are safe for concurrent use.
type Store struct {
	mu       sync.RWMutex
	col      *chromem.Collection
	hashFile string
	dimFile  string
	hashes   map[string]string
	dim      int
}

// compile-time check that Store implements VectorStore.
var _ VectorStore = (*Store)(nil)

// New opens or creates a persistent chromem-go database.
func New(ctx context.Context, dataDir string) (*Store, error) {
	slog.InfoContext(ctx, "opening store", "data_dir", dataDir)
	db, err := chromem.NewPersistentDB(dataDir, false)
	if err != nil {
		return nil, fmt.Errorf("open chromem db: %w", err)
	}

	embedFn := func(ctx context.Context, text string) ([]float32, error) {
		return nil, fmt.Errorf("embeddings must be provided explicitly")
	}

	col, err := db.GetOrCreateCollection(collectionName, nil, embedFn)
	if err != nil {
		return nil, fmt.Errorf("create collection: %w", err)
	}

	s := &Store{
		col:      col,
		hashFile: filepath.Join(dataDir, hashFileName),
		dimFile:  filepath.Join(dataDir, dimFileName),
		hashes:   make(map[string]string),
	}
	if err := s.loadHashes(); err != nil {
		return nil, fmt.Errorf("load file hashes: %w", err)
	}
	if err := s.loadDim(); err != nil {
		return nil, fmt.Errorf("load embedding dimension: %w", err)
	}

	return s, nil
}

// AddChunks stores chunks with their precomputed embeddings.
func (s *Store) AddChunks(ctx context.Context, chunks []chunker.Chunk, embeddings [][]float32) error {
	if len(chunks) == 0 {
		return nil
	}
	slog.DebugContext(ctx, "adding chunks", "count", len(chunks))
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
		return fmt.Errorf("embedding dimension mismatch: expected %d, got %d; clear the data directory and restart after changing the embedding model", s.dim, dim)
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
		return fmt.Errorf("add chunks: %w", err)
	}
	if s.dim == 0 {
		s.dim = dim
		if err := s.saveDim(); err != nil {
			return fmt.Errorf("save embedding dimension: %w", err)
		}
	}
	if err := s.saveHashes(); err != nil {
		return fmt.Errorf("save file hashes: %w", err)
	}
	slog.DebugContext(ctx, "added chunks", "count", len(chunks), "total", s.col.Count())

	return nil
}

// validateEmbeddings checks that all vectors are non-empty and share the same length.
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

// DeleteByPath removes all chunks belonging to a file.
func (s *Store) DeleteByPath(ctx context.Context, path string) error {
	return s.DeleteByPaths(ctx, []string{path})
}

// DeleteByPaths removes all chunks for the given paths in a single batch.
func (s *Store) DeleteByPaths(ctx context.Context, paths []string) error {
	if len(paths) == 0 {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, path := range paths {
		if err := s.col.Delete(ctx, map[string]string{"path": path}, nil); err != nil {
			return fmt.Errorf("delete path %s: %w", path, err)
		}
		delete(s.hashes, path)
	}
	if err := s.saveHashes(); err != nil {
		return fmt.Errorf("save file hashes: %w", err)
	}
	return nil
}

// Query searches the vector store with a precomputed embedding.
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
		return nil, fmt.Errorf("embedding dimension mismatch: query has %d dimensions but the store was indexed with %d dimensions; clear the data directory and restart after changing the embedding model", len(embedding), s.dim)
	}
	results, err := s.col.QueryEmbedding(ctx, embedding, n, filters, nil)
	s.mu.RUnlock()
	if err != nil {
		if strings.Contains(err.Error(), "vectors must have the same length") {
			return nil, fmt.Errorf("embedding dimension mismatch: query vector length differs from stored vectors; clear the data directory and restart after changing the embedding model")
		}
		return nil, fmt.Errorf("query: %w", err)
	}

	if s.dim == 0 && len(results) > 0 {
		s.mu.Lock()
		if s.dim == 0 {
			s.dim = len(embedding)
			_ = s.saveDim()
		}
		s.mu.Unlock()
	}

	return results, nil
}

// Count returns the number of stored chunks.
func (s *Store) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.col.Count()
}

// CountByProject returns the number of chunks belonging to the given project.
func (s *Store) CountByProject(ctx context.Context, projectID string) (int, error) {
	if projectID == "" {
		return 0, nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.dim == 0 || s.col.Count() == 0 {
		return 0, nil
	}

	// Use a normalized basis vector so every filtered document is returned.
	query := make([]float32, s.dim)
	query[0] = 1

	results, err := s.col.QueryEmbedding(ctx, query, s.col.Count(), map[string]string{"project_id": projectID}, nil)
	if err != nil {
		return 0, fmt.Errorf("query project %q: %w", projectID, err)
	}
	return len(results), nil
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

// GetFileHash returns the stored hash for a file path.
func (s *Store) GetFileHash(_ context.Context, path string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.hashes[path], nil
}

// Paths returns all file paths currently tracked by the store.
func (s *Store) Paths() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	paths := make([]string, 0, len(s.hashes))
	for p := range s.hashes {
		paths = append(paths, p)
	}
	return paths
}

func (s *Store) loadHashes() error {
	data, err := os.ReadFile(s.hashFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read hash file: %w", err)
	}
	if err := json.Unmarshal(data, &s.hashes); err != nil {
		return fmt.Errorf("parse hash file: %w", err)
	}
	return nil
}

func (s *Store) saveHashes() error {
	data, err := json.Marshal(s.hashes)
	if err != nil {
		return fmt.Errorf("marshal hashes: %w", err)
	}
	if err := os.WriteFile(s.hashFile, data, 0o600); err != nil {
		return fmt.Errorf("write hash file: %w", err)
	}
	return nil
}

func (s *Store) loadDim() error {
	data, err := os.ReadFile(s.dimFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read dimension file: %w", err)
	}
	var dim int
	if err := json.Unmarshal(data, &dim); err != nil {
		return fmt.Errorf("parse dimension file: %w", err)
	}
	s.dim = dim
	return nil
}

func (s *Store) saveDim() error {
	data, err := json.Marshal(s.dim)
	if err != nil {
		return fmt.Errorf("marshal dimension: %w", err)
	}
	if err := os.WriteFile(s.dimFile, data, 0o600); err != nil {
		return fmt.Errorf("write dimension file: %w", err)
	}
	return nil
}

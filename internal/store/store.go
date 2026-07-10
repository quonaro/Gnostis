package store

import (
	"context"
	"fmt"
	"strconv"

	chromem "github.com/philippgille/chromem-go"

	"github.com/quonaro/gnostis/internal/chunker"
)

const collectionName = "code_chunks"

// Store persists chunks in chromem-go.
type Store struct {
	col *chromem.Collection
}

// New opens or creates a persistent chromem-go database.
func New(ctx context.Context, dataDir string) (*Store, error) {
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

	return &Store{col: col}, nil
}

// AddChunks stores chunks with their precomputed embeddings.
func (s *Store) AddChunks(ctx context.Context, chunks []chunker.Chunk, embeddings [][]float32) error {
	if len(chunks) == 0 {
		return nil
	}
	if len(chunks) != len(embeddings) {
		return fmt.Errorf("chunks (%d) and embeddings (%d) length mismatch", len(chunks), len(embeddings))
	}

	ids := make([]string, len(chunks))
	contents := make([]string, len(chunks))
	metadatas := make([]map[string]string, len(chunks))

	for i, ch := range chunks {
		ids[i] = ch.ID
		contents[i] = ch.Content
		metadatas[i] = chunkMetadata(ch)
	}

	if err := s.col.Add(ctx, ids, embeddings, metadatas, contents); err != nil {
		return fmt.Errorf("add chunks: %w", err)
	}

	return nil
}

// DeleteByPath removes all chunks belonging to a file.
func (s *Store) DeleteByPath(ctx context.Context, path string) error {
	if err := s.col.Delete(ctx, map[string]string{"path": path}, nil); err != nil {
		return fmt.Errorf("delete path %s: %w", path, err)
	}
	return nil
}

// Query searches the vector store with a precomputed embedding.
func (s *Store) Query(ctx context.Context, embedding []float32, n int, filters map[string]string) ([]chromem.Result, error) {
	count := s.col.Count()
	if count == 0 {
		return nil, nil
	}
	if n > count {
		n = count
	}

	results, err := s.col.QueryEmbedding(ctx, embedding, n, filters, nil)
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}

	return results, nil
}

// Count returns the number of stored chunks.
func (s *Store) Count() int {
	return s.col.Count()
}

func chunkMetadata(ch chunker.Chunk) map[string]string {
	return map[string]string{
		"project_id": ch.ProjectID,
		"path":       ch.Path,
		"language":   ch.Language,
		"symbol":     ch.Symbol,
		"signature":  ch.Signature,
		"start_line": strconv.Itoa(ch.StartLine),
		"end_line":   strconv.Itoa(ch.EndLine),
	}
}

package memory

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/quonaro/gnostis/internal/chunker"
	"github.com/quonaro/gnostis/internal/embeddings"
	"github.com/quonaro/gnostis/internal/indexer"
)

const projectIDPrefix = "memory-"

// Indexer indexes exported memory Markdown files into the memory store.
type Indexer struct {
	store   *Store
	chunker *chunker.Chunker
}

// NewIndexer creates a memory indexer.
func NewIndexer(store *Store) *Indexer {
	return &Indexer{
		store:   store,
		chunker: chunker.New(),
	}
}

// IndexFile indexes a single Markdown file under the given memory provider ID.
func (idx *Indexer) IndexFile(ctx context.Context, providerID, path string, provider embeddings.Provider, cache map[string][]float32) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat memory file: %w", err)
	}
	if info.IsDir() {
		return nil
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read memory file: %w", err)
	}

	file := indexer.FileInfo{
		ProjectID: projectIDPrefix + providerID,
		Path:      path,
		RelPath:   filepath.Base(path),
		Content:   string(content),
		ModTime:   info.ModTime(),
		Hash:      hashContent(content),
	}

	storedHash, err := idx.store.GetFileHash(ctx, path)
	if err != nil {
		return fmt.Errorf("lookup stored hash: %w", err)
	}
	if storedHash == file.Hash {
		slog.DebugContext(ctx, "memory file unchanged, skipping", "path", path)
		return nil
	}

	chunks, err := idx.chunker.ChunkFile(ctx, file)
	if err != nil {
		return fmt.Errorf("chunk memory file: %w", err)
	}
	if len(chunks) == 0 {
		return nil
	}

	if err := idx.store.DeleteByPath(ctx, path); err != nil {
		return fmt.Errorf("delete stale memory chunks: %w", err)
	}

	vectors, err := embedChunks(ctx, provider, chunks, cache, nil)
	if err != nil {
		return fmt.Errorf("embed memory chunks: %w", err)
	}

	if err := idx.store.AddChunks(ctx, chunks, vectors); err != nil {
		return fmt.Errorf("store memory chunks: %w", err)
	}

	slog.DebugContext(ctx, "indexed memory file", "path", path, "provider", providerID, "chunks", len(chunks))
	return nil
}

// RemoveFile removes all chunks for a deleted memory file.
func (idx *Indexer) RemoveFile(ctx context.Context, path string) error {
	return idx.store.DeleteByPath(ctx, path)
}

func embedChunks(ctx context.Context, provider embeddings.Provider, chunks []chunker.Chunk, cache map[string][]float32, onEmbedded func(int)) ([][]float32, error) {
	results := make([][]float32, len(chunks))
	var missingIndices []int
	var missingTexts []string

	for i, c := range chunks {
		if cache == nil {
			missingIndices = append(missingIndices, i)
			missingTexts = append(missingTexts, c.Content)
			continue
		}
		if v, ok := cache[c.ID]; ok {
			results[i] = v
			continue
		}
		missingIndices = append(missingIndices, i)
		missingTexts = append(missingTexts, c.Content)
	}

	if len(missingTexts) == 0 {
		return results, nil
	}

	batchSize := provider.BatchSize()
	if batchSize <= 0 {
		batchSize = 32
	}

	for i := 0; i < len(missingTexts); i += batchSize {
		end := i + batchSize
		if end > len(missingTexts) {
			end = len(missingTexts)
		}

		vectors, err := provider.Embed(ctx, missingTexts[i:end])
		if err != nil {
			return nil, fmt.Errorf("embed batch %d-%d: %w", i, end, err)
		}
		if len(vectors) != end-i {
			return nil, fmt.Errorf("expected %d embeddings, got %d", end-i, len(vectors))
		}

		for j, idx := range missingIndices[i:end] {
			results[idx] = vectors[j]
			if cache != nil {
				cache[chunks[idx].ID] = vectors[j]
			}
		}

		if onEmbedded != nil {
			onEmbedded(len(vectors))
		}
	}

	return results, nil
}

func hashContent(data []byte) string {
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum)
}

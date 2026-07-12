package app

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/quonaro/gnostis/internal/chunker"
	"github.com/quonaro/gnostis/internal/embeddings"
	"github.com/quonaro/gnostis/internal/symbol"
)

func chunksToSymbolChunks(chunks []chunker.Chunk) []symbol.Chunk {
	out := make([]symbol.Chunk, 0, len(chunks))
	for _, c := range chunks {
		out = append(out, symbol.Chunk{
			ProjectID: c.ProjectID,
			Path:      c.Path,
			Language:  c.Language,
			Symbol:    c.Symbol,
			Signature: c.Signature,
			StartLine: c.StartLine,
			EndLine:   c.EndLine,
		})
	}
	return out
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

	if len(missingTexts) > 0 {
		batchSize := provider.BatchSize()
		if batchSize <= 0 {
			batchSize = 32
		}

		slog.DebugContext(ctx, "embedding chunks", "count", len(missingTexts), "cached", len(chunks)-len(missingTexts), "batch_size", batchSize, "model", provider.ModelName())

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
	}

	return results, nil
}

package search

import (
	"context"
	"fmt"
	"sort"

	"github.com/quonaro/gnostis/internal/embeddings"
	"github.com/quonaro/gnostis/internal/store"
)

// Result is a ranked search result.
type Result struct {
	ID        string
	ProjectID string
	Path      string
	Language  string
	Symbol    string
	Signature string
	Content   string
	StartLine int
	EndLine   int
	Score     float32
}

// Engine runs semantic searches against the vector store.
type Engine struct {
	store    *store.Store
	provider embeddings.Provider
}

// New creates a search engine.
func New(s *store.Store, p embeddings.Provider) *Engine {
	return &Engine{store: s, provider: p}
}

// Search performs a vector search with optional metadata filters.
func (e *Engine) Search(ctx context.Context, query string, filters map[string]string, topK int) ([]Result, error) {
	if topK <= 0 {
		topK = 10
	}

	vectors, err := e.provider.Embed(ctx, []string{query})
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}
	if len(vectors) == 0 {
		return nil, fmt.Errorf("empty query embedding")
	}

	raw, err := e.store.Query(ctx, vectors[0], topK*2, filters)
	if err != nil {
		return nil, fmt.Errorf("query store: %w", err)
	}

	results := make([]Result, 0, len(raw))
	for _, r := range raw {
		res := resultFromChromem(r)
		res.Score = boostScore(res, query, r.Similarity)
		results = append(results, res)
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if len(results) > topK {
		results = results[:topK]
	}

	return results, nil
}

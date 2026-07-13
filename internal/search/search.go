package search

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"sort"
	"strings"

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
	store    store.VectorStore
	provider embeddings.Provider
}

// New creates a search engine.
func New(s store.VectorStore, p embeddings.Provider) *Engine {
	return &Engine{store: s, provider: p}
}

// Search performs a vector search with optional metadata filters.
// A "path" filter is treated as an absolute path prefix and is applied after
// the vector query.
func (e *Engine) Search(ctx context.Context, query string, filters map[string]string, topK int) ([]Result, error) {
	if topK <= 0 {
		topK = 10
	}

	pathPrefix, queryFilters := pathPrefixFromFilters(filters)

	slog.DebugContext(ctx, "search", "query", query, "filters", queryFilters, "path_prefix", pathPrefix, "top_k", topK)

	vectors, err := e.provider.Embed(ctx, []string{query})
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}
	if len(vectors) == 0 {
		return nil, fmt.Errorf("empty query embedding")
	}

	limit := topK * 2
	if pathPrefix != "" {
		limit = topK * 4
	}

	raw, err := e.store.Query(ctx, vectors[0], limit, queryFilters)
	if err != nil {
		return nil, fmt.Errorf("query store: %w", err)
	}

	results := make([]Result, 0, len(raw))
	for _, r := range raw {
		res := resultFromChromem(r)
		if pathPrefix != "" && !isUnderPath(res.Path, pathPrefix) {
			continue
		}
		res.Score = boostScore(res, query, r.Similarity)
		results = append(results, res)
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if len(results) > topK {
		results = results[:topK]
	}

	slog.DebugContext(ctx, "search results", "count", len(results))
	return results, nil
}

func pathPrefixFromFilters(filters map[string]string) (string, map[string]string) {
	if filters == nil {
		return "", nil
	}
	pathPrefix := filters["path"]
	if pathPrefix == "" {
		return "", filters
	}

	queryFilters := make(map[string]string, len(filters)-1)
	for k, v := range filters {
		if k == "path" {
			continue
		}
		queryFilters[k] = v
	}
	return pathPrefix, queryFilters
}

func isUnderPath(path, root string) bool {
	if root == string(filepath.Separator) {
		return true
	}
	if !strings.HasPrefix(path, root) {
		return false
	}
	if len(path) == len(root) {
		return true
	}
	return path[len(root)] == filepath.Separator
}

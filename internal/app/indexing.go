package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/quonaro/gnostis/internal/chunker"
	"github.com/quonaro/gnostis/internal/directory"
	"github.com/quonaro/gnostis/internal/embeddings"
	"github.com/quonaro/gnostis/internal/indexer"
	"github.com/quonaro/gnostis/internal/project"
	"github.com/quonaro/gnostis/internal/store"
	"github.com/quonaro/gnostis/internal/symbol"
)

func indexDirectory(ctx context.Context, dir directory.Directory, proj project.Project, idx *indexer.Indexer, ch *chunker.Chunker, provider embeddings.Provider, st store.VectorStore, sym *symbol.Index, cache map[string][]float32) error {
	files, err := idx.Index(ctx, dir, proj)
	if err != nil {
		return fmt.Errorf("walk directory: %w", err)
	}
	slog.InfoContext(ctx, "indexed files", "project", proj.Name, "count", len(files))

	changed, err := chunkFilesParallel(ctx, files, ch, st, sym)
	if err != nil {
		return fmt.Errorf("chunk files: %w", err)
	}

	allChunks := make([]chunker.Chunk, 0)
	for _, fc := range changed {
		allChunks = append(allChunks, fc.chunks...)
	}
	if len(allChunks) == 0 {
		slog.InfoContext(ctx, "no chunks to embed", "project", proj.Name)
		return nil
	}

	vectors, err := embedChunks(ctx, provider, allChunks, cache)
	if err != nil {
		return fmt.Errorf("embed chunks: %w", err)
	}

	if err := st.AddChunks(ctx, allChunks, vectors); err != nil {
		return fmt.Errorf("store chunks: %w", err)
	}
	slog.InfoContext(ctx, "stored chunks", "project", proj.Name, "count", len(allChunks))
	return nil
}

type fileChunks struct {
	file   indexer.FileInfo
	chunks []chunker.Chunk
}

func chunkFilesParallel(ctx context.Context, files []indexer.FileInfo, ch *chunker.Chunker, st store.VectorStore, sym *symbol.Index) ([]fileChunks, error) {
	workers := runtime.NumCPU()
	if workers < 2 {
		workers = 2
	}
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var changed []fileChunks

	for _, f := range files {
		storedHash, err := st.GetFileHash(ctx, f.Path)
		if err != nil {
			slog.WarnContext(ctx, "lookup stored hash", "path", f.Path, "error", err)
			continue
		}
		if storedHash == f.Hash {
			slog.DebugContext(ctx, "skipping unchanged file", "path", f.Path)
			continue
		}

		wg.Add(1)
		sem <- struct{}{}
		go func(file indexer.FileInfo) {
			defer wg.Done()
			defer func() { <-sem }()

			chunks, err := ch.ChunkFile(ctx, file)
			if err != nil {
				slog.WarnContext(ctx, "chunk file", "path", file.Path, "error", err)
				return
			}
			if len(chunks) == 0 {
				return
			}
			mu.Lock()
			changed = append(changed, fileChunks{file: file, chunks: chunks})
			mu.Unlock()
		}(f)
	}
	wg.Wait()

	for _, fc := range changed {
		if err := st.DeleteByPath(ctx, fc.file.Path); err != nil {
			slog.WarnContext(ctx, "delete stale chunks", "path", fc.file.Path, "error", err)
		}
		sym.RemoveByPath(fc.file.Path)
		sym.AddChunks(chunksToSymbolChunks(fc.chunks))
	}
	return changed, nil
}

func reindexFile(ctx context.Context, absPath string, dirs []directory.Directory, projects []project.Project, st store.VectorStore, sym *symbol.Index, provider embeddings.Provider, cache map[string][]float32) error {
	if len(dirs) != len(projects) {
		return fmt.Errorf("directory and project count mismatch")
	}

	for i, dir := range dirs {
		if !strings.HasPrefix(absPath, dir.Path) {
			continue
		}

		rel, err := filepath.Rel(dir.Path, absPath)
		if err != nil {
			return fmt.Errorf("relative path: %w", err)
		}

		info, err := os.Stat(absPath)
		if err != nil {
			_ = st.DeleteByPath(ctx, absPath)
			sym.RemoveByPath(absPath)
			return nil
		}

		if info.IsDir() || !dir.ShouldIndex(rel, info.Size()) {
			_ = st.DeleteByPath(ctx, absPath)
			sym.RemoveByPath(absPath)
			return nil
		}

		slog.InfoContext(ctx, "reindexing file", "path", absPath, "project", projects[i].Name)

		_ = st.DeleteByPath(ctx, absPath)
		sym.RemoveByPath(absPath)

		content, err := os.ReadFile(absPath)
		if err != nil {
			return fmt.Errorf("read file: %w", err)
		}

		f := indexer.FileInfo{
			ProjectID: projects[i].ID,
			Path:      absPath,
			RelPath:   rel,
			Content:   string(content),
			ModTime:   info.ModTime(),
		}

		ch := chunker.New()
		chunks, err := ch.ChunkFile(ctx, f)
		if err != nil {
			return fmt.Errorf("chunk file: %w", err)
		}
		if len(chunks) == 0 {
			return nil
		}

		sym.AddChunks(chunksToSymbolChunks(chunks))

		vectors, err := embedChunks(ctx, provider, chunks, cache)
		if err != nil {
			return fmt.Errorf("embed chunks: %w", err)
		}

		return st.AddChunks(ctx, chunks, vectors)
	}

	return nil
}

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

func embedChunks(ctx context.Context, provider embeddings.Provider, chunks []chunker.Chunk, cache map[string][]float32) ([][]float32, error) {
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
		slog.DebugContext(ctx, "embedding chunks", "count", len(missingTexts), "cached", len(chunks)-len(missingTexts), "model", provider.ModelName())
		vectors, err := provider.Embed(ctx, missingTexts)
		if err != nil {
			return nil, fmt.Errorf("embed: %w", err)
		}
		if len(vectors) != len(missingTexts) {
			return nil, fmt.Errorf("expected %d embeddings, got %d", len(missingTexts), len(vectors))
		}
		for j, idx := range missingIndices {
			results[idx] = vectors[j]
			if cache != nil {
				cache[chunks[idx].ID] = vectors[j]
			}
		}
	}

	return results, nil
}

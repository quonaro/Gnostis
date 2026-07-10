package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/quonaro/gnostis/internal/chunker"
	"github.com/quonaro/gnostis/internal/directory"
	"github.com/quonaro/gnostis/internal/embeddings"
	"github.com/quonaro/gnostis/internal/indexer"
	"github.com/quonaro/gnostis/internal/project"
	"github.com/quonaro/gnostis/internal/store"
)

func indexDirectory(ctx context.Context, dir directory.Directory, proj project.Project, idx *indexer.Indexer, ch *chunker.Chunker, provider embeddings.Provider, st *store.Store) error {
	files, err := idx.Index(ctx, dir, proj)
	if err != nil {
		return fmt.Errorf("walk directory: %w", err)
	}
	slog.InfoContext(ctx, "indexed files", "project", proj.Name, "count", len(files))

	var allChunks []chunker.Chunk
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

		chunks, err := ch.ChunkFile(ctx, f)
		if err != nil {
			slog.WarnContext(ctx, "chunk file", "path", f.Path, "error", err)
			continue
		}
		if len(chunks) > 0 {
			if err := st.DeleteByPath(ctx, f.Path); err != nil {
				slog.WarnContext(ctx, "delete stale chunks", "path", f.Path, "error", err)
			}
		}
		allChunks = append(allChunks, chunks...)
	}

	if len(allChunks) == 0 {
		slog.InfoContext(ctx, "no chunks to embed", "project", proj.Name)
		return nil
	}
	slog.InfoContext(ctx, "embedding chunks", "project", proj.Name, "count", len(allChunks))

	vectors, err := embedChunks(ctx, provider, allChunks)
	if err != nil {
		return fmt.Errorf("embed chunks: %w", err)
	}

	if err := st.AddChunks(ctx, allChunks, vectors); err != nil {
		return fmt.Errorf("store chunks: %w", err)
	}
	slog.InfoContext(ctx, "stored chunks", "project", proj.Name, "count", len(allChunks))

	return nil
}

func reindexFile(ctx context.Context, absPath string, dirs []directory.Directory, projects []project.Project, st *store.Store, provider embeddings.Provider) error {
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
			return nil
		}

		if info.IsDir() || !dir.ShouldIndex(rel, info.Size()) {
			_ = st.DeleteByPath(ctx, absPath)
			return nil
		}

		slog.InfoContext(ctx, "reindexing file", "path", absPath, "project", projects[i].Name)

		_ = st.DeleteByPath(ctx, absPath)

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

		vectors, err := embedChunks(ctx, provider, chunks)
		if err != nil {
			return fmt.Errorf("embed chunks: %w", err)
		}

		return st.AddChunks(ctx, chunks, vectors)
	}

	return nil
}

func embedChunks(ctx context.Context, provider embeddings.Provider, chunks []chunker.Chunk) ([][]float32, error) {
	texts := make([]string, len(chunks))
	for i, c := range chunks {
		texts[i] = c.Content
	}
	slog.DebugContext(ctx, "embedding chunks", "count", len(chunks), "model", provider.ModelName())

	vectors, err := provider.Embed(ctx, texts)
	if err != nil {
		return nil, fmt.Errorf("embed: %w", err)
	}
	if len(vectors) != len(chunks) {
		return nil, fmt.Errorf("expected %d embeddings, got %d", len(chunks), len(vectors))
	}

	return vectors, nil
}

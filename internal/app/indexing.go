package app

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/quonaro/gnostis/internal/chunker"
	"github.com/quonaro/gnostis/internal/config"
	"github.com/quonaro/gnostis/internal/directory"
	"github.com/quonaro/gnostis/internal/embeddings"
	"github.com/quonaro/gnostis/internal/indexer"
	"github.com/quonaro/gnostis/internal/progress"
	"github.com/quonaro/gnostis/internal/project"
	"github.com/quonaro/gnostis/internal/stats"
	"github.com/quonaro/gnostis/internal/store"
	"github.com/quonaro/gnostis/internal/symbol"
	"github.com/schollz/progressbar/v2"
)

func indexDirectory(ctx context.Context, out io.Writer, dir directory.Directory, proj project.Project, idx *indexer.Indexer, ch *chunker.Chunker, provider embeddings.Provider, st store.VectorStore, sym *symbol.Index, cache map[string][]float32, prog *progress.Progress, indexingStats *stats.Stats) error {
	if prog != nil {
		_ = prog.Start(proj.Name, 0)
	}

	files, err := idx.Index(ctx, dir, proj)
	if err != nil {
		return fmt.Errorf("walk directory: %w", err)
	}
	slog.InfoContext(ctx, "indexed files", "project", proj.Name, "count", len(files))

	if prog != nil {
		_ = prog.Start(proj.Name, len(files))
		_ = prog.SetPhase(progress.PhaseChunking)
	}

	var bar *progressbar.ProgressBar
	if out != nil {
		bar = progressbar.NewOptions(len(files),
			progressbar.OptionSetWriter(out),
			progressbar.OptionShowCount(),
			progressbar.OptionSetDescription(fmt.Sprintf("chunking %s", proj.Name)),
			progressbar.OptionSetPredictTime(true),
		)
	}

	changed, err := chunkFilesParallel(ctx, files, ch, st, sym, bar, prog)
	if err != nil {
		return fmt.Errorf("chunk files: %w", err)
	}

	allChunks := make([]chunker.Chunk, 0)
	for _, fc := range changed {
		allChunks = append(allChunks, fc.chunks...)
	}
	if len(allChunks) == 0 {
		if bar != nil {
			_ = bar.Finish()
		}
		if prog != nil {
			_ = prog.Done()
		}
		slog.InfoContext(ctx, "no chunks to embed", "project", proj.Name)
		updateStats(ctx, indexingStats, st, proj.Name)
		return nil
	}

	if prog != nil {
		_ = prog.SetPhase(progress.PhaseEmbedding)
		_ = prog.SetTotalChunks(len(allChunks))
	}

	if bar != nil {
		_ = bar.Finish()
		bar = progressbar.NewOptions(len(allChunks),
			progressbar.OptionSetWriter(out),
			progressbar.OptionShowCount(),
			progressbar.OptionSetDescription(fmt.Sprintf("embedding %s", proj.Name)),
			progressbar.OptionShowIts(),
			progressbar.OptionSetPredictTime(true),
		)
	}

	vectors, err := embedChunks(ctx, provider, allChunks, cache, func(done int) {
		if bar != nil {
			_ = bar.Add(done)
		}
		if prog != nil {
			_ = prog.AddChunks(done)
		}
	})
	if err != nil {
		return fmt.Errorf("embed chunks: %w", err)
	}

	if bar != nil {
		_ = bar.Finish()
	}

	if err := st.AddChunks(ctx, allChunks, vectors); err != nil {
		return fmt.Errorf("store chunks: %w", err)
	}

	updateStats(ctx, indexingStats, st, proj.Name)

	if prog != nil {
		_ = prog.Done()
	}
	slog.InfoContext(ctx, "stored chunks", "project", proj.Name, "count", len(allChunks))
	return nil
}

func updateStats(ctx context.Context, indexingStats *stats.Stats, st store.VectorStore, projectID string) {
	if indexingStats == nil {
		return
	}
	count, _ := st.CountByProject(ctx, projectID)
	_ = indexingStats.Update(projectID, count)
}

type fileChunks struct {
	file   indexer.FileInfo
	chunks []chunker.Chunk
}

func progressAdd(bar *progressbar.ProgressBar, prog *progress.Progress, n int) {
	if bar != nil {
		_ = bar.Add(n)
	}
	if prog != nil {
		_ = prog.AddFiles(n)
	}
}

func chunkFilesParallel(ctx context.Context, files []indexer.FileInfo, ch *chunker.Chunker, st store.VectorStore, sym *symbol.Index, bar *progressbar.ProgressBar, prog *progress.Progress) ([]fileChunks, error) {
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
			progressAdd(bar, prog, 1)
			continue
		}
		if storedHash == f.Hash {
			slog.DebugContext(ctx, "skipping unchanged file", "path", f.Path)
			progressAdd(bar, prog, 1)
			continue
		}

		wg.Add(1)
		sem <- struct{}{}
		go func(file indexer.FileInfo) {
			defer wg.Done()
			defer func() { <-sem }()
			defer progressAdd(bar, prog, 1)

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

	paths := make([]string, len(changed))
	for i, fc := range changed {
		paths[i] = fc.file.Path
	}
	if err := st.DeleteByPaths(ctx, paths); err != nil {
		slog.WarnContext(ctx, "delete stale chunks", "error", err)
	}
	for _, fc := range changed {
		sym.RemoveByPath(fc.file.Path)
		sym.AddChunks(chunksToSymbolChunks(fc.chunks))
	}
	return changed, nil
}

func reindexFile(ctx context.Context, absPath string, dirs []directory.Directory, projects []project.Project, cfg config.Config, st store.VectorStore, sym *symbol.Index, provider embeddings.Provider, cache map[string][]float32, indexingStats *stats.Stats) error {
	if len(dirs) != len(projects) {
		return fmt.Errorf("directory and project count mismatch")
	}

	for i, dir := range dirs {
		if !strings.HasPrefix(absPath, dir.Path) {
			continue
		}
		return reindexFileUnder(ctx, absPath, dir, projects[i], st, sym, provider, cache, indexingStats)
	}

	// Path is not under any configured directory. Index it under a synthetic
	// directory rooted at the file's parent so global rules still apply.
	parent := filepath.Dir(absPath)
	dir := directory.FromConfig(cfg.Index, config.Directory{Path: parent, Name: filepath.Base(parent)})
	proj := project.New(filepath.Base(parent), parent)
	return reindexFileUnder(ctx, absPath, dir, proj, st, sym, provider, cache, indexingStats)
}

func reindexFileUnder(ctx context.Context, absPath string, dir directory.Directory, proj project.Project, st store.VectorStore, sym *symbol.Index, provider embeddings.Provider, cache map[string][]float32, indexingStats *stats.Stats) (err error) {
	defer func() {
		if err == nil {
			updateStats(ctx, indexingStats, st, proj.ID)
		}
	}()

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

	slog.InfoContext(ctx, "reindexing file", "path", absPath, "project", proj.Name)

	_ = st.DeleteByPath(ctx, absPath)
	sym.RemoveByPath(absPath)

	content, err := os.ReadFile(absPath)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	f := indexer.FileInfo{
		ProjectID: proj.ID,
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

	vectors, err := embedChunks(ctx, provider, chunks, cache, nil)
	if err != nil {
		return fmt.Errorf("embed chunks: %w", err)
	}

	return st.AddChunks(ctx, chunks, vectors)
}

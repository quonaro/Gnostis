package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/quonaro/gnostis/internal/config"
	"github.com/quonaro/gnostis/internal/directory"
	"github.com/quonaro/gnostis/internal/project"
)

// RebuildProject removes the existing index for a single project and reindexes it.
func (a *App) RebuildProject(ctx context.Context, name string) error {
	for i, p := range a.projects {
		if p.Name != name {
			continue
		}

		slog.InfoContext(ctx, "rebuilding project", "project", p.Name, "path", a.dirs[i].Path)

		if err := a.deleteChunksByPrefix(ctx, a.dirs[i].Path); err != nil {
			_ = a.progress.Fail(err)
			return fmt.Errorf("delete project chunks: %w", err)
		}

		if err := indexDirectory(ctx, a.ProgressWriter, a.dirs[i], p, a.indexer, a.chunker, a.provider, a.store, a.symbolIndex, a.embeddingCache, a.progress, a.indexingStats); err != nil {
			return fmt.Errorf("index %s: %w", a.dirs[i].Path, err)
		}

		if err := a.symbolIndex.Save(); err != nil {
			_ = a.progress.Fail(err)
			return fmt.Errorf("save symbol index: %w", err)
		}

		slog.InfoContext(ctx, "project rebuild complete", "project", p.Name, "chunks", a.store.Count())
		return nil
	}

	return fmt.Errorf("project %q not found", name)
}

// StartRebuildProject starts a rebuild job for a single project and returns a job ID.
func (a *App) StartRebuildProject(ctx context.Context, name string) (string, error) {
	return a.startJob(ctx, "project:"+name, func(ctx context.Context) error {
		return a.RebuildProject(ctx, name)
	})
}

// RebuildIndex removes the existing index and rebuilds everything.
func (a *App) RebuildIndex(ctx context.Context) error {
	if a.watcher != nil {
		if err := a.watcher.Stop(); err != nil {
			return fmt.Errorf("stop watcher: %w", err)
		}
	}

	if err := a.deleteChunksByPrefix(ctx, ""); err != nil {
		_ = a.progress.Fail(err)
		return fmt.Errorf("delete all chunks: %w", err)
	}

	if err := a.initialIndex(ctx); err != nil {
		_ = a.progress.Fail(err)
		return fmt.Errorf("initial index: %w", err)
	}

	if a.watcher != nil {
		if err := a.watcher.Start(); err != nil {
			return fmt.Errorf("restart watcher: %w", err)
		}
	}

	slog.InfoContext(ctx, "full rebuild complete", "chunks", a.store.Count())
	return nil
}

// StartRebuildIndex starts a full rebuild job and returns a job ID.
func (a *App) StartRebuildIndex(ctx context.Context) (string, error) {
	return a.startJob(ctx, "index", func(ctx context.Context) error {
		return a.RebuildIndex(ctx)
	})
}

func (a *App) startJob(ctx context.Context, prefix string, fn func(context.Context) error) (string, error) {
	a.jobMu.Lock()
	if a.jobRunning {
		id := a.currentJobID
		a.jobMu.Unlock()
		return id, nil
	}

	id := fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	a.jobRunning = true
	a.currentJobID = id
	a.progress.SetJobID(id)
	a.jobMu.Unlock()

	go func() {
		a.rebuildMu.Lock()
		defer a.rebuildMu.Unlock()
		defer func() {
			a.jobMu.Lock()
			a.jobRunning = false
			a.jobMu.Unlock()
		}()

		if err := fn(ctx); err != nil {
			_ = a.progress.Fail(err)
			slog.ErrorContext(ctx, "job failed", "job_id", id, "error", err)
			return
		}
		slog.InfoContext(ctx, "job completed", "job_id", id)
	}()

	return id, nil
}

// deleteChunksByPrefix removes all indexed chunks whose path is under prefix.
// An empty prefix matches every path.
func (a *App) deleteChunksByPrefix(ctx context.Context, prefix string) error {
	var toDelete []string
	for _, path := range a.store.Paths() {
		if !isUnderPath(path, prefix) {
			continue
		}
		toDelete = append(toDelete, path)
		a.symbolIndex.RemoveByPath(path)
	}
	if err := a.store.DeleteByPaths(ctx, toDelete); err != nil {
		return fmt.Errorf("delete chunks: %w", err)
	}
	return nil
}

func isUnderPath(path, root string) bool {
	if root == string(filepath.Separator) {
		return true
	}
	if root == "" {
		if path == "" {
			return false
		}
		return path[0] == filepath.Separator
	}
	if !strings.HasPrefix(path, root) {
		return false
	}
	if len(path) == len(root) {
		return true
	}
	return path[len(root)] == filepath.Separator
}

// rebuildDirectory removes existing chunks under dirPath and reindexes the directory.
func (a *App) rebuildDirectory(ctx context.Context, dirPath string) error {
	slog.InfoContext(ctx, "rebuilding directory", "path", dirPath)

	if err := a.deleteChunksByPrefix(ctx, dirPath); err != nil {
		return fmt.Errorf("delete directory chunks: %w", err)
	}

	dir := directory.FromConfig(a.cfg.Index, config.Directory{Path: dirPath, Name: filepath.Base(dirPath)})
	proj := project.New(filepath.Base(dirPath), dirPath)

	if err := indexDirectory(ctx, a.ProgressWriter, dir, proj, a.indexer, a.chunker, a.provider, a.store, a.symbolIndex, a.embeddingCache, nil, a.indexingStats); err != nil {
		return fmt.Errorf("index directory: %w", err)
	}

	slog.InfoContext(ctx, "directory rebuild complete", "path", dirPath, "chunks", a.store.Count())
	return nil
}

// rebuildFile removes existing chunks for a single file and reindexes it.
func (a *App) rebuildFile(ctx context.Context, filePath string) error {
	_ = a.store.DeleteByPath(ctx, filePath)
	a.symbolIndex.RemoveByPath(filePath)

	if err := reindexFile(ctx, filePath, a.dirs, a.projects, a.cfg, a.store, a.symbolIndex, a.provider, a.embeddingCache, a.indexingStats); err != nil {
		return fmt.Errorf("reindex file: %w", err)
	}
	return nil
}

// ReindexFiles reindexes the given file or directory paths and persists the symbol index.
// Paths outside configured directories are indexed with global defaults.
func (a *App) ReindexFiles(ctx context.Context, paths []string) error {
	for _, raw := range paths {
		path, err := filepath.Abs(raw)
		if err != nil {
			return fmt.Errorf("resolve path %q: %w", raw, err)
		}

		info, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("stat %s: %w", path, err)
		}

		if info.IsDir() {
			if err := a.rebuildDirectory(ctx, path); err != nil {
				return fmt.Errorf("reindex directory %s: %w", path, err)
			}
			continue
		}

		if err := a.rebuildFile(ctx, path); err != nil {
			return fmt.Errorf("reindex file %s: %w", path, err)
		}
	}
	if err := a.symbolIndex.Save(); err != nil {
		return fmt.Errorf("save symbol index: %w", err)
	}
	return nil
}

// RebuildPaths rebuilds the index for the given paths. Configured project names are
// rebuilt as projects; files and directories are reindexed directly, even when they
// are not part of the configuration.
func (a *App) RebuildPaths(ctx context.Context, paths []string) error {
	for _, raw := range paths {
		matched := false
		for _, p := range a.projects {
			if p.Name != raw {
				continue
			}
			matched = true
			if err := a.RebuildProject(ctx, p.Name); err != nil {
				return fmt.Errorf("rebuild project %s: %w", p.Name, err)
			}
			break
		}
		if matched {
			continue
		}

		if err := a.ReindexFiles(ctx, []string{raw}); err != nil {
			return fmt.Errorf("rebuild path %s: %w", raw, err)
		}
	}
	return nil
}

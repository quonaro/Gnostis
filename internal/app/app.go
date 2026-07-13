package app

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/quonaro/gnostis/internal/chunker"
	"github.com/quonaro/gnostis/internal/config"
	"github.com/quonaro/gnostis/internal/directory"
	"github.com/quonaro/gnostis/internal/embeddings"
	"github.com/quonaro/gnostis/internal/indexer"
	"github.com/quonaro/gnostis/internal/lock"
	mcpServer "github.com/quonaro/gnostis/internal/mcp"
	"github.com/quonaro/gnostis/internal/progress"
	"github.com/quonaro/gnostis/internal/project"
	"github.com/quonaro/gnostis/internal/search"
	"github.com/quonaro/gnostis/internal/stats"
	"github.com/quonaro/gnostis/internal/store"
	"github.com/quonaro/gnostis/internal/symbol"
	"github.com/quonaro/gnostis/internal/watcher"
)

// App orchestrates configuration, indexing, search, and the MCP server.
type App struct {
	cfg            config.Config
	dirs           []directory.Directory
	projects       []project.Project
	store          store.VectorStore
	provider       embeddings.Provider
	engine         *search.Engine
	indexer        *indexer.Indexer
	chunker        *chunker.Chunker
	symbolIndex    *symbol.Index
	watcher        *watcher.Watcher
	mcp            *mcpServer.Server
	embeddingCache map[string][]float32
	progress       *progress.Progress
	indexingStats  *stats.Stats
	lock           *lock.Lock
	ProgressWriter io.Writer
}

// New builds the application from configuration.
func New(cfg config.Config) (*App, error) {
	slog.Info("initializing app", "data_dir", cfg.DataDir, "provider", cfg.Embeddings.Provider, "model", cfg.Embeddings.Model)

	dirs := make([]directory.Directory, len(cfg.Directories))
	projects := make([]project.Project, len(cfg.Directories))

	for i, d := range cfg.Directories {
		dirs[i] = directory.FromConfig(cfg.Index, d)
		projects[i] = project.New(d.Name, d.Path)
	}

	ctx := context.Background()

	st, err := store.New(ctx, cfg.DataDir)
	if err != nil {
		return nil, fmt.Errorf("create store: %w", err)
	}

	provider, err := embeddings.New(cfg.Embeddings)
	if err != nil {
		return nil, fmt.Errorf("create embeddings provider: %w", err)
	}

	engine := search.New(st, provider)

	symbolIndex, err := symbol.New(filepath.Join(cfg.DataDir, "symbols.json"))
	if err != nil {
		return nil, fmt.Errorf("create symbol index: %w", err)
	}

	embeddingCache := make(map[string][]float32)

	a := &App{
		cfg:            cfg,
		dirs:           dirs,
		projects:       projects,
		store:          st,
		provider:       provider,
		engine:         engine,
		indexer:        indexer.New(),
		chunker:        chunker.New(),
		symbolIndex:    symbolIndex,
		embeddingCache: embeddingCache,
		progress:       progress.New(filepath.Join(cfg.DataDir, "indexing-progress.json")),
		indexingStats:  stats.New(filepath.Join(cfg.DataDir, "project-stats.json")),
		lock:           lock.New(filepath.Dir(cfg.DataDir)),
	}

	mcpSrv := mcpServer.New(cfg.MCP.Name, cfg.MCP.Version, engine, symbolIndex, a, projects)
	a.mcp = mcpSrv

	w := watcher.New(dirs, func(path string) {
		if err := reindexFile(context.Background(), path, dirs, projects, a.cfg, a.store, a.symbolIndex, a.provider, a.embeddingCache, a.indexingStats); err != nil {
			slog.Error("reindex file", "path", path, "error", err)
			return
		}
		if err := a.symbolIndex.Save(); err != nil {
			slog.Error("save symbol index", "error", err)
		}
	})
	a.watcher = w

	return a, nil
}

// Run serves the MCP HTTP server while performing initial indexing and starting
// the watcher in the background. The first component error stops the app.
func (a *App) Run(ctx context.Context) error {
	slog.InfoContext(ctx, "starting app")
	if err := a.lock.TryLock(); err != nil {
		return fmt.Errorf("acquire lock: %w", err)
	}
	defer func() { _ = a.lock.Unlock() }()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	errCh := make(chan error, 2)
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		slog.InfoContext(ctx, "serving mcp http", "name", a.cfg.MCP.Name, "version", a.cfg.MCP.Version, "address", a.cfg.MCP.Address)
		if err := a.runHTTP(ctx); err != nil {
			errCh <- err
		}
		cancel()
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := a.initialIndex(ctx); err != nil {
			errCh <- fmt.Errorf("initial index: %w", err)
			cancel()
			return
		}
		if err := a.watcher.Start(); err != nil {
			errCh <- fmt.Errorf("start watcher: %w", err)
			cancel()
			return
		}
		<-ctx.Done()
		_ = a.watcher.Stop()
	}()

	wg.Wait()
	close(errCh)
	for err := range errCh {
		return err
	}
	return nil
}

func (a *App) runHTTP(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigChan)

	go func() {
		if err := a.mcp.StartHTTP(ctx, a.cfg.MCP.Address, a.cfg.MCP.Token); err != nil {
			slog.ErrorContext(ctx, "mcp http server stopped", "error", err)
			cancel()
		}
	}()

	select {
	case <-ctx.Done():
	case <-sigChan:
		slog.InfoContext(ctx, "shutting down")
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := a.mcp.StopHTTP(shutdownCtx); err != nil {
		slog.ErrorContext(ctx, "stop mcp http server", "error", err)
	}
	return nil
}

func (a *App) initialIndex(ctx context.Context) error {
	a.cleanupDeletedFiles(ctx)
	for i, dir := range a.dirs {
		slog.InfoContext(ctx, "indexing directory", "path", dir.Path, "project", a.projects[i].Name)
		if err := indexDirectory(ctx, a.ProgressWriter, dir, a.projects[i], a.indexer, a.chunker, a.provider, a.store, a.symbolIndex, a.embeddingCache, a.progress, a.indexingStats); err != nil {
			if a.progress != nil {
				_ = a.progress.Fail(err)
			}
			return fmt.Errorf("index %s: %w", dir.Path, err)
		}
	}
	if err := a.symbolIndex.Save(); err != nil {
		slog.ErrorContext(ctx, "save symbol index", "error", err)
	}
	slog.InfoContext(ctx, "initial index complete", "chunks", a.store.Count())
	return nil
}

// InitialIndex performs the first-time indexing of all configured directories.
func (a *App) InitialIndex(ctx context.Context) error {
	return a.initialIndex(ctx)
}

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

// deleteChunksByPrefix removes all indexed chunks whose path is under prefix.
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

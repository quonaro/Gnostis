package app

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
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
	ConfigPath     string

	jobMu          sync.Mutex
	jobRunning     bool
	currentJobID   string
	rebuildMu      sync.RWMutex
	watcherStarted bool
}

// New builds the application from configuration.
func New(cfg config.Config) (*App, error) {
	slog.Info("initializing app", "data_dir", cfg.DataDir, "provider", cfg.Embeddings.Provider, "model", cfg.Embeddings.Model)

	dirs, projects, err := resolveProjects(cfg)
	if err != nil {
		return nil, fmt.Errorf("resolve projects: %w", err)
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

	a.watcher = a.newWatcher()

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

	errCh := make(chan error, 3)
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
		a.rebuildMu.Lock()
		if err := a.initialIndex(ctx); err != nil {
			a.rebuildMu.Unlock()
			errCh <- fmt.Errorf("initial index: %w", err)
			cancel()
			return
		}
		if err := a.watcher.Start(); err != nil {
			a.rebuildMu.Unlock()
			errCh <- fmt.Errorf("start watcher: %w", err)
			cancel()
			return
		}
		a.watcherStarted = true
		a.rebuildMu.Unlock()
		<-ctx.Done()
		_ = a.watcher.Stop()
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := a.watchConfig(ctx); err != nil && err != context.Canceled {
			errCh <- fmt.Errorf("config watcher: %w", err)
			cancel()
		}
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
	a.rebuildMu.Lock()
	defer a.rebuildMu.Unlock()
	return a.initialIndex(ctx)
}

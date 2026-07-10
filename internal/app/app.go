package app

import (
	"context"
	"fmt"
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
	mcpServer "github.com/quonaro/gnostis/internal/mcp"
	"github.com/quonaro/gnostis/internal/project"
	"github.com/quonaro/gnostis/internal/search"
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

	mcpSrv := mcpServer.New(cfg.MCP.Name, cfg.MCP.Version, engine, symbolIndex, projects)
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
		mcp:            mcpSrv,
		embeddingCache: embeddingCache,
	}

	w := watcher.New(dirs, func(path string) {
		if err := reindexFile(context.Background(), path, dirs, projects, a.store, a.symbolIndex, a.provider, a.embeddingCache); err != nil {
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

// Run serves MCP immediately while performing initial indexing and starting the
// watcher in the background. The first component error stops the app.
func (a *App) Run(ctx context.Context) error {
	slog.InfoContext(ctx, "starting app")
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	errCh := make(chan error, 2)
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		slog.InfoContext(ctx, "serving mcp", "name", a.cfg.MCP.Name, "version", a.cfg.MCP.Version, "transport", a.cfg.MCP.Transport)
		var err error
		switch a.cfg.MCP.Transport {
		case "streamable-http":
			err = a.runHTTP(ctx)
		default:
			err = a.mcp.Start(ctx)
		}
		if err != nil {
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
		if err := a.mcp.StartHTTP(ctx, a.cfg.MCP.Address); err != nil {
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
		if err := indexDirectory(ctx, dir, a.projects[i], a.indexer, a.chunker, a.provider, a.store, a.symbolIndex, a.embeddingCache); err != nil {
			return fmt.Errorf("index %s: %w", dir.Path, err)
		}
	}
	if err := a.symbolIndex.Save(); err != nil {
		slog.ErrorContext(ctx, "save symbol index", "error", err)
	}
	slog.InfoContext(ctx, "initial index complete", "chunks", a.store.Count())
	return nil
}

func (a *App) cleanupDeletedFiles(ctx context.Context) {
	for _, path := range a.store.Paths() {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			slog.InfoContext(ctx, "removing deleted file from index", "path", path)
			_ = a.store.DeleteByPath(ctx, path)
			a.symbolIndex.RemoveByPath(path)
		}
	}
}

// Status returns the configured project names and current chunk count.
func (a *App) Status() ([]string, int) {
	names := make([]string, len(a.projects))
	for i, p := range a.projects {
		names[i] = p.Name
	}
	return names, a.store.Count()
}

// Info returns runtime metadata about the active provider and index.
func (a *App) Info() (provider, model string, symbols int) {
	return a.provider.ModelName(), a.cfg.Embeddings.Model, a.symbolIndex.Count()
}

// InitialIndex performs the first-time indexing of all configured directories.
func (a *App) InitialIndex(ctx context.Context) error {
	return a.initialIndex(ctx)
}

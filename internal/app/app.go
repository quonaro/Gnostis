package app

import (
	"context"
	"fmt"
	"os"

	"github.com/quonaro/gnostis/internal/chunker"
	"github.com/quonaro/gnostis/internal/config"
	"github.com/quonaro/gnostis/internal/directory"
	"github.com/quonaro/gnostis/internal/embeddings"
	"github.com/quonaro/gnostis/internal/indexer"
	mcpServer "github.com/quonaro/gnostis/internal/mcp"
	"github.com/quonaro/gnostis/internal/project"
	"github.com/quonaro/gnostis/internal/search"
	"github.com/quonaro/gnostis/internal/store"
	"github.com/quonaro/gnostis/internal/watcher"
)

// App orchestrates configuration, indexing, search, and the MCP server.
type App struct {
	cfg      config.Config
	dirs     []directory.Directory
	projects []project.Project
	store    *store.Store
	provider embeddings.Provider
	engine   *search.Engine
	indexer  *indexer.Indexer
	chunker  *chunker.Chunker
	watcher  *watcher.Watcher
	mcp      *mcpServer.Server
}

// New builds the application from configuration.
func New(cfg config.Config) (*App, error) {
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
	mcpSrv := mcpServer.New(cfg.MCP.Name, cfg.MCP.Version, engine, projects)

	w := watcher.New(dirs, func(path string) {
		if err := reindexFile(context.Background(), path, dirs, projects, st, provider); err != nil {
			fmt.Fprintf(os.Stderr, "reindex %s: %v\n", path, err)
		}
	})

	return &App{
		cfg:      cfg,
		dirs:     dirs,
		projects: projects,
		store:    st,
		provider: provider,
		engine:   engine,
		indexer:  indexer.New(),
		chunker:  chunker.New(),
		watcher:  w,
		mcp:      mcpSrv,
	}, nil
}

// Run performs initial indexing, starts the watcher, and serves MCP.
func (a *App) Run(ctx context.Context) error {
	if err := a.initialIndex(ctx); err != nil {
		return fmt.Errorf("initial index: %w", err)
	}

	if err := a.watcher.Start(); err != nil {
		return fmt.Errorf("start watcher: %w", err)
	}
	defer func() { _ = a.watcher.Stop() }()

	return a.mcp.Start(ctx)
}

func (a *App) initialIndex(ctx context.Context) error {
	for i, dir := range a.dirs {
		fmt.Fprintf(os.Stderr, "indexing %s...\n", dir.Path)
		if err := indexDirectory(ctx, dir, a.projects[i], a.indexer, a.chunker, a.provider, a.store); err != nil {
			return fmt.Errorf("index %s: %w", dir.Path, err)
		}
	}
	fmt.Fprintf(os.Stderr, "index complete: %d chunks\n", a.store.Count())
	return nil
}

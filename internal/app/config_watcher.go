package app

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/quonaro/gnostis/internal/config"
	"github.com/quonaro/gnostis/internal/watcher"
)

// watchConfig watches the config file for changes and reloads the configuration.
func (a *App) watchConfig(ctx context.Context) error {
	if a.ConfigPath == "" {
		return nil
	}

	cfgDir := filepath.Dir(a.ConfigPath)

	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("create config watcher: %w", err)
	}
	defer func() { _ = fw.Close() }()

	if err := fw.Add(cfgDir); err != nil {
		return fmt.Errorf("watch config directory %s: %w", cfgDir, err)
	}

	slog.InfoContext(ctx, "watching config file", "path", a.ConfigPath)

	var debounce *time.Timer
	reset := func() {
		if debounce != nil {
			if !debounce.Stop() {
				select {
				case <-debounce.C:
				default:
				}
			}
		}
		debounce = time.NewTimer(200 * time.Millisecond)
	}
	defer func() {
		if debounce != nil {
			debounce.Stop()
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event, ok := <-fw.Events:
			if !ok {
				return nil
			}
			if filepath.Base(event.Name) != filepath.Base(a.ConfigPath) {
				continue
			}
			if event.Op&fsnotify.Write == 0 && event.Op&fsnotify.Create == 0 && event.Op&fsnotify.Rename == 0 {
				continue
			}
			reset()
		case <-func() <-chan time.Time {
			if debounce == nil {
				return nil
			}
			return debounce.C
		}():
			if err := a.ReloadConfig(ctx); err != nil {
				slog.ErrorContext(ctx, "reload config", "error", err)
			} else {
				slog.InfoContext(ctx, "config reloaded")
			}
			debounce = nil
		case err, ok := <-fw.Errors:
			if !ok {
				return nil
			}
			slog.ErrorContext(ctx, "config watcher error", "error", err)
		}
	}
}

// newWatcher creates a fresh filesystem watcher with the current directory list.
func (a *App) newWatcher() *watcher.Watcher {
	return watcher.New(a.dirs, func(path string) {
		a.rebuildMu.Lock()
		defer a.rebuildMu.Unlock()

		if err := reindexFile(context.Background(), path, a.dirs, a.projects, a.cfg, a.store, a.symbolIndex, a.provider, a.embeddingCache, a.indexingStats); err != nil {
			slog.Error("reindex file", "path", path, "error", err)
			return
		}
		if err := a.symbolIndex.Save(); err != nil {
			slog.Error("save symbol index", "error", err)
		}
	})
}

// restartWatcher stops the current watcher and starts a new one with the current
// directory list. It must be called with rebuildMu held.
func (a *App) restartWatcher(ctx context.Context) error {
	if !a.watcherStarted {
		a.watcher = a.newWatcher()
		return nil
	}

	if a.watcher != nil {
		if err := a.watcher.Stop(); err != nil {
			slog.ErrorContext(ctx, "stop watcher", "error", err)
		}
	}

	a.watcher = a.newWatcher()
	if err := a.watcher.Start(); err != nil {
		return fmt.Errorf("start watcher: %w", err)
	}
	return nil
}

// ReloadConfig reloads the configuration from disk and updates the project list.
func (a *App) ReloadConfig(ctx context.Context) error {
	path := a.ConfigPath
	if path == "" {
		resolved, err := config.ResolvePath("")
		if err != nil {
			return fmt.Errorf("resolve config path: %w", err)
		}
		path = resolved
	}

	cfg, err := config.Load(path)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Preserve runtime settings that cannot be changed without a restart.
	cfg.DataDir = a.cfg.DataDir
	cfg.Embeddings = a.cfg.Embeddings
	cfg.MCP.Address = a.cfg.MCP.Address
	cfg.MCP.Token = a.cfg.MCP.Token
	if cfg.MCP.Version == "" {
		cfg.MCP.Version = a.cfg.MCP.Version
	}
	if cfg.MCP.Name == "" {
		cfg.MCP.Name = a.cfg.MCP.Name
	}

	dirs, projects, err := resolveProjects(cfg)
	if err != nil {
		return fmt.Errorf("resolve projects: %w", err)
	}

	a.rebuildMu.Lock()
	defer a.rebuildMu.Unlock()

	a.cfg = cfg
	a.dirs = dirs
	a.projects = projects
	a.updateSnapshots(cfg, projects)

	if a.mcp != nil {
		a.mcp.ReloadProjects(projects)
	}

	if err := a.restartWatcher(ctx); err != nil {
		return fmt.Errorf("restart watcher: %w", err)
	}

	return nil
}

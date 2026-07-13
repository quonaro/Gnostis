package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/quonaro/gnostis/internal/chat_providers"
	"github.com/quonaro/gnostis/internal/chat_providers/all"
	"github.com/quonaro/gnostis/internal/config"
)

const chatSyncInterval = 5 * time.Minute

// chatManager keeps the decrypted Markdown cache in sync with local chat providers.
type chatManager struct {
	cfg      config.Cascade
	registry *all.Registry
	exporter chat_providers.Exporter
	watcher  *fsnotify.Watcher
	stop     chan struct{}
	wg       sync.WaitGroup
	onExport func(string)
}

func newChatManager(cfg config.Cascade, onExport func(string)) *chatManager {
	return &chatManager{
		cfg:      cfg,
		registry: all.NewRegistry(),
		exporter: chat_providers.Exporter{
			MinUserMessageLength: cfg.MinUserMessageLength,
		},
		onExport: onExport,
		stop:     make(chan struct{}),
	}
}

// Start performs an initial batch export and begins watching source directories.
func (m *chatManager) Start(ctx context.Context) error {
	if err := os.MkdirAll(m.cfg.DataDir, 0o755); err != nil {
		return fmt.Errorf("create chat data dir: %w", err)
	}

	if err := m.syncAll(ctx); err != nil {
		slog.ErrorContext(ctx, "initial chat sync failed", "error", err)
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("create chat watcher: %w", err)
	}
	m.watcher = watcher

	for _, p := range m.registry.Providers() {
		for _, src := range p.Discover() {
			if err := m.watchRecursive(src); err != nil {
				slog.WarnContext(ctx, "watch chat source dir", "provider", p.Name(), "path", src, "error", err)
			}
		}
	}

	m.wg.Add(1)
	go m.run(ctx)

	return nil
}

// Stop shuts down the chat watcher.
func (m *chatManager) Stop() error {
	close(m.stop)
	m.wg.Wait()
	if m.watcher != nil {
		return m.watcher.Close()
	}
	return nil
}

func (m *chatManager) watchRecursive(root string) error {
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if err := m.watcher.Add(path); err != nil {
				return fmt.Errorf("watch %s: %w", path, err)
			}
		}
		return nil
	})
}

func (m *chatManager) run(ctx context.Context) {
	defer m.wg.Done()
	ticker := time.NewTicker(chatSyncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-m.stop:
			return
		case <-ticker.C:
			if err := m.syncAll(ctx); err != nil {
				slog.ErrorContext(ctx, "periodic chat sync failed", "error", err)
			}
		case event, ok := <-m.watcher.Events:
			if !ok {
				return
			}
			if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
				if err := m.exportFile(ctx, event.Name); err != nil {
					slog.ErrorContext(ctx, "export chat file", "path", event.Name, "error", err)
				}
			}
		case err, ok := <-m.watcher.Errors:
			if !ok {
				return
			}
			slog.ErrorContext(ctx, "chat watcher error", "error", err)
		}
	}
}

func (m *chatManager) syncAll(ctx context.Context) error {
	for _, p := range m.registry.Providers() {
		for _, src := range p.Discover() {
			entries, err := os.ReadDir(src)
			if err != nil {
				if os.IsNotExist(err) {
					continue
				}
				slog.WarnContext(ctx, "read chat source dir", "provider", p.Name(), "path", src, "error", err)
				continue
			}
			for _, entry := range entries {
				if entry.IsDir() {
					continue
				}
				path := filepath.Join(src, entry.Name())
				if err := m.exportFile(ctx, path); err != nil {
					slog.ErrorContext(ctx, "export chat file", "provider", p.Name(), "path", path, "error", err)
				}
			}
		}
	}
	return nil
}

func (m *chatManager) exportFile(ctx context.Context, path string) error {
	for _, p := range m.registry.Providers() {
		for _, ext := range supportedExtensions(p) {
			if !strings.HasSuffix(path, ext) {
				continue
			}
			plaintext, err := p.Decrypt(path)
			if err != nil {
				// File may not belong to this provider; try the next one.
				continue
			}
			mdPath, err := m.exporter.ExportSession(p, path, m.cfg.DataDir, plaintext)
			if err != nil {
				return fmt.Errorf("export %s: %w", p.Name(), err)
			}
			slog.DebugContext(ctx, "chat export", "provider", p.Name(), "source", path, "md", mdPath)
			if m.onExport != nil {
				m.onExport(mdPath)
			}
			return nil
		}
	}
	return nil
}

func supportedExtensions(p chat_providers.Provider) []string {
	// Default extension mapping per provider name. In the future providers can
	// expose their own extensions.
	switch p.Name() {
	case "cascade":
		return []string{".pb"}
	default:
		return []string{".pb"}
	}
}

package memory

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

	"github.com/quonaro/gnostis/internal/chat_providers/all"
	"github.com/quonaro/gnostis/internal/config"
	"github.com/quonaro/gnostis/internal/embeddings"
)

const syncInterval = 5 * time.Minute

// Manager keeps exported memory Markdown files in sync with local chat providers.
type Manager struct {
	dataDir   string
	providers []*Provider
	indexer   *Indexer
	watcher   *fsnotify.Watcher
	stop      chan struct{}
	wg        sync.WaitGroup
	provider  embeddings.Provider
	cache     map[string][]float32
}

// NewManager creates a memory manager from configuration.
func NewManager(cfg config.Memory, dataDir string, provider embeddings.Provider) (*Manager, error) {
	registry := all.NewRegistry()
	var providers []*Provider
	for _, p := range registry.Providers() {
		var pc config.ProviderConfig
		switch p.Name() {
		case "cascade":
			pc = cfg.Cascade
		case "cursor":
			pc = cfg.Cursor
		}
		providers = append(providers, NewProvider(p, pc))
	}

	store, err := NewStore(context.Background(), dataDir)
	if err != nil {
		return nil, fmt.Errorf("open memory store: %w", err)
	}

	return &Manager{
		dataDir:   dataDir,
		providers: providers,
		indexer:   NewIndexer(store),
		stop:      make(chan struct{}),
		provider:  provider,
		cache:     make(map[string][]float32),
	}, nil
}

// Start creates the memory data directory, begins watching source directories,
// and performs the initial export/index in the background so that MCP startup
// is not blocked by large dialogue backlogs.
func (m *Manager) Start(ctx context.Context) error {
	if err := os.MkdirAll(m.dataDir, 0o755); err != nil {
		return fmt.Errorf("create memory data dir: %w", err)
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("create memory watcher: %w", err)
	}
	m.watcher = watcher

	for _, p := range m.providers {
		if !p.Enabled() {
			continue
		}
		for _, src := range p.SourceDirs() {
			if err := m.watchRecursive(src); err != nil {
				slog.WarnContext(ctx, "watch memory source dir", "provider", p.Name(), "path", src, "error", err)
			}
		}
	}

	m.wg.Add(1)
	go m.run(ctx)

	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		if err := m.syncAll(ctx); err != nil {
			slog.ErrorContext(ctx, "initial memory sync failed", "error", err)
		}
	}()

	return nil
}

// Stop shuts down the memory watcher.
func (m *Manager) Stop() error {
	close(m.stop)
	m.wg.Wait()
	if m.watcher != nil {
		return m.watcher.Close()
	}
	return nil
}

func (m *Manager) watchRecursive(root string) error {
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

func (m *Manager) run(ctx context.Context) {
	defer m.wg.Done()
	ticker := time.NewTicker(syncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-m.stop:
			return
		case <-ticker.C:
			if err := m.syncAll(ctx); err != nil {
				slog.ErrorContext(ctx, "periodic memory sync failed", "error", err)
			}
		case event, ok := <-m.watcher.Events:
			if !ok {
				return
			}
			if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
				if err := m.exportFile(ctx, event.Name); err != nil {
					slog.ErrorContext(ctx, "export memory file", "path", event.Name, "error", err)
				}
			}
		case err, ok := <-m.watcher.Errors:
			if !ok {
				return
			}
			slog.ErrorContext(ctx, "memory watcher error", "error", err)
		}
	}
}

func (m *Manager) syncAll(ctx context.Context) error {
	for _, p := range m.providers {
		if !p.Enabled() {
			continue
		}
		for _, src := range p.SourceDirs() {
			entries, err := os.ReadDir(src)
			if err != nil {
				if os.IsNotExist(err) {
					continue
				}
				slog.WarnContext(ctx, "read memory source dir", "provider", p.Name(), "path", src, "error", err)
				continue
			}
			for _, entry := range entries {
				if entry.IsDir() {
					continue
				}
				path := filepath.Join(src, entry.Name())
				if err := m.exportFile(ctx, path); err != nil {
					slog.ErrorContext(ctx, "export memory file", "provider", p.Name(), "path", path, "error", err)
				}
			}
		}
	}
	return nil
}

func (m *Manager) exportFile(ctx context.Context, path string) error {
	for _, p := range m.providers {
		if !p.Enabled() {
			continue
		}
		for _, ext := range supportedExtensions(p.Provider) {
			if !strings.HasSuffix(path, ext) {
				continue
			}
			plaintext, err := p.Decrypt(path)
			if err != nil {
				continue
			}
			mdPath, err := p.Exporter().ExportSession(p.Provider, path, m.dataDir, plaintext)
			if err != nil {
				return fmt.Errorf("export %s: %w", p.Name(), err)
			}
			slog.DebugContext(ctx, "memory export", "provider", p.Name(), "source", path, "md", mdPath)
			if err := m.indexer.IndexFile(ctx, p.Name(), mdPath, m.provider, m.cache); err != nil {
				return fmt.Errorf("index %s: %w", p.Name(), err)
			}
			return nil
		}
	}
	return nil
}

func supportedExtensions(p interface{ Name() string }) []string {
	switch p.Name() {
	case "cascade":
		return []string{".pb"}
	default:
		return []string{".pb"}
	}
}

// Store returns the underlying memory store for use by MCP tools.
func (m *Manager) Store() *Store {
	return m.indexer.store
}

// Provider returns the embedding provider used by the memory manager.
func (m *Manager) Provider() embeddings.Provider {
	return m.provider
}

// DataDir returns the directory where exported memory Markdown files are stored.
func (m *Manager) DataDir() string {
	return m.dataDir
}

// Providers returns the configured memory providers.
func (m *Manager) Providers() []*Provider {
	return m.providers
}

// WriteNote writes a user note to the memory data dir and indexes it.
func (m *Manager) WriteNote(ctx context.Context, title, content string, tags []string, providerID string) (string, error) {
	if err := os.MkdirAll(m.dataDir, 0o755); err != nil {
		return "", fmt.Errorf("create memory data dir: %w", err)
	}

	slug := slugify(title)
	if slug == "" {
		slug = "note"
	}
	filename := fmt.Sprintf("%s-%s.md", time.Now().UTC().Format("20060102-150405"), slug)
	path := filepath.Join(m.dataDir, filename)

	if err := writeNoteMarkdown(path, title, content, tags, providerID); err != nil {
		return "", fmt.Errorf("write note: %w", err)
	}

	if err := m.indexer.IndexFile(ctx, providerID, path, m.provider, m.cache); err != nil {
		return "", fmt.Errorf("index note: %w", err)
	}

	return path, nil
}

func writeNoteMarkdown(path, title, content string, tags []string, providerID string) error {
	var b strings.Builder
	b.WriteString("# ")
	b.WriteString(title)
	b.WriteString("\n\n")
	b.WriteString("- **Provider:** ")
	b.WriteString(providerID)
	b.WriteString("\n")
	if len(tags) > 0 {
		b.WriteString("- **Tags:** ")
		b.WriteString(strings.Join(tags, ", "))
		b.WriteString("\n")
	}
	b.WriteString("- **Saved:** ")
	b.WriteString(time.Now().UTC().Format(time.RFC3339))
	b.WriteString("\n\n")
	b.WriteString(content)
	b.WriteString("\n")

	return os.WriteFile(path, []byte(b.String()), 0o600)
}

var slugRe = strings.NewReplacer(
	" ", "-",
	"_", "-",
	"/", "-",
	"\\", "-",
	":", "-",
)

func slugify(title string) string {
	s := strings.ToLower(strings.TrimSpace(title))
	s = slugRe.Replace(s)
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteRune('-')
		}
	}
	s = strings.Trim(b.String(), "-")
	if len(s) > 60 {
		s = s[:60]
	}
	if s == "" {
		s = "note"
	}
	return s
}

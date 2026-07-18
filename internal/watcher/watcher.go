package watcher

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/quonaro/gnostis/internal/directory"
)

// ChangeFunc is called with the absolute path of a changed file.
type ChangeFunc func(path string)

// Watcher monitors directories and debounces file change notifications.
type Watcher struct {
	dirs     []directory.Directory
	onChange ChangeFunc
	watcher  *fsnotify.Watcher
	debounce time.Duration
	mu       sync.Mutex
	pending  map[string]bool
	timer    *time.Timer
	ctx      context.Context
	cancel   context.CancelFunc
}

// New creates a watcher for the given directories.
func New(dirs []directory.Directory, onChange ChangeFunc) *Watcher {
	return &Watcher{
		dirs:     dirs,
		onChange: onChange,
		debounce: 2 * time.Second,
		pending:  make(map[string]bool),
	}
}

// Start begins watching directories recursively.
func (w *Watcher) Start() error {
	if w.watcher != nil {
		_ = w.watcher.Close()
	}
	if w.cancel != nil {
		w.cancel()
	}

	w.ctx, w.cancel = context.WithCancel(context.Background())

	w.mu.Lock()
	w.pending = make(map[string]bool)
	if w.timer != nil {
		w.timer.Stop()
		w.timer = nil
	}
	w.mu.Unlock()

	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("create watcher: %w", err)
	}
	w.watcher = fw

	for _, dir := range w.dirs {
		slog.Info("watching directory", "path", dir.Path)
		if err := w.addRecursive(dir.Path); err != nil {
			slog.Error("watch directory", "path", dir.Path, "error", err)
		}
	}

	go w.run()
	return nil
}

// Stop shuts down the watcher.
func (w *Watcher) Stop() error {
	slog.Info("stopping watcher")
	if w.cancel != nil {
		w.cancel()
	}
	if w.watcher != nil {
		if err := w.watcher.Close(); err != nil {
			slog.Error("close watcher", "error", err)
		}
		w.watcher = nil
	}

	w.mu.Lock()
	if w.timer != nil {
		w.timer.Stop()
		w.timer = nil
	}
	w.pending = make(map[string]bool)
	w.mu.Unlock()

	return nil
}

func (w *Watcher) addRecursive(root string) error {
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			slog.Error("walk directory", "path", path, "error", err)
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		if err := w.watcher.Add(path); err != nil {
			slog.Error("watch dir", "path", path, "error", err)
			return filepath.SkipDir
		}
		return nil
	})
}

func (w *Watcher) run() {
	for {
		select {
		case <-w.ctx.Done():
			return
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			w.handleEvent(event)
		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			slog.Error("watcher error", "error", err)
		}
	}
}

func (w *Watcher) handleEvent(event fsnotify.Event) {
	slog.Debug("filesystem event", "path", event.Name, "op", event.Op.String())

	w.mu.Lock()
	defer w.mu.Unlock()

	if w.ctx.Err() != nil {
		return
	}

	if event.Op == fsnotify.Create {
		info, err := os.Stat(event.Name)
		if err == nil && info.IsDir() {
			_ = w.addRecursive(event.Name)
		}
	}

	if !isWatchedOp(event.Op) {
		return
	}

	w.pending[event.Name] = true
	if w.timer != nil {
		w.timer.Stop()
	}
	w.timer = time.AfterFunc(w.debounce, w.flush)
}

func (w *Watcher) flush() {
	w.mu.Lock()
	paths := make([]string, 0, len(w.pending))
	for p := range w.pending {
		paths = append(paths, p)
	}
	w.pending = make(map[string]bool)
	w.timer = nil
	w.mu.Unlock()

	slog.Debug("flushing filesystem changes", "count", len(paths))
	for _, p := range paths {
		w.onChange(p)
	}
}

func isWatchedOp(op fsnotify.Op) bool {
	return op&fsnotify.Write == fsnotify.Write ||
		op&fsnotify.Create == fsnotify.Create ||
		op&fsnotify.Rename == fsnotify.Rename ||
		op&fsnotify.Remove == fsnotify.Remove
}

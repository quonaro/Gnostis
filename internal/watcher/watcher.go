package watcher

import (
	"context"
	"fmt"
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
	ctx, cancel := context.WithCancel(context.Background())
	return &Watcher{
		dirs:     dirs,
		onChange: onChange,
		debounce: 2 * time.Second,
		pending:  make(map[string]bool),
		ctx:      ctx,
		cancel:   cancel,
	}
}

// Start begins watching directories recursively.
func (w *Watcher) Start() error {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("create watcher: %w", err)
	}
	w.watcher = fw

	for _, dir := range w.dirs {
		if err := w.addRecursive(dir.Path); err != nil {
			return fmt.Errorf("watch %s: %w", dir.Path, err)
		}
	}

	go w.run()
	return nil
}

// Stop shuts down the watcher.
func (w *Watcher) Stop() error {
	w.cancel()
	if w.watcher != nil {
		return w.watcher.Close()
	}
	return nil
}

func (w *Watcher) addRecursive(root string) error {
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if err := w.watcher.Add(path); err != nil {
				return fmt.Errorf("watch dir %s: %w", path, err)
			}
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
			fmt.Fprintf(os.Stderr, "watcher error: %v\n", err)
		}
	}
}

func (w *Watcher) handleEvent(event fsnotify.Event) {
	if event.Op == fsnotify.Create {
		info, err := os.Stat(event.Name)
		if err == nil && info.IsDir() {
			_ = w.addRecursive(event.Name)
		}
	}

	if !isWatchedOp(event.Op) {
		return
	}

	w.mu.Lock()
	w.pending[event.Name] = true
	if w.timer != nil {
		w.timer.Stop()
	}
	w.timer = time.AfterFunc(w.debounce, w.flush)
	w.mu.Unlock()
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

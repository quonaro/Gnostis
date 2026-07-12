package stats

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Project holds per-project indexing metadata.
type Project struct {
	Chunks        int       `json:"chunks"`
	LastIndexedAt time.Time `json:"last_indexed_at"`
}

// Stats persists per-project indexing statistics.
type Stats struct {
	mu   sync.Mutex
	path string
	data map[string]Project
}

// New creates a Stats writer for the given file path.
func New(path string) *Stats {
	return &Stats{
		path: path,
		data: make(map[string]Project),
	}
}

// Load reads the persisted stats from disk. If the file does not exist, it
// returns an empty map.
func (s *Stats) Load() (map[string]Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]Project{}, nil
		}
		return nil, fmt.Errorf("read stats file: %w", err)
	}

	var loaded map[string]Project
	if err := json.Unmarshal(data, &loaded); err != nil {
		return nil, fmt.Errorf("decode stats file: %w", err)
	}
	s.data = loaded
	return loaded, nil
}

// Update records the chunk count and current time for the given project.
func (s *Stats) Update(project string, chunks int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.data[project] = Project{
		Chunks:        chunks,
		LastIndexedAt: time.Now().UTC(),
	}
	return s.saveLocked()
}

func (s *Stats) saveLocked() error {
	if s.path == "" {
		return nil
	}

	data, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal stats: %w", err)
	}

	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create stats dir: %w", err)
	}

	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write stats temp file: %w", err)
	}

	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("rename stats file: %w", err)
	}

	return nil
}

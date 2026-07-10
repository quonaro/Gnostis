package symbol

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Location describes a single symbol definition.
type Location struct {
	ProjectID string `json:"project_id"`
	Path      string `json:"path"`
	Language  string `json:"language"`
	Symbol    string `json:"symbol"`
	Signature string `json:"signature"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
}

// Chunk is a minimal subset of chunker.Chunk used to feed the symbol index.
type Chunk struct {
	ProjectID string
	Path      string
	Language  string
	Symbol    string
	Signature string
	StartLine int
	EndLine   int
}

// Index maps symbol names to their definition locations.
// It is safe for concurrent use.
type Index struct {
	mu   sync.RWMutex
	data map[string][]Location
	path string
}

// New opens or creates a symbol index persisted at path.
func New(path string) (*Index, error) {
	idx := &Index{
		data: make(map[string][]Location),
		path: path,
	}
	if err := idx.load(); err != nil {
		return nil, fmt.Errorf("load symbol index: %w", err)
	}
	return idx, nil
}

// Add records a symbol location. Empty symbols are ignored.
func (idx *Index) Add(loc Location) {
	if loc.Symbol == "" {
		return
	}
	key := strings.ToLower(loc.Symbol)
	idx.mu.Lock()
	defer idx.mu.Unlock()
	idx.data[key] = append(idx.data[key], loc)
}

// AddChunks adds locations extracted from generic chunks that expose the
// required fields.
func (idx *Index) AddChunks(chunks []Chunk) {
	for _, ch := range chunks {
		if ch.Symbol == "" {
			continue
		}
		idx.Add(Location(ch))
	}
}

// RemoveByPath deletes all locations belonging to a file path.
func (idx *Index) RemoveByPath(path string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	for key, locs := range idx.data {
		kept := locs[:0]
		for _, loc := range locs {
			if loc.Path != path {
				kept = append(kept, loc)
			}
		}
		if len(kept) == 0 {
			delete(idx.data, key)
		} else {
			idx.data[key] = kept
		}
	}
}

// Lookup returns exact matches for a symbol name (case-insensitive).
func (idx *Index) Lookup(name string) []Location {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return append([]Location(nil), idx.data[strings.ToLower(name)]...)
}

// Count returns the total number of stored symbol locations.
func (idx *Index) Count() int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	total := 0
	for _, locs := range idx.data {
		total += len(locs)
	}
	return total
}

// SearchFuzzy returns locations whose symbol contains the query substring.
func (idx *Index) SearchFuzzy(query string) []Location {
	q := strings.ToLower(query)
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	var out []Location
	for key, locs := range idx.data {
		if key == q || strings.Contains(key, q) {
			out = append(out, locs...)
		}
	}
	return out
}

// Save persists the index to disk.
func (idx *Index) Save() error {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	if err := os.MkdirAll(filepath.Dir(idx.path), 0o750); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}
	data, err := json.Marshal(idx.data)
	if err != nil {
		return fmt.Errorf("marshal index: %w", err)
	}
	tmp := idx.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o640); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}
	return os.Rename(tmp, idx.path)
}

func (idx *Index) load() error {
	info, err := os.Stat(idx.path)
	if os.IsNotExist(err) || (info != nil && info.Size() == 0) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("stat index: %w", err)
	}
	data, err := os.ReadFile(idx.path)
	if err != nil {
		return fmt.Errorf("read index: %w", err)
	}
	return json.Unmarshal(data, &idx.data)
}

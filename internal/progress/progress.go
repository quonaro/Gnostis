package progress

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Status values for a rebuild operation.
const (
	StatusIdle    = "idle"
	StatusRunning = "running"
	StatusError   = "error"
	StatusDone    = "done"
)

// Phase values describe the current stage of a rebuild.
const (
	PhaseIdle      = ""
	PhaseIndexing  = "indexing"
	PhaseChunking  = "chunking"
	PhaseEmbedding = "embedding"
)

// State describes the progress of an index rebuild.
type State struct {
	JobID       string    `json:"job_id,omitempty"`
	Status      string    `json:"status"`
	Phase       string    `json:"phase"`
	Project     string    `json:"project"`
	TotalFiles  int       `json:"total_files"`
	DoneFiles   int       `json:"done_files"`
	TotalChunks int       `json:"total_chunks"`
	DoneChunks  int       `json:"done_chunks"`
	PID         int       `json:"pid"`
	StartedAt   time.Time `json:"started_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Error       string    `json:"error,omitempty"`
}

// Progress persists rebuild progress to a JSON file.
type Progress struct {
	mu    sync.Mutex
	path  string
	jobID string
	state State
}

// New creates a Progress writer for the given file path.
func New(path string) *Progress {
	return &Progress{path: path}
}

// Load reads the persisted state from disk. If the file does not exist, it
// returns an idle state.
func (p *Progress) Load() (State, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	data, err := os.ReadFile(p.path)
	if err != nil {
		if os.IsNotExist(err) {
			p.state = State{Status: StatusIdle}
			return p.state, nil
		}
		return State{}, fmt.Errorf("read progress file: %w", err)
	}

	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return State{}, fmt.Errorf("decode progress file: %w", err)
	}
	p.state = s
	if p.jobID != "" {
		p.state.JobID = p.jobID
	}
	return p.state, nil
}

// SetJobID sets the identifier of the current rebuild job.
// It is preserved across Start/SetPhase/AddFiles calls.
func (p *Progress) SetJobID(id string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.jobID = id
	p.state.JobID = id
	_ = p.saveLocked()
}

// Start resets the state for a new rebuild of the given project.
func (p *Progress) Start(project string, totalFiles int) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now().UTC()
	p.state = State{
		JobID:      p.jobID,
		Status:     StatusRunning,
		Phase:      PhaseIndexing,
		Project:    project,
		TotalFiles: totalFiles,
		PID:        os.Getpid(),
		StartedAt:  now,
		UpdatedAt:  now,
	}
	return p.saveLocked()
}

// SetPhase updates the current phase.
func (p *Progress) SetPhase(phase string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.state.Phase = phase
	p.state.UpdatedAt = time.Now().UTC()
	return p.saveLocked()
}

// SetTotalChunks updates the total number of chunks to embed.
func (p *Progress) SetTotalChunks(n int) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.state.TotalChunks = n
	p.state.UpdatedAt = time.Now().UTC()
	return p.saveLocked()
}

// AddFiles increments the number of processed files.
func (p *Progress) AddFiles(n int) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.state.DoneFiles += n
	p.state.UpdatedAt = time.Now().UTC()
	return p.saveLocked()
}

// AddChunks increments the number of embedded chunks.
func (p *Progress) AddChunks(n int) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.state.DoneChunks += n
	p.state.UpdatedAt = time.Now().UTC()
	return p.saveLocked()
}

// Reset clears any running/error state and returns to idle.
func (p *Progress) Reset() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.state = State{Status: StatusIdle, UpdatedAt: time.Now().UTC()}
	return p.saveLocked()
}

// Done marks the rebuild as successfully completed.
func (p *Progress) Done() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.state.Status = StatusDone
	p.state.Phase = PhaseIdle
	p.state.UpdatedAt = time.Now().UTC()
	return p.saveLocked()
}

// Fail marks the rebuild as failed with the given error.
func (p *Progress) Fail(err error) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.state.Status = StatusError
	if err != nil {
		p.state.Error = err.Error()
	}
	p.state.UpdatedAt = time.Now().UTC()
	return p.saveLocked()
}

// State returns a copy of the current state.
func (p *Progress) State() State {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.state
}

func (p *Progress) saveLocked() error {
	if p.path == "" {
		return nil
	}

	data, err := json.MarshalIndent(p.state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal progress: %w", err)
	}

	dir := filepath.Dir(p.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create progress dir: %w", err)
	}

	tmp := p.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write progress temp file: %w", err)
	}

	if err := os.Rename(tmp, p.path); err != nil {
		return fmt.Errorf("rename progress file: %w", err)
	}

	return nil
}

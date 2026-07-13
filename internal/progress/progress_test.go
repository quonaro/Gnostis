package progress

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestProgressStateLifecycle(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "progress.json")
	p := New(path)

	if err := p.Start("test", 100); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	s, err := p.Load()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if s.Status != StatusRunning {
		t.Fatalf("expected status %q, got %q", StatusRunning, s.Status)
	}
	if s.Project != "test" {
		t.Fatalf("expected project %q, got %q", "test", s.Project)
	}
	if s.TotalFiles != 100 {
		t.Fatalf("expected total files 100, got %d", s.TotalFiles)
	}

	_ = p.SetPhase(PhaseEmbedding)
	_ = p.SetTotalChunks(42)
	_ = p.AddChunks(10)

	s, _ = p.Load()
	if s.Phase != PhaseEmbedding {
		t.Fatalf("expected phase %q, got %q", PhaseEmbedding, s.Phase)
	}
	if s.DoneChunks != 10 {
		t.Fatalf("expected done chunks 10, got %d", s.DoneChunks)
	}

	_ = p.Done()
	s, _ = p.Load()
	if s.Status != StatusDone {
		t.Fatalf("expected status %q, got %q", StatusDone, s.Status)
	}
}

func TestProgressFail(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "progress.json")
	p := New(path)

	_ = p.Start("test", 10)
	_ = p.Fail(os.ErrClosed)

	s, _ := p.Load()
	if s.Status != StatusError {
		t.Fatalf("expected status %q, got %q", StatusError, s.Status)
	}
	if s.Error == "" {
		t.Fatal("expected non-empty error message")
	}
}

func TestProgressLoadMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "missing.json")
	p := New(path)

	s, err := p.Load()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if s.Status != StatusIdle {
		t.Fatalf("expected status %q, got %q", StatusIdle, s.Status)
	}
}

func TestProgressLoadPreservesJobID(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "progress.json")
	if err := os.WriteFile(path, []byte(`{"job_id":"project:RuobrOld-123","status":"running","project":"RuobrOld"}`), 0o644); err != nil {
		t.Fatalf("write progress file: %v", err)
	}

	p := New(path)
	s, err := p.Load()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if s.JobID != "project:RuobrOld-123" {
		t.Fatalf("expected job id %q, got %q", "project:RuobrOld-123", s.JobID)
	}

	if err := p.Start("RuobrOld", 100); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	s, _ = p.Load()
	if s.JobID != "project:RuobrOld-123" {
		t.Fatalf("expected resumed job id %q, got %q", "project:RuobrOld-123", s.JobID)
	}
}

func TestStateETA(t *testing.T) {
	now := time.Now().UTC()
	s := State{
		Status:      StatusRunning,
		Phase:       PhaseEmbedding,
		StartedAt:   now.Add(-2 * time.Minute),
		UpdatedAt:   now,
		TotalChunks: 1000,
		DoneChunks:  100,
	}
	eta := s.ETA()
	if eta <= 0 {
		t.Fatalf("expected positive ETA, got %v", eta)
	}
	// 100 chunks in 2 minutes => rate 0.833 chunks/sec => 900 remaining => ~1080s.
	if eta < 15*time.Minute || eta > 21*time.Minute {
		t.Errorf("ETA = %v, want between 15m and 21m", eta)
	}
}

func TestStateETAEdgeCases(t *testing.T) {
	now := time.Now().UTC()
	cases := []struct {
		name  string
		state State
	}{
		{"idle", State{Status: StatusIdle, StartedAt: now, TotalChunks: 100, DoneChunks: 10}},
		{"zero done", State{Status: StatusRunning, StartedAt: now, TotalChunks: 100, DoneChunks: 0}},
		{"zero total", State{Status: StatusRunning, StartedAt: now, TotalChunks: 0, DoneChunks: 0}},
		{"all done", State{Status: StatusRunning, StartedAt: now, TotalChunks: 100, DoneChunks: 100}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if eta := tc.state.ETA(); eta != 0 {
				t.Errorf("expected zero ETA, got %v", eta)
			}
		})
	}
}

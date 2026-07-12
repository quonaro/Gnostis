package progress

import (
	"os"
	"path/filepath"
	"testing"
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

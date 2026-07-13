package memory

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/quonaro/gnostis/internal/config"
)

type mockEmbedder struct{}

func (m *mockEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i := range texts {
		out[i] = []float32{1, 0, 0, 0}
	}
	return out, nil
}

func (m *mockEmbedder) BatchSize() int    { return 32 }
func (m *mockEmbedder) ModelName() string { return "mock" }

func TestManagerWriteNote(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	cfg := config.Memory{
		Cascade: config.ProviderConfig{Enabled: true},
	}
	mgr, err := NewManager(cfg, dir, &mockEmbedder{})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	path, err := mgr.WriteNote(ctx, "Test Note", "This is a test note.", []string{"test"}, "cascade")
	if err != nil {
		t.Fatalf("WriteNote: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("note file not created: %v", err)
	}
	if !strings.HasPrefix(path, dir) {
		t.Errorf("note path %q not under data dir %q", path, dir)
	}
	if mgr.Store().Count() != 1 {
		t.Errorf("store count = %d, want 1", mgr.Store().Count())
	}
}

func TestManagerProviderConfig(t *testing.T) {
	src := t.TempDir()
	cfg := config.Memory{
		Cascade: config.ProviderConfig{
			Enabled:              true,
			SourceDirs:           []string{src},
			MinUserMessageLength: 42,
		},
	}
	mgr, err := NewManager(cfg, t.TempDir(), &mockEmbedder{})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	for _, p := range mgr.Providers() {
		if p.Name() == "cascade" {
			if !p.Enabled() {
				t.Fatal("cascade provider should be enabled")
			}
			if got := p.MinUserMessageLength(); got != 42 {
				t.Errorf("min length = %d, want 42", got)
			}
			if got := p.SourceDirs(); len(got) != 1 || got[0] != src {
				t.Errorf("source dirs = %v, want [%s]", got, src)
			}
		}
	}
}

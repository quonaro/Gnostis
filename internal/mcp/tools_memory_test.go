package mcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/quonaro/gnostis/internal/config"
	"github.com/quonaro/gnostis/internal/memory"
)

type mockMemoryEmbedder struct{}

func (m *mockMemoryEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i := range texts {
		out[i] = []float32{1, 0, 0, 0}
	}
	return out, nil
}

func (m *mockMemoryEmbedder) BatchSize() int    { return 32 }
func (m *mockMemoryEmbedder) ModelName() string { return "mock" }

func newMemoryServer(t *testing.T) (*Server, *memory.Manager, string) {
	t.Helper()
	dir := t.TempDir()
	cfg := config.Memory{Cascade: config.ProviderConfig{Enabled: true}}
	mgr, err := memory.NewManager(cfg, dir, &mockMemoryEmbedder{})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	srv := New("test", "1.0.0", &mockSearcher{}, nil, nil, mgr, nil)
	return srv, mgr, dir
}

func TestMemoryWrite_RequiredFields(t *testing.T) {
	srv, _, _ := newMemoryServer(t)

	res, err := srv.memoryWrite(context.Background(), mcp.CallToolRequest{}, memoryWriteArgs{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected error result for empty title")
	}
	if !strings.Contains(extractText(t, res), "title is required") {
		t.Fatalf("unexpected error text: %s", extractText(t, res))
	}
}

func TestMemoryWrite_WritesAndIndexes(t *testing.T) {
	srv, mgr, _ := newMemoryServer(t)

	res, err := srv.memoryWrite(context.Background(), mcp.CallToolRequest{}, memoryWriteArgs{
		Title:   "Important Decision",
		Content: "Use Ollama for embeddings.",
		Tags:    []string{"embedding", "decision"},
	})
	if err != nil {
		t.Fatalf("memoryWrite: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", extractText(t, res))
	}

	var got memoryWriteResult
	if err := json.Unmarshal([]byte(extractText(t, res)), &got); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	content, err := os.ReadFile(got.Path)
	if err != nil {
		t.Fatalf("read note: %v", err)
	}
	if !strings.Contains(string(content), "Important Decision") {
		t.Errorf("note does not contain title")
	}
	if !strings.Contains(string(content), "Use Ollama for embeddings.") {
		t.Errorf("note does not contain content")
	}

	if mgr.Store().Count() != 1 {
		t.Errorf("store count = %d, want 1", mgr.Store().Count())
	}
}

func TestMemoryRead(t *testing.T) {
	srv, _, dir := newMemoryServer(t)

	path := filepath.Join(dir, "note.md")
	if err := os.WriteFile(path, []byte("# Test\n\ncontent\n"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	res, err := srv.memoryRead(context.Background(), mcp.CallToolRequest{}, memoryReadArgs{Path: path})
	if err != nil {
		t.Fatalf("memoryRead: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", extractText(t, res))
	}
	if !strings.Contains(extractText(t, res), "content") {
		t.Errorf("read content missing expected text")
	}
}

func TestMemoryRead_OutsideDir(t *testing.T) {
	srv, _, _ := newMemoryServer(t)

	res, err := srv.memoryRead(context.Background(), mcp.CallToolRequest{}, memoryReadArgs{Path: "/etc/passwd"})
	if err != nil {
		t.Fatalf("memoryRead: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected error for path outside memory dir")
	}
}

func TestMemoryList(t *testing.T) {
	srv, _, dir := newMemoryServer(t)

	path := filepath.Join(dir, "note.md")
	if err := os.WriteFile(path, []byte("# Test\n\n- **Provider:** cascade\n\ncontent\n"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	res, err := srv.memoryList(context.Background(), mcp.CallToolRequest{}, memoryListArgs{})
	if err != nil {
		t.Fatalf("memoryList: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", extractText(t, res))
	}

	var got []memoryListResult
	if err := json.Unmarshal([]byte(extractText(t, res)), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("list results = %d, want 1", len(got))
	}
}

func TestMemorySearch_Disabled(t *testing.T) {
	srv := New("test", "1.0.0", &mockSearcher{}, nil, nil, nil, nil)

	res, err := srv.memorySearch(context.Background(), mcp.CallToolRequest{}, memorySearchArgs{Query: "test"})
	if err != nil {
		t.Fatalf("memorySearch: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected error when memory disabled")
	}
}

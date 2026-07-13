package mcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/quonaro/gnostis/internal/project"
)

func TestWriteDialog_RequiredFields(t *testing.T) {
	srv := New("test", "1.0.0", &mockSearcher{}, nil, &mockIndexer{}, nil)

	res, err := srv.writeDialog(context.Background(), mcp.CallToolRequest{}, writeDialogArgs{})
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

func TestWriteDialog_WritesAndReindexes(t *testing.T) {
	dir := t.TempDir()
	idx := &mockIndexer{}
	srv := New("test", "1.0.0", &mockSearcher{}, nil, idx, []project.Project{{ID: cascadeProjectID, Name: cascadeProjectID, Path: dir}})

	res, err := srv.writeDialog(context.Background(), mcp.CallToolRequest{}, writeDialogArgs{
		Title:   "Important Decision",
		Content: "Use Ollama for embeddings.",
		Tags:    []string{"embedding", "decision"},
		Project: "gnostis",
	})
	if err != nil {
		t.Fatalf("writeDialog: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", extractText(t, res))
	}

	var got writeDialogResult
	if err := json.Unmarshal([]byte(extractText(t, res)), &got); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if !strings.HasPrefix(got.Path, filepath.Join(dir, "notes")) {
		t.Errorf("path = %q, expected prefix %q", got.Path, filepath.Join(dir, "notes"))
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

	if len(idx.paths) != 1 || idx.paths[0] != got.Path {
		t.Errorf("reindexed paths = %v, want [%s]", idx.paths, got.Path)
	}
}

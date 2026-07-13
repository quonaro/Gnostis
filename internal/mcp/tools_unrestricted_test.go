package mcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestFSRead_AbsolutePath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.txt")
	if err := os.WriteFile(path, []byte("line1\nline2\nline3\n"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	srv := New("test", "1.0.0", &mockSearcher{}, nil, nil, nil, nil)
	res, err := srv.fsRead(context.Background(), mcp.CallToolRequest{}, fsReadArgs{Path: path})
	if err != nil {
		t.Fatalf("fsRead: %v", err)
	}
	if extractText(t, res) != "line1\nline2\nline3\n" {
		t.Errorf("unexpected content: %q", extractText(t, res))
	}
}

func TestFSRead_LineRange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.txt")
	if err := os.WriteFile(path, []byte("a\nb\nc\nd\n"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	srv := New("test", "1.0.0", &mockSearcher{}, nil, nil, nil, nil)
	res, err := srv.fsRead(context.Background(), mcp.CallToolRequest{}, fsReadArgs{Path: path, StartLine: 2, EndLine: 3})
	if err != nil {
		t.Fatalf("fsRead: %v", err)
	}
	if extractText(t, res) != "b\nc" {
		t.Errorf("unexpected content: %q", extractText(t, res))
	}
}

func TestFSRead_NotAbsolute(t *testing.T) {
	srv := New("test", "1.0.0", &mockSearcher{}, nil, nil, nil, nil)
	res, err := srv.fsRead(context.Background(), mcp.CallToolRequest{}, fsReadArgs{Path: "relative/path.txt"})
	if err != nil {
		t.Fatalf("fsRead: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected error for relative path")
	}
	assertTextEquals(t, res, "path must be absolute")
}

func TestFSRead_MissingFile(t *testing.T) {
	srv := New("test", "1.0.0", &mockSearcher{}, nil, nil, nil, nil)
	res, err := srv.fsRead(context.Background(), mcp.CallToolRequest{}, fsReadArgs{Path: "/does/not/exist.txt"})
	if err != nil {
		t.Fatalf("fsRead: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected error for missing file")
	}
	assertTextEquals(t, res, "file not found")
}

func TestFSGrep(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("func Foo() {}\nfunc Bar() {}\n"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	srv := New("test", "1.0.0", &mockSearcher{}, nil, nil, nil, nil)
	res, err := srv.fsGrep(context.Background(), mcp.CallToolRequest{}, fsGrepArgs{Path: dir, Query: "Foo"})
	if err != nil {
		t.Fatalf("fsGrep: %v", err)
	}
	var matches []grepMatch
	if err := json.Unmarshal([]byte(extractText(t, res)), &matches); err != nil {
		t.Fatalf("unmarshal matches: %v", err)
	}
	if len(matches) != 1 || matches[0].Line != 1 {
		t.Errorf("unexpected matches: %+v", matches)
	}
}

func TestFSGrep_Regex(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("func Foo() {}\nfunc Bar() {}\n"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	srv := New("test", "1.0.0", &mockSearcher{}, nil, nil, nil, nil)
	res, err := srv.fsGrep(context.Background(), mcp.CallToolRequest{}, fsGrepArgs{Path: dir, Query: `func [A-Z][a-z]+`, Regex: true})
	if err != nil {
		t.Fatalf("fsGrep: %v", err)
	}
	var matches []grepMatch
	if err := json.Unmarshal([]byte(extractText(t, res)), &matches); err != nil {
		t.Fatalf("unmarshal matches: %v", err)
	}
	if len(matches) != 2 {
		t.Errorf("expected 2 regex matches, got %d", len(matches))
	}
}

func TestFSList(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("a"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.md"), []byte("b"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	srv := New("test", "1.0.0", &mockSearcher{}, nil, nil, nil, nil)
	res, err := srv.fsList(context.Background(), mcp.CallToolRequest{}, fsListArgs{Path: dir, Pattern: "*.go"})
	if err != nil {
		t.Fatalf("fsList: %v", err)
	}
	var files []fileEntry
	if err := json.Unmarshal([]byte(extractText(t, res)), &files); err != nil {
		t.Fatalf("unmarshal files: %v", err)
	}
	if len(files) != 1 || !strings.HasSuffix(files[0].Path, "a.go") {
		t.Errorf("unexpected files: %+v", files)
	}
}

func TestFSTree(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub", "file.go"), []byte("x"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	srv := New("test", "1.0.0", &mockSearcher{}, nil, nil, nil, nil)
	res, err := srv.fsTree(context.Background(), mcp.CallToolRequest{}, fsTreeArgs{Path: dir, Depth: 2})
	if err != nil {
		t.Fatalf("fsTree: %v", err)
	}
	var tree treeEntry
	if err := json.Unmarshal([]byte(extractText(t, res)), &tree); err != nil {
		t.Fatalf("unmarshal tree: %v", err)
	}
	if tree.Type != "dir" || len(tree.Children) != 1 {
		t.Errorf("unexpected tree: %+v", tree)
	}
}

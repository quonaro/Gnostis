package mcp

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/quonaro/gnostis/internal/search"
)

func TestUnifiedSearch_EmptyQuery(t *testing.T) {
	srv := New("test", "1.0.0", &mockSearcher{}, nil, nil, nil, nil)
	res, err := srv.unifiedSearch(context.Background(), mcp.CallToolRequest{}, unifiedSearchArgs{Source: searchSourceAll})
	if err != nil {
		t.Fatalf("unifiedSearch: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected error for empty query")
	}
	assertTextEquals(t, res, "query is required")
}

func TestUnifiedSearch_InvalidSource(t *testing.T) {
	srv := New("test", "1.0.0", &mockSearcher{}, nil, nil, nil, nil)
	res, err := srv.unifiedSearch(context.Background(), mcp.CallToolRequest{}, unifiedSearchArgs{Query: "x", Source: "bad"})
	if err != nil {
		t.Fatalf("unifiedSearch: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected error for invalid source")
	}
	assertTextEquals(t, res, "invalid source")
}

func TestUnifiedSearch_Code(t *testing.T) {
	mock := &mockSearcher{results: []search.Result{{
		ID:        "chunk-1",
		ProjectID: "proj",
		Path:      "/foo/bar.go",
		Language:  "go",
		Symbol:    "Bar",
		StartLine: 10,
		EndLine:   20,
		Score:     0.9,
		Content:   "func Bar() {}",
	}}}
	srv := New("test", "1.0.0", mock, nil, nil, nil, nil)
	res, err := srv.unifiedSearch(context.Background(), mcp.CallToolRequest{}, unifiedSearchArgs{Query: "bar", Source: searchSourceCode, IncludeContent: true})
	if err != nil {
		t.Fatalf("unifiedSearch: %v", err)
	}
	items := extractUnifiedResults(t, res)
	if len(items) != 1 {
		t.Fatalf("expected 1 result, got %d", len(items))
	}
	if items[0].Source != searchSourceCode || items[0].Language != "go" {
		t.Errorf("unexpected result: %+v", items[0])
	}
}

func TestUnifiedSearch_Docs(t *testing.T) {
	mock := &mockSearcher{results: []search.Result{{
		ID:        "doc-1",
		ProjectID: "proj",
		Path:      "/docs/README.md",
		Language:  "markdown",
		StartLine: 1,
		EndLine:   5,
		Score:     0.85,
		Content:   "readme",
	}}}
	srv := New("test", "1.0.0", mock, nil, nil, nil, nil)
	res, err := srv.unifiedSearch(context.Background(), mcp.CallToolRequest{}, unifiedSearchArgs{Query: "readme", Source: searchSourceDocs, IncludeContent: true})
	if err != nil {
		t.Fatalf("unifiedSearch: %v", err)
	}
	items := extractUnifiedResults(t, res)
	if len(items) != 1 || items[0].Source != searchSourceDocs {
		t.Errorf("unexpected results: %+v", items)
	}
}

func TestUnifiedSearch_All_Deduplicates(t *testing.T) {
	mock := &mockSearcher{results: []search.Result{{
		ID:        "chunk-1",
		ProjectID: "proj",
		Path:      "/foo/shared.go",
		Language:  "go",
		StartLine: 1,
		EndLine:   5,
		Score:     0.9,
		Content:   "shared",
	}}}
	srv := New("test", "1.0.0", mock, nil, nil, nil, nil)
	res, err := srv.unifiedSearch(context.Background(), mcp.CallToolRequest{}, unifiedSearchArgs{Query: "shared", Source: searchSourceAll})
	if err != nil {
		t.Fatalf("unifiedSearch: %v", err)
	}
	items := extractUnifiedResults(t, res)
	if len(items) != 1 {
		t.Errorf("expected 1 deduplicated result, got %d: %+v", len(items), items)
	}
}

func extractUnifiedResults(t *testing.T, res *mcp.CallToolResult) []unifiedSearchResult {
	t.Helper()
	var items []unifiedSearchResult
	if err := json.Unmarshal([]byte(extractText(t, res)), &items); err != nil {
		t.Fatalf("unmarshal unified results: %v", err)
	}
	return items
}

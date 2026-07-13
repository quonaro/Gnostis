package mcp

import (
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/quonaro/gnostis/internal/project"
)

func TestToolError_StructuredJSON(t *testing.T) {
	res := toolError(errReasonInvalidArgument, "query is required", "provide a query")
	if res == nil || !res.IsError {
		t.Fatal("expected error result")
	}
	text, ok := mcp.AsTextContent(res.Content[0])
	if !ok {
		t.Fatalf("expected text content, got %T", res.Content[0])
	}
	var body toolErrorResponse
	if err := json.Unmarshal([]byte(text.Text), &body); err != nil {
		t.Fatalf("unmarshal error body: %v", err)
	}
	if !body.Error || body.Reason != errReasonInvalidArgument || body.Message != "query is required" || body.Suggestion != "provide a query" {
		t.Errorf("unexpected body: %+v", body)
	}
}

func TestIsPathAllowed_PrefixSiblings(t *testing.T) {
	srv := New("test", "1.0.0", &mockSearcher{}, nil, nil, nil, []project.Project{
		{Name: "tmp", Path: "/tmp"},
	})

	if !srv.isPathAllowed("/tmp") {
		t.Errorf("expected exact project path to be allowed")
	}
	if !srv.isPathAllowed("/tmp/inside") {
		t.Errorf("expected child path to be allowed")
	}
	if srv.isPathAllowed("/tmp2") {
		t.Errorf("expected sibling path /tmp2 to be rejected")
	}
	if srv.isPathAllowed("/tmp2/inside") {
		t.Errorf("expected sibling path /tmp2/inside to be rejected")
	}
}

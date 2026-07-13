package mcp

import (
	"testing"

	"github.com/quonaro/gnostis/internal/project"
)

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

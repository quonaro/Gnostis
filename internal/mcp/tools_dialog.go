package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

const cascadeProjectID = "cascade-dialogues"

var slugRe = regexp.MustCompile(`[^a-z0-9-]+`)

// writeDialogArgs describes the model-facing write_dialog tool.
type writeDialogArgs struct {
	Title   string   `json:"title"`
	Content string   `json:"content"`
	Tags    []string `json:"tags,omitempty"`
	Project string   `json:"project,omitempty"`
}

// writeDialogResult is returned after a successful write.
type writeDialogResult struct {
	Path string `json:"path"`
}

func writeDialogTool() mcp.Tool {
	return mcp.NewTool("write_dialog",
		mcp.WithDescription("Persist a note, summary, or fact to the cascade-dialogues memory so it can be retrieved later via semantic search."),
		mcp.WithString("title", mcp.Required(), mcp.Description("Short title for the note")),
		mcp.WithString("content", mcp.Required(), mcp.Description("Body text to save")),
		mcp.WithArray("tags", mcp.Description("Optional tags for filtering")),
		mcp.WithString("project", mcp.Description("Optional project name this note relates to")),
	)
}

func (s *Server) writeDialog(ctx context.Context, request mcp.CallToolRequest, args writeDialogArgs) (*mcp.CallToolResult, error) {
	slog.InfoContext(ctx, "mcp tool call", "tool", "write_dialog", "title", args.Title)

	if strings.TrimSpace(args.Title) == "" {
		return mcp.NewToolResultError("title is required"), nil
	}
	if strings.TrimSpace(args.Content) == "" {
		return mcp.NewToolResultError("content is required"), nil
	}

	dir := s.dialoguesDir()
	notesDir := filepath.Join(dir, "notes")
	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		slog.ErrorContext(ctx, "write_dialog mkdir failed", "path", notesDir, "error", err)
		return nil, fmt.Errorf("create notes directory: %w", err)
	}

	slug := slugify(args.Title)
	if slug == "" {
		slug = "note"
	}
	filename := fmt.Sprintf("%s-%s.md", time.Now().UTC().Format("20060102-150405"), slug)
	path := filepath.Join(notesDir, filename)

	if err := writeDialogMarkdown(path, args); err != nil {
		slog.ErrorContext(ctx, "write_dialog write failed", "path", path, "error", err)
		return nil, fmt.Errorf("write note: %w", err)
	}

	if s.indexer != nil {
		if err := s.indexer.ReindexFiles(ctx, []string{path}); err != nil {
			slog.ErrorContext(ctx, "write_dialog reindex failed", "path", path, "error", err)
			return nil, fmt.Errorf("reindex note: %w", err)
		}
	}

	res := writeDialogResult{Path: path}
	data, err := json.Marshal(res)
	if err != nil {
		return nil, fmt.Errorf("marshal write_dialog result: %w", err)
	}
	return mcp.NewToolResultText(string(data)), nil
}

func (s *Server) dialoguesDir() string {
	s.mu.RLock()
	projects := s.projects
	s.mu.RUnlock()

	for _, p := range projects {
		if p.ID == cascadeProjectID {
			return p.Path
		}
	}

	home := os.Getenv("HOME")
	if home == "" {
		return "gnostis-dialogues"
	}
	return filepath.Join(home, ".gnostis", "data", "dialogues")
}

func writeDialogMarkdown(path string, args writeDialogArgs) error {
	var b strings.Builder
	b.WriteString("# ")
	b.WriteString(args.Title)
	b.WriteString("\n\n")
	if args.Project != "" {
		b.WriteString("- **Project:** ")
		b.WriteString(args.Project)
		b.WriteString("\n")
	}
	if len(args.Tags) > 0 {
		b.WriteString("- **Tags:** ")
		b.WriteString(strings.Join(args.Tags, ", "))
		b.WriteString("\n")
	}
	b.WriteString("- **Saved:** ")
	b.WriteString(time.Now().UTC().Format(time.RFC3339))
	b.WriteString("\n\n")
	b.WriteString(args.Content)
	b.WriteString("\n")

	return os.WriteFile(path, []byte(b.String()), 0o600)
}

func slugify(title string) string {
	s := strings.ToLower(strings.TrimSpace(title))
	s = slugRe.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if len(s) > 60 {
		s = s[:60]
	}
	return s
}

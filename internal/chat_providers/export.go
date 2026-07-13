package chat_providers

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Exporter writes decrypted chat trajectories to Markdown files.
type Exporter struct {
	MinUserMessageLength int
}

// ExportSession writes a single decrypted session to a Markdown file.
func (e Exporter) ExportSession(p Provider, sourcePath, destDir string, plaintext []byte) (string, error) {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", fmt.Errorf("create dest dir: %w", err)
	}

	base := filepath.Base(sourcePath)
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	mdPath := filepath.Join(destDir, stem+".md")

	turns := p.ExtractDialogue(plaintext)
	turns = e.filterTurns(turns)

	allStrings := p.ExtractStrings(plaintext)
	allStrings = deduplicateStrings(allStrings)

	var b strings.Builder
	fmt.Fprintf(&b, "# Chat trajectory: %s\n\n", stem)
	fmt.Fprintf(&b, "- **Provider:** %s\n", p.Name())
	fmt.Fprintf(&b, "- **Source:** `%s`\n", sourcePath)
	fmt.Fprintf(&b, "- **Decrypted size:** %d bytes\n", len(plaintext))
	fmt.Fprintf(&b, "- **Dialogue turns:** %d\n", len(turns))
	fmt.Fprintf(&b, "- **Extracted strings:** %d\n", len(allStrings))
	fmt.Fprintf(&b, "- **Exported:** %s\n", time.Now().UTC().Format(time.RFC3339))
	fmt.Fprintf(&b, "\n---\n\n")

	if len(turns) > 0 {
		fmt.Fprintf(&b, "## Dialogue\n\n")
		for _, turn := range turns {
			label := "Assistant"
			if turn.Role == "user" {
				label = "User"
			}
			fmt.Fprintf(&b, "### %s\n\n", label)
			fmt.Fprintf(&b, "%s\n\n", escapeMarkdownHeaders(turn.Content))
		}
	}

	fmt.Fprintf(&b, "## All extracted strings\n\n")
	for _, s := range allStrings {
		fmt.Fprintf(&b, "%s\n\n", escapeMarkdownHeaders(s))
	}

	if err := os.WriteFile(mdPath, []byte(b.String()), 0o600); err != nil {
		return "", fmt.Errorf("write markdown: %w", err)
	}
	return mdPath, nil
}

func (e Exporter) filterTurns(turns []Turn) []Turn {
	if e.MinUserMessageLength <= 0 {
		return turns
	}
	out := make([]Turn, 0, len(turns))
	for _, t := range turns {
		if t.Role == "user" && len(t.Content) < e.MinUserMessageLength {
			continue
		}
		out = append(out, t)
	}
	return out
}

func deduplicateStrings(strings []string) []string {
	seen := make(map[string]bool)
	out := make([]string, 0, len(strings))
	for _, s := range strings {
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

func escapeMarkdownHeaders(s string) string {
	return strings.ReplaceAll(s, "#", "\\#")
}

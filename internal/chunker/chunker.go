package chunker

import (
	"context"
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/quonaro/gnostis/internal/indexer"
)

// Chunk represents a single indexed symbol or document section.
type Chunk struct {
	ID        string
	ProjectID string
	Path      string
	FileHash  string
	Language  string
	Symbol    string
	Signature string
	Docstring string
	Content   string
	StartLine int
	EndLine   int
}

// Chunker splits files into symbol-level chunks.
type Chunker struct {
	handlers map[string]languageHandler
}

// New creates a Chunker with built-in language handlers.
func New() *Chunker {
	return &Chunker{handlers: buildHandlers()}
}

// ChunkFile produces chunks from a single file.
func (c *Chunker) ChunkFile(ctx context.Context, file indexer.FileInfo) ([]Chunk, error) {
	lang := detectLanguage(file.Path)
	if lang == "markdown" {
		return c.chunkMarkdown(file), nil
	}

	handler, ok := c.handlers[lang]
	if !ok {
		return c.chunkFallback(file, lang), nil
	}

	return c.chunkWithTreeSitter(ctx, file, handler), nil
}

func (c *Chunker) chunkWithTreeSitter(ctx context.Context, file indexer.FileInfo, handler languageHandler) []Chunk {
	parser := sitter.NewParser()
	parser.SetLanguage(handler.lang)

	tree, err := parser.ParseCtx(ctx, nil, []byte(file.Content))
	if err != nil {
		return c.chunkFallback(file, handler.name)
	}
	defer tree.Close()

	root := tree.RootNode()
	content := []byte(file.Content)
	var chunks []Chunk

	c.collectSymbols(&chunks, file, handler, root, content)
	if len(chunks) == 0 {
		return c.chunkFallback(file, handler.name)
	}

	return chunks
}

func (c *Chunker) collectSymbols(chunks *[]Chunk, file indexer.FileInfo, handler languageHandler, node *sitter.Node, content []byte) {
	if node == nil {
		return
	}

	if isSymbolNode(node, handler.symbolTypes) {
		chunk := c.buildChunk(file, handler.name, node, content)
		*chunks = append(*chunks, chunk)
		return
	}

	for i := 0; i < int(node.ChildCount()); i++ {
		c.collectSymbols(chunks, file, handler, node.Child(i), content)
	}
}

func (c *Chunker) buildChunk(file indexer.FileInfo, lang string, node *sitter.Node, content []byte) Chunk {
	text := string(node.Content(content))
	lines := strings.Split(text, "\n")
	firstLine := text
	if len(lines) > 0 {
		firstLine = strings.TrimSpace(lines[0])
	}

	symbol := extractSymbolName(node, content)
	if symbol == "" {
		symbol = guessSymbolName(firstLine)
	}

	return Chunk{
		ID:        hashChunk(file.Path, text),
		ProjectID: file.ProjectID,
		Path:      file.Path,
		FileHash:  file.Hash,
		Language:  lang,
		Symbol:    symbol,
		Signature: firstLine,
		Content:   text,
		StartLine: int(node.StartPoint().Row) + 1,
		EndLine:   int(node.EndPoint().Row) + 1,
	}
}

func (c *Chunker) chunkMarkdown(file indexer.FileInfo) []Chunk {
	var chunks []Chunk
	re := regexp.MustCompile(`(?m)^#{1,6}\s+(.+)$`)
	matches := re.FindAllStringIndex(file.Content, -1)
	if len(matches) == 0 {
		return append(chunks, c.wholeFileChunk(file, "markdown"))
	}

	for i, m := range matches {
		start := m[0]
		end := len(file.Content)
		if i < len(matches)-1 {
			end = matches[i+1][0]
		}

		section := file.Content[start:end]
		header := strings.TrimSpace(file.Content[m[0]:m[1]])
		chunks = append(chunks, Chunk{
			ID:        hashChunk(file.Path, section),
			ProjectID: file.ProjectID,
			Path:      file.Path,
			FileHash:  file.Hash,
			Language:  "markdown",
			Symbol:    header,
			Signature: header,
			Content:   section,
			StartLine: strings.Count(file.Content[:start], "\n") + 1,
			EndLine:   strings.Count(file.Content[:end], "\n") + 1,
		})
	}

	return chunks
}

func (c *Chunker) chunkFallback(file indexer.FileInfo, lang string) []Chunk {
	return []Chunk{c.wholeFileChunk(file, lang)}
}

func (c *Chunker) wholeFileChunk(file indexer.FileInfo, lang string) Chunk {
	lines := strings.Split(file.Content, "\n")
	return Chunk{
		ID:        hashChunk(file.Path, file.Content),
		ProjectID: file.ProjectID,
		Path:      file.Path,
		FileHash:  file.Hash,
		Language:  lang,
		Symbol:    filepath.Base(file.Path),
		Signature: filepath.Base(file.Path),
		Content:   file.Content,
		StartLine: 1,
		EndLine:   len(lines),
	}
}

func detectLanguage(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".go":
		return "go"
	case ".py":
		return "python"
	case ".js", ".mjs", ".cjs":
		return "javascript"
	case ".ts", ".tsx":
		return "typescript"
	case ".jsx":
		return "javascript"
	case ".rs":
		return "rust"
	case ".md", ".markdown":
		return "markdown"
	default:
		return ""
	}
}

func isSymbolNode(node *sitter.Node, types []string) bool {
	for _, t := range types {
		if node.Type() == t {
			return true
		}
	}
	return false
}

func extractSymbolName(node *sitter.Node, content []byte) string {
	nameNode := node.ChildByFieldName("name")
	if nameNode != nil {
		return string(nameNode.Content(content))
	}

	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child == nil {
			continue
		}
		t := child.Type()
		if t == "identifier" || t == "field_identifier" || t == "type_identifier" {
			return string(child.Content(content))
		}
	}

	return ""
}

func guessSymbolName(line string) string {
	re := regexp.MustCompile(`(?:func|def|fn|class|struct|enum|trait|impl|type|const|var)\s+(\w+)`)
	matches := re.FindStringSubmatch(line)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

func hashChunk(path, content string) string {
	sum := sha256.Sum256([]byte(path + "\n" + content))
	return fmt.Sprintf("%x", sum)
}

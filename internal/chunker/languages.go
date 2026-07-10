package chunker

import (
	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/golang"
	"github.com/smacker/go-tree-sitter/javascript"
	"github.com/smacker/go-tree-sitter/python"
	"github.com/smacker/go-tree-sitter/rust"
)

// languageHandler bundles a tree-sitter language and the node types considered
// chunk boundaries for that language.
type languageHandler struct {
	name        string
	lang        *sitter.Language
	symbolTypes []string
}

// buildHandlers returns the supported language parsers.
func buildHandlers() map[string]languageHandler {
	return map[string]languageHandler{
		"go": {
			name: "go",
			lang: golang.GetLanguage(),
			symbolTypes: []string{
				"function_declaration",
				"method_declaration",
				"type_declaration",
			},
		},
		"python": {
			name: "python",
			lang: python.GetLanguage(),
			symbolTypes: []string{
				"function_definition",
				"class_definition",
			},
		},
		"javascript": {
			name: "javascript",
			lang: javascript.GetLanguage(),
			symbolTypes: []string{
				"function_declaration",
				"class_declaration",
				"method_definition",
			},
		},
		"typescript": {
			name: "typescript",
			lang: javascript.GetLanguage(),
			symbolTypes: []string{
				"function_declaration",
				"class_declaration",
				"method_definition",
				"export_statement",
			},
		},
		"rust": {
			name: "rust",
			lang: rust.GetLanguage(),
			symbolTypes: []string{
				"function_item",
				"impl_item",
				"trait_item",
				"struct_item",
				"enum_item",
			},
		},
	}
}

package cursor

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/quonaro/gnostis/internal/chat_providers"
)

// Provider implements chat_providers.Provider for Cursor conversations.
// It is currently a placeholder; Cursor stores history in a different format
// and encryption than Windsurf Cascade, so decryption logic is not yet
// implemented.
type Provider struct{}

// NewProvider creates a Cursor provider placeholder.
func NewProvider() *Provider {
	return &Provider{}
}

// Name returns the provider identifier.
func (Provider) Name() string { return "cursor" }

// Discover returns the default Cursor history directory if it exists.
func (Provider) Discover() []string {
	home := os.Getenv("HOME")
	if home == "" {
		return nil
	}
	cursorDir := filepath.Join(home, ".cursor")
	if info, err := os.Stat(cursorDir); err != nil || !info.IsDir() {
		return nil
	}
	return []string{cursorDir}
}

// Decrypt is not yet implemented for Cursor.
func (Provider) Decrypt(path string) ([]byte, error) {
	return nil, fmt.Errorf("cursor decryption not implemented: %s", path)
}

// ExtractDialogue is not yet implemented for Cursor.
func (Provider) ExtractDialogue(data []byte) []chat_providers.Turn {
	return nil
}

// ExtractStrings is not yet implemented for Cursor.
func (Provider) ExtractStrings(data []byte) []string {
	return nil
}

// Compile-time interface check.
var _ chat_providers.Provider = Provider{}

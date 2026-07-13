package cascade

import (
	"github.com/quonaro/gnostis/internal/chat_providers"
)

// Provider implements chat_providers.Provider for Windsurf Cascade trajectories.
type Provider struct{}

// NewProvider creates a Cascade provider.
func NewProvider() *Provider {
	return &Provider{}
}

// Name returns the provider identifier.
func (Provider) Name() string { return "cascade" }

// Discover returns the known Cascade trajectory directories that exist.
func (Provider) Discover() []string { return SourceDirs() }

// Decrypt reads and decrypts a single .pb file.
func (Provider) Decrypt(path string) ([]byte, error) { return DecryptFile(path) }

// ExtractDialogue extracts user/assistant turns from decrypted protobuf data.
func (Provider) ExtractDialogue(data []byte) []chat_providers.Turn { return ExtractDialogue(data) }

// ExtractStrings returns all printable strings from decrypted protobuf data.
func (Provider) ExtractStrings(data []byte) []string { return ExtractStrings(data) }

// Compile-time interface check.
var _ chat_providers.Provider = Provider{}

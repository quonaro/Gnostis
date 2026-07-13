package chat_providers

// Turn represents a single user or assistant turn in a chat trajectory.
type Turn struct {
	Role    string
	Content string
}

// Provider abstracts a local chat client that stores conversation history.
// Implementations are provided per IDE (e.g. Windsurf Cascade, Cursor).
type Provider interface {
	// Name returns the provider identifier, e.g. "cascade" or "cursor".
	Name() string

	// Discover returns source directories that may contain conversation files.
	Discover() []string

	// Decrypt reads a conversation file and returns its plaintext payload.
	Decrypt(path string) ([]byte, error)

	// ExtractDialogue extracts user/assistant turns from a decrypted payload.
	ExtractDialogue(data []byte) []Turn

	// ExtractStrings returns all human-readable strings from a decrypted payload.
	ExtractStrings(data []byte) []string
}

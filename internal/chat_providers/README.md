# Chat Providers

This package abstracts local chat clients (Windsurf Cascade, Cursor, etc.) so
Gnostis can decrypt their conversation history and index it as Markdown.

## Structure

- `provider.go` — shared `Provider` interface and `Turn` type.
- `export.go` — generic Markdown exporter that works with any `Provider`.
- `all/registry.go` — registry that imports all built-in providers.
- `cascade/` — Windsurf Cascade implementation (AES-256-GCM protobuf).
- `cursor/` — placeholder for Cursor support.

## Adding a new provider

1. Create a new subdirectory, e.g. `internal/chat_providers/cursor`.
2. Implement the `chat_providers.Provider` interface.
3. Register it in `internal/chat_providers/all/registry.go`.

The `Provider` interface requires:

- `Name() string` — provider identifier.
- `Discover() []string` — source directories.
- `Decrypt(path string) ([]byte, error)` — decrypt a conversation file.
- `ExtractDialogue(data []byte) []chat_providers.Turn` — user/assistant turns.
- `ExtractStrings(data []byte) []string` — raw strings.

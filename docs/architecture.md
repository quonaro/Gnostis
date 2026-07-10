# Architecture

## Overview

Gnostis is a single Go binary that runs as a background MCP server. It indexes configured directories and exposes semantic search tools to AI editors.

## Components

```text
Cursor/Windsurf
       │
       │ stdio (MCP)
       ▼
┌──────────────┐
│  MCP Server  │
│  internal/mcp│
└──────┬───────┘
       │
       ▼
┌──────────────┐
│    Search    │
│ internal/    │
│   search     │
└──────┬───────┘
       │
   ┌───┴───┐
   ▼       ▼
┌──────┐ ┌──────┐
│ Store│ │Chunker│
│chromem│ │tree-  │
│ -go  │ │sitter │
└──┬───┘ └───┬───┘
   │         │
   ▼         ▼
┌──────┐ ┌────────┐
│ Disk │ │Embeddings│
│      │ │Provider │
└──────┘ └────┬───┘
              │
        ┌─────┴─────┐
        ▼           ▼
    ┌───────┐   ┌────────┐
    │Ollama │   │ OpenAI │
    │(local)│   │compatible
    └───────┘   └────────┘
```

## Data flow

### Indexing

1. `watcher` detects file changes or `indexer` is invoked manually.
2. `indexer` walks the directory, applies `.gitignore` and per-directory filters.
3. `chunker` parses accepted files and extracts symbol-level chunks.
4. `embeddings` provider converts chunks to vectors.
5. `store` persists chunks with metadata.

### Search

1. `mcp` receives a tool call.
2. `search` embeds the query and queries the vector store.
3. Results are reranked by vector score and keyword match.
4. `mcp` returns structured JSON to the editor.

## Directory layout

See the project root. Key packages:

- `internal/config` — YAML loading and env interpolation.
- `internal/directory` — per-directory indexing rules.
- `internal/indexer` — file walking and filtering.
- `internal/chunker` — tree-sitter symbol extraction.
- `internal/embeddings` — provider interface and implementations.
- `internal/store` — chromem-go persistence layer.
- `internal/search` — search orchestration and reranking.
- `internal/mcp` — MCP server and tools.
- `internal/watcher` — fsnotify debounced watcher.

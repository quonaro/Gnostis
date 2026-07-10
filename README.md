# Gnostis

A local "second brain" for developers. Gnostis indexes your projects with tree-sitter-aware chunking, stores embeddings locally, and exposes semantic search tools to AI agents through the Model Context Protocol (MCP).

## What it does

- Watches configured directories and incrementally indexes changed files.
- Splits code into symbol-level chunks (functions, types, methods, classes).
- Stores embeddings locally using `chromem-go`.
- Supports Ollama, OpenAI-compatible APIs, and **local ONNX embedding models** (no external server required after the first model download).
- Maintains a dedicated symbol index for fast exact symbol lookups.
- Answers semantic search queries from Cursor/Windsurf via MCP tools including `grep`, `list_files`, `directory_tree`, `get_recent_changes`, and `query_documentation`.

## Quick links

- [Architecture](docs/architecture.md)
- [Configuration](docs/config.md)
- [Embedding providers](docs/embedding-providers.md)
- [Indexing](docs/indexing.md)
- [MCP tools](docs/mcp-tools.md)

## CLI

Gnostis embeds the Lota task runner. Run `gnostis` without arguments to see help.

```bash
gnostis run                        # start the server (old default behavior)
gnostis version                    # show build version
gnostis index status               # list projects and chunk count
gnostis index rebuild              # delete the index and rebuild it
gnostis config validate            # validate config.yaml
gnostis config show                # print config with secrets masked
gnostis config discover /path      # add first-level directories as projects
gnostis config discover /path --git --backup  # only git repos, with backup
```

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

## Quick start

Gnostis runs as an HTTP-only MCP server managed by a systemd user unit.

```bash
lota build       # build ./gnostis with the short git commit hash as version
lota install     # install ~/.local/bin/gnostis, ~/.gnostis/config.yaml and the systemd user unit
```

The daemon listens on `http://127.0.0.1:8080/mcp` (override with `GNOSTIS_PORT`).

```bash
systemctl --user status gnostis    # check daemon status
systemctl --user stop gnostis      # stop the daemon
systemctl --user restart gnostis   # restart the daemon
gnostis status                     # show index and daemon status
gnostis rebuild                    # stop daemon, rebuild the index, and start it again
```

## CLI

Gnostis embeds the Lota task runner. Run `gnostis` without arguments to see help.

```bash
gnostis run                        # start the HTTP MCP server in the foreground
gnostis status                     # list projects, chunk count, and daemon status
gnostis rebuild                    # delete the index and rebuild it
gnostis validate                   # validate config.yaml
gnostis show                       # print config with secrets masked
gnostis discover /path             # add first-level directories as projects
gnostis discover /path --git --backup  # only git repos, with backup
```

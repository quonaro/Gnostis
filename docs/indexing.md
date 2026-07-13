# Indexing

## How it works

Gnostis keeps a persistent index of configured directories. Each directory is treated as a project.

## Initial indexing

When Gnostis starts, it scans every configured directory, filters files, chunks symbols, computes embeddings, and stores them.

## Filters

For each file the indexer applies filters in this order:

1. **`.gitignore`**: parsed per directory.
2. **`include`**: only files matching these globs are kept (if defined).
3. **`exclude`**: files matching these globs are skipped.
4. **`extensions`**: file extension must be in the allow-list.
5. **`max_file_size_mb`**: files larger than the limit are skipped.

## Chunking

Files are parsed with tree-sitter and split into symbol-level chunks:

- Functions and methods
- Types, structs, interfaces, classes
- Exported constants and variables
- Markdown sections

Each chunk stores:

- `id`: deterministic hash
- `project_id`: owning project
- `path`: absolute file path
- `language`: detected language
- `symbol`, `signature`, `docstring`
- `content`: full chunk text
- `start_line`, `end_line`

## Incremental updates

The `watcher` monitors configured directories with `fsnotify`. Changed files are debounced and reindexed individually:

1. Delete existing chunks for the file from the store.
2. Re-chunk the file.
3. Embed and insert new chunks.

Files matching filters are ignored by the watcher.

## Rebuilding

A full rebuild deletes the data directory and re-indexes all projects. The daemon is stopped and restarted automatically by `gnostis rebuild`:

```bash
gnostis rebuild
```

`gnostis rebuild` refuses to run if another Gnostis process is active.

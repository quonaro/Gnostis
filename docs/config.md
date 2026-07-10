# Configuration

Gnostis reads its configuration on startup in this order:

1. A path explicitly set via the `-config` command-line flag.
2. A path explicitly set via the `GNOSTIS_CONFIG` environment variable.
3. `config.yaml` located in the same directory as the running binary.
4. `config.yaml` in the current working directory.
5. `~/.gnostis/config.yaml` as a fallback.

Environment variables inside the file can be interpolated with `${VAR}` or `${VAR:-default}`.

## Example

```yaml
log_level: ${GNOSTIS_LOG_LEVEL:-info}

data_dir: ${HOME}/.gnostis/data

embeddings:
  provider: ollama
  url: ${OLLAMA_URL:-http://localhost:11434/v1}
  model: ${EMBEDDING_MODEL:-nomic-embed-text}
  api_key: ${OPENAI_API_KEY:-}
  batch_size: 32

index:
  default_extensions: [".go", ".py", ".js", ".ts", ".jsx", ".tsx", ".rs", ".md"]
  default_exclude_patterns:
    [
      "node_modules/**",
      ".git/**",
      "vendor/**",
      "dist/**",
      "build/**",
      "__pycache__/**",
    ]

directories:
  - path: ${HOME}/projects/myapp
    name: myapp
    extensions: [".go", ".md"]
    exclude:
      - "**/vendor/**"
      - "**/*_test.go"
      - "docs/legacy/**"
    include:
      - "src/**"
      - "pkg/**"
    max_file_size_mb: 10

  - path: ${HOME}/projects/shared-lib
    name: shared-lib
    exclude:
      - "**/__pycache__/**"
      - "**/*.pyc"

mcp:
  name: gnostis
  version: "0.1.0"
  transport: sse
  address: "127.0.0.1:8080"
```

## Fields

### `log_level`

Log level for the application. One of: `debug`, `info`, `warn`, `error`. Default: `info`.
Set to `debug` to see detailed embedding request logs and model activity.

### `data_dir`

Persistent data directory for the vector store and metadata. Default: `~/.gnostis/data`.

### `embeddings`

- `provider`: `ollama` or `openai`.
- `url`: endpoint for HTTP providers.
- `model`: model name.
- `api_key`: optional, used by `openai` provider.
- `batch_size`: max texts per embedding request.

### `index`

- `default_extensions`: allowed file extensions for all directories unless overridden.
- `default_exclude_patterns`: excluded globs for all directories unless overridden.

### `directories`

List of indexing roots. Each entry supports:

- `path` (required): absolute directory path.
- `name`: project name; inferred from directory name if omitted.
- `extensions`: overrides `index.default_extensions`.
- `include`: if set, only matching files are indexed.
- `exclude`: excluded globs; merged with defaults.
- `max_file_size_mb`: files larger than this are skipped.

### `mcp`

- `name`: server name.
- `version`: server version.
- `transport`: `stdio` or `sse`; `stdio` is recommended for editors, `sse` runs a background HTTP server.
- `address`: listen address for `sse` transport. Default: `:8080`.

## Filter precedence

1. `.gitignore`
2. `include`
3. `exclude`
4. `extensions`
5. `max_file_size_mb`

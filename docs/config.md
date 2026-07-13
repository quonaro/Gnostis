# Configuration

Gnostis reads `~/.gnostis/config.yaml`. Data is always stored in `~/.gnostis/data` and logs in `~/.gnostis/gnostis.log`.

The only environment variable that controls startup behavior is `GNOSTIS_PORT`, which overrides the default HTTP port `8080`.

Environment variables inside the file can still be interpolated with `${VAR}` or `${VAR:-default}`.

## Example

```yaml
log_level: info

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
  address: "127.0.0.1:8080"
  token: ""
```

## Fields

### `log_level`

Log level for the application. One of: `debug`, `info`, `warn`, `error`. Default: `info`.
Set to `debug` to see detailed embedding request logs and model activity.

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

Gnostis only supports the `streamable-http` MCP transport. The endpoint is `/mcp`.

- `name`: server name. Default: `gnostis`.
- `version`: server version. Default: short git commit hash at build time.
- `address`: listen address. Default: `127.0.0.1:8080`, or `127.0.0.1:${GNOSTIS_PORT}`.
- `token`: optional Bearer token. When set, clients must send `Authorization: Bearer <token>`.

## Filter precedence

1. `.gitignore`
2. `include`
3. `exclude`
4. `extensions`
5. `max_file_size_mb`

## Discovering projects

The `gnostis discover <path>` command scans `<path>` and proposes adding every first-level subdirectory as a project.

```bash
gnostis discover /home/user/CascadeProjects/my
gnostis discover /home/user/CascadeProjects/my --git --backup
```

Flags:

- `--git` — only include directories that contain `.git`.
- `--go` — only include directories that contain `go.mod`.
- `--nm` — only include directories that contain `node_modules`.
- `--venv` — only include directories that contain `.venv`.
- `--backup` — create a numbered backup of `config.yaml` before writing.

When multiple flags are provided, a directory matching any of them is included. The command shows a preview and asks for `[Y/n]` confirmation before modifying the config. Already configured directories are shown as `already configured` and skipped.

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

  - path: ${HOME}/CascadeProjects
    name: my-workspaces
    auto: true
    depth: 3
    discover:
      git: true
      go: true
      node_modules: false
      venv: false
      workspace: true

mcp:
  name: gnostis
  address: "127.0.0.1:8080"
  token: ""

cascade:
  enabled: false
  data_dir: ${HOME}/.gnostis/data/dialogues
  source_dirs:
    - ${HOME}/.codeium/windsurf-next/cascade
  min_user_message_length: 10
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
- `auto`: when `true`, automatically discover projects under `path` using the `discover` rules. Default: `false`.
- `depth`: maximum recursion depth for auto-discovery. Default: `2`.
- `discover`: markers used to detect projects when `auto` is `true`:
  - `git`: directories containing `.git`. Default: `true`.
  - `go`: directories containing `go.mod`. Default: `false`.
  - `node_modules`: directories containing `node_modules`. Default: `false`.
  - `venv`: directories containing `.venv`. Default: `false`.
  - `workspace`: parse `.code-workspace` files and include their folders. Default: `true`.
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

### `cascade`

Opt-in indexing of Windsurf/Cascade/Devin Desktop conversation trajectories.

- `enabled`: when `true`, Gnostis decrypts `.pb` files, exports them as Markdown, and indexes them as the `cascade-dialogues` project. Default: `false`.
- `data_dir`: where exported Markdown files are stored. Default: `${HOME}/.gnostis/data/dialogues`.
- `source_dirs`: list of directories containing `.pb` files. Default: all existing `~/.codeium/{windsurf,windsurf-next,devin,devin-desktop}/cascade` directories.
- `min_user_message_length`: shortest user message to keep in the dialogue section. Default: `10`.

You can also export sessions manually without enabling auto-indexing:

```bash
gnostis decrypt-cascade
```

To export to a different directory, set the `OUTPUT_DIR` variable in your shell or use the configuration file.

## Filter precedence

1. `.gitignore`
2. `include`
3. `exclude`
4. `extensions`
5. `max_file_size_mb`

## Discovering projects

Auto-discovery is configured in `config.yaml` by setting `auto: true` on a directory. Gnostis scans the directory and adds matching projects on startup. Changes to `config.yaml` are detected at runtime and reload automatically.

Alternatively, use the `discover_projects` MCP tool to preview which projects would be added under a given path.

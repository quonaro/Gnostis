# MCP Tools

Gnostis exposes the following tools to AI agents. All filesystem paths are
restricted to the configured indexed projects.

## `search_codebase`

Semantic search over indexed code and documentation.

**Parameters:**

- `query` (string, required)
- `project` (string, optional)
- `path` (string, optional) — absolute path prefix to filter results.
- `language` (string, optional)
- `top_k` (int, optional, default 10)
- `include_content` (bool, optional, default true)

**Returns:** array of chunks with `project`, `path`, `symbol`, `signature`, `start_line`, `end_line`, `score`, and `content`.

## `find_symbol`

Find the definition of a named symbol. The tool first looks up the exact symbol
in the dedicated symbol index, then falls back to fuzzy matching, and finally
to semantic search.

**Parameters:**

- `name` (string, required)
- `project` (string, optional)
- `language` (string, optional)

**Returns:** matching symbol definitions.

## `get_file_context`

Read a specific file or a range of lines. Paths outside the indexed projects are
rejected.

**Parameters:**

- `path` (string, required)
- `start_line` (int, optional)
- `end_line` (int, optional)

**Returns:** file content fragment.

## `list_projects`

List all indexed projects.

**Parameters:** none.

**Returns:** array of `{name, path}`.

## `grep`

Search file contents by substring or regular expression.

**Parameters:**

- `query` (string, required)
- `project` (string, optional)
- `path` (string, optional, relative path within the project)
- `regex` (bool, optional, default false)
- `top_k` (int, optional, default 20)

At least one of `project` or `path` must be provided.

**Returns:** array of `{path, line, content}` matches.

## `list_files`

List files matching a glob pattern.

**Parameters:**

- `project` (string, optional)
- `path` (string, optional, relative path within the project)
- `pattern` (string, optional, default `*`)
- `include_dirs` (bool, optional, default false)

At least one of `project` or `path` must be provided. By default only files are returned; set `include_dirs` to true to include directories.

**Returns:** array of `{path}` entries.

## `directory_tree`

Return the directory tree up to a given depth.

**Parameters:**

- `project` (string, optional)
- `path` (string, optional, relative path within the project)
- `depth` (int, optional, default 3)

**Returns:** nested tree with `path`, `type`, and `children`.

## `get_recent_changes`

List files modified within the last N minutes.

**Parameters:**

- `project` (string, optional)
- `path` (string, optional, relative path within the project)
- `minutes` (int, optional, default 60)

**Returns:** array of `{path, mod_time}`.

## `query_documentation`

Semantic search restricted to Markdown and README files.

**Parameters:**

- `query` (string, required)
- `project` (string, optional)
- `top_k` (int, optional, default 10)

**Returns:** array of chunks.

## `reindex_files`

Reindex specific files so their latest content is searchable. Only paths inside the indexed projects are accepted.

**Parameters:**

- `paths` (array of strings, required) — absolute file paths to reindex.

**Returns:** object with `reindexed` array of paths.

## `get_index_status`

Return the current index status, project list, provider/model, and progress.

**Parameters:** none.

**Returns:** object with `projects`, `total_chunks`, `provider`, `model`, `symbols`, `progress`, and `project_stats`.

## `get_index_job`

Return the status of a previously started rebuild job.

**Parameters:**

- `job_id` (string, required) — value returned by `rebuild_project` or `rebuild_index`.

**Returns:** progress state object.

## `rebuild_project`

Rebuild the index for a single project. The operation runs in the background.

**Parameters:**

- `project` (string, required)

**Returns:** object with `job_id`.

## `rebuild_index`

Rebuild the entire index. The operation runs in the background and may take a while.

**Parameters:** none.

**Returns:** object with `job_id`.

## `discover_projects`

Discover projects under a directory and show what would be added.

**Parameters:**

- `path` (string, required) — absolute directory path.
- `depth` (int, optional, default 3)
- `git` (bool, optional, default true)
- `go` (bool, optional, default false)
- `node_modules` (bool, optional, default false)
- `venv` (bool, optional, default false)
- `workspace` (bool, optional, default true)

**Returns:** object with `new` and `already_added` arrays.

## `add_project`

Add a directory to the index and write it to `config.yaml`.

**Parameters:**

- `path` (string, required) — absolute directory path.
- `name` (string, optional) — project name, defaults to directory name.

**Returns:** object with `added`.

## `remove_project`

Remove a project from the index and `config.yaml`.

**Parameters:**

- `name` (string, required)
- `confirm` (bool, required) — must be `true`.

**Returns:** object with `removed`.

## Planned tools

- `find_references`: locate all usages of a symbol.
- `find_related_code`: discover files related to a given symbol or file.

# MCP Tools

Gnostis exposes the following tools to AI agents. All filesystem paths are
restricted to the configured indexed projects.

## `search_codebase`

Semantic search over indexed code and documentation.

**Parameters:**

- `query` (string, required)
- `project` (string, optional)
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

**Returns:** array of `{path, line, content}` matches.

## `list_files`

List files matching a glob pattern.

**Parameters:**

- `project` (string, optional)
- `path` (string, optional, relative path within the project)
- `pattern` (string, optional, default `*`)

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

## Planned tools

- `find_references`: locate all usages of a symbol.
- `find_related_code`: discover files related to a given symbol or file.

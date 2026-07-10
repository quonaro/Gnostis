# MCP Tools

Gnostis exposes the following tools to AI agents.

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

Find the definition of a named symbol.

**Parameters:**

- `name` (string, required)
- `project` (string, optional)
- `language` (string, optional)

**Returns:** matching symbol definitions.

## `get_file_context`

Read a specific file or a range of lines.

**Parameters:**

- `path` (string, required)
- `start_line` (int, optional)
- `end_line` (int, optional)

**Returns:** file content fragment with metadata.

## `list_projects`

List all indexed projects.

**Parameters:** none.

**Returns:** array of `{name, path, status, chunks}`.

## Planned tools

- `find_references`: locate all usages of a symbol.
- `query_documentation`: search only Markdown and README files.
- `get_recent_changes`: files modified recently.
- `find_related_code`: discover files related to a given symbol or file.

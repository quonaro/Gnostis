package mcp

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/mark3labs/mcp-go/mcp"
	mcpServer "github.com/mark3labs/mcp-go/server"

	"github.com/quonaro/gnostis/internal/project"
	"github.com/quonaro/gnostis/internal/search"
)

// Searcher is the subset of the search engine used by MCP tools.
type Searcher interface {
	Search(ctx context.Context, query string, filters map[string]string, topK int) ([]search.Result, error)
}

// Server wraps the mcp-go server and exposes Gnostis tools.
type Server struct {
	server   *mcpServer.MCPServer
	name     string
	version  string
	engine   Searcher
	projects []project.Project
}

// New creates and configures the MCP server.
func New(name, version string, engine Searcher, projects []project.Project) *Server {
	slog.Info("creating mcp server", "name", name, "version", version)
	s := &Server{
		name:     name,
		version:  version,
		engine:   engine,
		projects: projects,
	}

	s.server = mcpServer.NewMCPServer(
		name,
		version,
		mcpServer.WithToolCapabilities(false),
	)
	s.registerTools()

	return s
}

// Start runs the stdio MCP server until the process exits.
func (s *Server) Start(ctx context.Context) error {
	slog.InfoContext(ctx, "starting mcp server", "name", s.name, "version", s.version)
	if err := mcpServer.ServeStdio(s.server); err != nil {
		return fmt.Errorf("serve stdio: %w", err)
	}
	return nil
}

func (s *Server) registerTools() {
	slog.Info("registering mcp tools")
	s.server.AddTool(searchCodebaseTool(), mcp.NewTypedToolHandler(s.searchCodebase))
	s.server.AddTool(findSymbolTool(), mcp.NewTypedToolHandler(s.findSymbol))
	s.server.AddTool(getFileContextTool(), mcp.NewTypedToolHandler(s.getFileContext))
	s.server.AddTool(listProjectsTool(), mcp.NewTypedToolHandler(s.listProjects))
}

func searchCodebaseTool() mcp.Tool {
	return mcp.NewTool("search_codebase",
		mcp.WithDescription("Semantic search over indexed code and documentation"),
		mcp.WithString("query", mcp.Required(), mcp.Description("Natural language search query")),
		mcp.WithString("project", mcp.Description("Project name to restrict the search")),
		mcp.WithString("language", mcp.Description("Language filter, e.g. go, python, markdown")),
		mcp.WithNumber("top_k", mcp.Description("Number of results"), mcp.DefaultNumber(10)),
		mcp.WithBoolean("include_content", mcp.Description("Include full chunk text"), mcp.DefaultBool(true)),
	)
}

func findSymbolTool() mcp.Tool {
	return mcp.NewTool("find_symbol",
		mcp.WithDescription("Find the definition of a named symbol"),
		mcp.WithString("name", mcp.Required(), mcp.Description("Symbol name")),
		mcp.WithString("project", mcp.Description("Project name")),
		mcp.WithString("language", mcp.Description("Language filter")),
	)
}

func getFileContextTool() mcp.Tool {
	return mcp.NewTool("get_file_context",
		mcp.WithDescription("Read a file or a range of lines"),
		mcp.WithString("path", mcp.Required(), mcp.Description("Absolute file path")),
		mcp.WithNumber("start_line", mcp.Description("First line (1-based)")),
		mcp.WithNumber("end_line", mcp.Description("Last line (1-based)")),
	)
}

func listProjectsTool() mcp.Tool {
	return mcp.NewTool("list_projects",
		mcp.WithDescription("List all indexed projects"),
	)
}

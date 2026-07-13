package mcp

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"

	"github.com/mark3labs/mcp-go/mcp"
	mcpServer "github.com/mark3labs/mcp-go/server"

	"github.com/quonaro/gnostis/internal/discover"
	"github.com/quonaro/gnostis/internal/memory"
	"github.com/quonaro/gnostis/internal/progress"
	"github.com/quonaro/gnostis/internal/project"
	"github.com/quonaro/gnostis/internal/search"
	"github.com/quonaro/gnostis/internal/stats"
	"github.com/quonaro/gnostis/internal/symbol"
)

// Searcher is the subset of the search engine used by MCP tools.
type Searcher interface {
	Search(ctx context.Context, query string, filters map[string]string, topK int) ([]search.Result, error)
}

// Finder is the subset of the symbol index used by MCP tools.
type Finder interface {
	Lookup(name string) []symbol.Location
	SearchFuzzy(query string) []symbol.Location
}

// Indexer exposes the operations MCP tools can perform on the index.
type Indexer interface {
	Status() ([]string, int)
	Info() (provider, model string, symbols int)
	ProgressState() (progress.State, error)
	ProjectStats(ctx context.Context) (map[string]stats.Project, error)
	ReindexFiles(ctx context.Context, paths []string) error
	StartRebuildProject(ctx context.Context, name string) (string, error)
	StartRebuildIndex(ctx context.Context) (string, error)
	DiscoverProjects(ctx context.Context, root string, opts discover.Options) (discover.Result, error)
	AddProject(ctx context.Context, path, name string) error
	RemoveProject(ctx context.Context, name string) error
}

// Server wraps the mcp-go server and exposes Gnostis tools.
type Server struct {
	mu            sync.RWMutex
	server        *mcpServer.MCPServer
	http          *mcpServer.StreamableHTTPServer
	name          string
	version       string
	engine        Searcher
	symbols       Finder
	indexer       Indexer
	memoryManager *memory.Manager
	projects      []project.Project
}

// New creates and configures the MCP server.
func New(name, version string, engine Searcher, symbols Finder, indexer Indexer, memoryManager *memory.Manager, projects []project.Project) *Server {
	slog.Info("creating mcp server", "name", name, "version", version)
	s := &Server{
		name:          name,
		version:       version,
		engine:        engine,
		symbols:       symbols,
		indexer:       indexer,
		memoryManager: memoryManager,
		projects:      projects,
	}

	s.server = mcpServer.NewMCPServer(
		name,
		version,
		mcpServer.WithToolCapabilities(false),
	)
	s.registerTools()

	return s
}

// ReloadProjects updates the project list used for path resolution and list_projects.
func (s *Server) ReloadProjects(projects []project.Project) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.projects = projects
}

// ReloadMemoryManager replaces the memory manager used by memory tools.
func (s *Server) ReloadMemoryManager(mgr *memory.Manager) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.memoryManager = mgr
}

// StartHTTP runs the MCP server over Streamable HTTP on the given address.
// If token is non-empty, the Authorization header must be "Bearer <token>".
func (s *Server) StartHTTP(ctx context.Context, addr, token string) error {
	slog.InfoContext(ctx, "starting mcp streamable http server", "name", s.name, "version", s.version, "address", addr)

	// The handler forwards requests to the StreamableHTTPServer once it is created.
	var handler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.http.ServeHTTP(w, r)
	})
	if token != "" {
		handler = bearerTokenHandler(handler, token)
	}

	httpServer := &http.Server{Addr: addr, Handler: handler}
	s.http = mcpServer.NewStreamableHTTPServer(s.server, mcpServer.WithStreamableHTTPServer(httpServer))
	if err := s.http.Start(addr); err != nil {
		return fmt.Errorf("serve streamable http: %w", err)
	}
	return nil
}

func bearerTokenHandler(next http.Handler, token string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		const prefix = "Bearer "
		if !strings.HasPrefix(auth, prefix) || auth[len(prefix):] != token {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// StopHTTP gracefully shuts down the Streamable HTTP server.
func (s *Server) StopHTTP(ctx context.Context) error {
	if s.http == nil {
		return nil
	}
	return s.http.Shutdown(ctx)
}

func (s *Server) registerTools() {
	slog.Info("registering mcp tools")
	s.server.AddTool(searchCodebaseTool(), mcp.NewTypedToolHandler(s.searchCodebase))
	s.server.AddTool(findSymbolTool(), mcp.NewTypedToolHandler(s.findSymbol))
	s.server.AddTool(getFileContextTool(), mcp.NewTypedToolHandler(s.getFileContext))
	s.server.AddTool(listProjectsTool(), mcp.NewTypedToolHandler(s.listProjects))
	s.server.AddTool(grepTool(), mcp.NewTypedToolHandler(s.grep))
	s.server.AddTool(listFilesTool(), mcp.NewTypedToolHandler(s.listFiles))
	s.server.AddTool(directoryTreeTool(), mcp.NewTypedToolHandler(s.directoryTree))
	s.server.AddTool(getRecentChangesTool(), mcp.NewTypedToolHandler(s.getRecentChanges))
	s.server.AddTool(queryDocumentationTool(), mcp.NewTypedToolHandler(s.queryDocumentation))
	s.server.AddTool(reindexFilesTool(), mcp.NewTypedToolHandler(s.reindexFiles))
	s.server.AddTool(getIndexStatusTool(), mcp.NewTypedToolHandler(s.getIndexStatus))
	s.server.AddTool(getIndexJobTool(), mcp.NewTypedToolHandler(s.getIndexJob))
	s.server.AddTool(rebuildProjectTool(), mcp.NewTypedToolHandler(s.rebuildProject))
	s.server.AddTool(rebuildIndexTool(), mcp.NewTypedToolHandler(s.rebuildIndex))
	s.server.AddTool(discoverProjectsTool(), mcp.NewTypedToolHandler(s.discoverProjects))
	s.server.AddTool(addProjectTool(), mcp.NewTypedToolHandler(s.addProject))
	s.server.AddTool(removeProjectTool(), mcp.NewTypedToolHandler(s.removeProject))
	s.server.AddTool(memorySearchTool(), mcp.NewTypedToolHandler(s.memorySearch))
	s.server.AddTool(memoryWriteTool(), mcp.NewTypedToolHandler(s.memoryWrite))
	s.server.AddTool(memoryListTool(), mcp.NewTypedToolHandler(s.memoryList))
	s.server.AddTool(memoryReadTool(), mcp.NewTypedToolHandler(s.memoryRead))
}

func searchCodebaseTool() mcp.Tool {
	return mcp.NewTool("search_codebase",
		mcp.WithDescription("Semantic search over indexed code and documentation"),
		mcp.WithString("query", mcp.Required(), mcp.Description("Natural language search query")),
		mcp.WithString("project", mcp.Description("Project name to restrict the search")),
		mcp.WithString("path", mcp.Description("Absolute or project-relative path prefix to restrict the search")),
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

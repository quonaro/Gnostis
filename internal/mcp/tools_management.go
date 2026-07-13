package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/quonaro/gnostis/internal/discover"
	"github.com/quonaro/gnostis/internal/progress"
	"github.com/quonaro/gnostis/internal/stats"
)

type getIndexStatusArgs struct{}

type indexStatusResult struct {
	Projects     []string                 `json:"projects"`
	TotalChunks  int                      `json:"total_chunks"`
	Provider     string                   `json:"provider"`
	Model        string                   `json:"model"`
	Symbols      int                      `json:"symbols"`
	Progress     progress.State           `json:"progress"`
	ETA          string                   `json:"eta,omitempty"`
	ETASeconds   int64                    `json:"eta_seconds,omitempty"`
	ProjectStats map[string]stats.Project `json:"project_stats"`
}

func getIndexStatusTool() mcp.Tool {
	return mcp.NewTool("get_index_status",
		mcp.WithDescription("Return the current index status, project list, and progress"),
	)
}

func (s *Server) getIndexStatus(ctx context.Context, request mcp.CallToolRequest, args getIndexStatusArgs) (*mcp.CallToolResult, error) {
	slog.InfoContext(ctx, "mcp tool call", "tool", "get_index_status")
	if s.indexer == nil {
		return mcp.NewToolResultError("indexer is not configured"), nil
	}

	projects, chunks := s.indexer.Status()
	provider, model, symbols := s.indexer.Info()

	pstate, err := s.indexer.ProgressState()
	if err != nil {
		return nil, fmt.Errorf("load progress: %w", err)
	}

	pst, err := s.indexer.ProjectStats(ctx)
	if err != nil {
		return nil, fmt.Errorf("load project stats: %w", err)
	}

	eta := pstate.ETA()
	result := indexStatusResult{
		Projects:     projects,
		TotalChunks:  chunks,
		Provider:     provider,
		Model:        model,
		Symbols:      symbols,
		Progress:     pstate,
		ProjectStats: pst,
	}
	if eta > 0 {
		result.ETA = eta.String()
		result.ETASeconds = int64(eta.Seconds())
	}

	data, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("marshal status: %w", err)
	}
	return mcp.NewToolResultText(string(data)), nil
}

type getIndexJobArgs struct {
	JobID string `json:"job_id"`
}

func getIndexJobTool() mcp.Tool {
	return mcp.NewTool("get_index_job",
		mcp.WithDescription("Return the status of a previously started rebuild job"),
		mcp.WithString("job_id", mcp.Required(), mcp.Description("Job ID returned by rebuild_project or rebuild_index")),
	)
}

func (s *Server) getIndexJob(ctx context.Context, request mcp.CallToolRequest, args getIndexJobArgs) (*mcp.CallToolResult, error) {
	slog.InfoContext(ctx, "mcp tool call", "tool", "get_index_job", "job_id", args.JobID)
	if s.indexer == nil {
		return mcp.NewToolResultError("indexer is not configured"), nil
	}
	if args.JobID == "" {
		return mcp.NewToolResultError("job_id is required"), nil
	}

	pstate, err := s.indexer.ProgressState()
	if err != nil {
		return nil, fmt.Errorf("load progress: %w", err)
	}
	if pstate.JobID != args.JobID {
		return mcp.NewToolResultError(fmt.Sprintf("job %q not found", args.JobID)), nil
	}

	data, err := json.Marshal(pstate)
	if err != nil {
		return nil, fmt.Errorf("marshal job state: %w", err)
	}
	return mcp.NewToolResultText(string(data)), nil
}

type rebuildProjectArgs struct {
	Project string `json:"project"`
}

func rebuildProjectTool() mcp.Tool {
	return mcp.NewTool("rebuild_project",
		mcp.WithDescription("Rebuild the index for a single project"),
		mcp.WithString("project", mcp.Required(), mcp.Description("Project name")),
	)
}

func (s *Server) rebuildProject(ctx context.Context, request mcp.CallToolRequest, args rebuildProjectArgs) (*mcp.CallToolResult, error) {
	slog.InfoContext(ctx, "mcp tool call", "tool", "rebuild_project", "project", args.Project)
	if s.indexer == nil {
		return mcp.NewToolResultError("indexer is not configured"), nil
	}
	if args.Project == "" {
		return mcp.NewToolResultError("project is required"), nil
	}

	jobID, err := s.indexer.StartRebuildProject(ctx, args.Project)
	if err != nil {
		return nil, fmt.Errorf("start rebuild project: %w", err)
	}

	return mcp.NewToolResultText(fmt.Sprintf(`{"job_id":%q}`, jobID)), nil
}

type rebuildIndexArgs struct{}

func rebuildIndexTool() mcp.Tool {
	return mcp.NewTool("rebuild_index",
		mcp.WithDescription("Rebuild the entire index"),
	)
}

func (s *Server) rebuildIndex(ctx context.Context, request mcp.CallToolRequest, args rebuildIndexArgs) (*mcp.CallToolResult, error) {
	slog.InfoContext(ctx, "mcp tool call", "tool", "rebuild_index")
	if s.indexer == nil {
		return mcp.NewToolResultError("indexer is not configured"), nil
	}

	jobID, err := s.indexer.StartRebuildIndex(ctx)
	if err != nil {
		return nil, fmt.Errorf("start rebuild index: %w", err)
	}

	return mcp.NewToolResultText(fmt.Sprintf(`{"job_id":%q}`, jobID)), nil
}

type discoverProjectsArgs struct {
	Path        string `json:"path"`
	Depth       int    `json:"depth"`
	Git         bool   `json:"git"`
	Go          bool   `json:"go"`
	NodeModules bool   `json:"node_modules"`
	Venv        bool   `json:"venv"`
	Workspace   bool   `json:"workspace"`
}

func discoverProjectsTool() mcp.Tool {
	return mcp.NewTool("discover_projects",
		mcp.WithDescription("Discover projects under a directory and show what would be added"),
		mcp.WithString("path", mcp.Required(), mcp.Description("Absolute directory path to scan")),
		mcp.WithNumber("depth", mcp.Description("Maximum recursion depth"), mcp.DefaultNumber(3)),
		mcp.WithBoolean("git", mcp.Description("Detect .git repositories"), mcp.DefaultBool(true)),
		mcp.WithBoolean("go", mcp.Description("Detect go.mod directories"), mcp.DefaultBool(false)),
		mcp.WithBoolean("node_modules", mcp.Description("Detect node_modules directories"), mcp.DefaultBool(false)),
		mcp.WithBoolean("venv", mcp.Description("Detect .venv directories"), mcp.DefaultBool(false)),
		mcp.WithBoolean("workspace", mcp.Description("Detect .code-workspace files"), mcp.DefaultBool(true)),
	)
}

func (s *Server) discoverProjects(ctx context.Context, request mcp.CallToolRequest, args discoverProjectsArgs) (*mcp.CallToolResult, error) {
	slog.InfoContext(ctx, "mcp tool call", "tool", "discover_projects", "path", args.Path)
	if s.indexer == nil {
		return mcp.NewToolResultError("indexer is not configured"), nil
	}
	if args.Path == "" {
		return mcp.NewToolResultError("path is required"), nil
	}

	root, err := s.resolveAbsolutePath(args.Path)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		return mcp.NewToolResultError(fmt.Sprintf("%s is not a directory", root)), nil
	}

	opts := discover.Options{
		Git:         args.Git,
		Go:          args.Go,
		NodeModules: args.NodeModules,
		Venv:        args.Venv,
		Workspace:   args.Workspace,
		Depth:       args.Depth,
	}
	result, err := s.indexer.DiscoverProjects(ctx, root, opts)
	if err != nil {
		return nil, fmt.Errorf("discover projects: %w", err)
	}

	data, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("marshal discover result: %w", err)
	}
	return mcp.NewToolResultText(string(data)), nil
}

type addProjectArgs struct {
	Path string `json:"path"`
	Name string `json:"name"`
}

func addProjectTool() mcp.Tool {
	return mcp.NewTool("add_project",
		mcp.WithDescription("Add a directory to the index"),
		mcp.WithString("path", mcp.Required(), mcp.Description("Absolute directory path")),
		mcp.WithString("name", mcp.Description("Project name (defaults to directory name)")),
	)
}

func (s *Server) addProject(ctx context.Context, request mcp.CallToolRequest, args addProjectArgs) (*mcp.CallToolResult, error) {
	slog.InfoContext(ctx, "mcp tool call", "tool", "add_project", "path", args.Path, "name", args.Name)
	if s.indexer == nil {
		return mcp.NewToolResultError("indexer is not configured"), nil
	}
	if args.Path == "" {
		return mcp.NewToolResultError("path is required"), nil
	}

	if err := s.indexer.AddProject(ctx, args.Path, args.Name); err != nil {
		return nil, fmt.Errorf("add project: %w", err)
	}
	return mcp.NewToolResultText(`{"added":true}`), nil
}

type removeProjectArgs struct {
	Name    string `json:"name"`
	Confirm bool   `json:"confirm"`
}

func removeProjectTool() mcp.Tool {
	return mcp.NewTool("remove_project",
		mcp.WithDescription("Remove a project from the index and configuration"),
		mcp.WithString("name", mcp.Required(), mcp.Description("Project name")),
		mcp.WithBoolean("confirm", mcp.Description("Must be true to remove"), mcp.DefaultBool(false)),
	)
}

func (s *Server) removeProject(ctx context.Context, request mcp.CallToolRequest, args removeProjectArgs) (*mcp.CallToolResult, error) {
	slog.InfoContext(ctx, "mcp tool call", "tool", "remove_project", "name", args.Name)
	if s.indexer == nil {
		return mcp.NewToolResultError("indexer is not configured"), nil
	}
	if args.Name == "" {
		return mcp.NewToolResultError("name is required"), nil
	}
	if !args.Confirm {
		return mcp.NewToolResultError("confirm must be true to remove"), nil
	}

	if err := s.indexer.RemoveProject(ctx, args.Name); err != nil {
		return nil, fmt.Errorf("remove project: %w", err)
	}
	return mcp.NewToolResultText(`{"removed":true}`), nil
}

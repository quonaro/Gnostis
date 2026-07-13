package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/quonaro/gnostis/internal/config"
	"github.com/quonaro/gnostis/internal/directory"
	"github.com/quonaro/gnostis/internal/discover"
	"github.com/quonaro/gnostis/internal/project"
)

// resolveProjects expands config directories into concrete directory/project pairs.
// Auto-discovery is applied to directories marked with Auto.
// Explicit (non-auto) directories are processed first so auto-discovery can skip
// paths that are already configured.
func resolveProjects(cfg config.Config) ([]directory.Directory, []project.Project, error) {
	var dirs []directory.Directory
	var projects []project.Project
	usedNames := make(map[string]bool)
	usedPaths := make(map[string]bool)

	// Process explicit directories first.
	for _, d := range cfg.Directories {
		if d.Auto {
			continue
		}
		p := project.New(d.Name, d.Path)
		dirs = append(dirs, directory.FromConfig(cfg.Index, d))
		projects = append(projects, p)
		usedNames[d.Name] = true
		usedPaths[d.Path] = true
	}

	// Then expand auto roots, skipping paths already covered by explicit entries.
	for _, d := range cfg.Directories {
		if !d.Auto {
			continue
		}

		opts := discover.Options{
			Git:         d.Discover.Git,
			Go:          d.Discover.Go,
			NodeModules: d.Discover.NodeModules,
			Venv:        d.Discover.Venv,
			Workspace:   d.Discover.Workspace,
			Depth:       d.Depth,
		}
		found, err := discover.Expand(d.Path, opts)
		if err != nil {
			return nil, nil, fmt.Errorf("expand directory %s: %w", d.Path, err)
		}
		found = discover.UniqueNames(found, usedNames)
		for _, p := range found {
			if usedPaths[p.Path] {
				continue
			}
			child := config.Directory{
				Path:          p.Path,
				Name:          p.Name,
				Extensions:    d.Extensions,
				Include:       d.Include,
				Exclude:       d.Exclude,
				MaxFileSizeMB: d.MaxFileSizeMB,
			}
			dirs = append(dirs, directory.FromConfig(cfg.Index, child))
			projects = append(projects, project.New(p.Name, p.Path))
			usedNames[p.Name] = true
			usedPaths[p.Path] = true
		}
	}

	return dirs, projects, nil
}

// DiscoverProjects scans root and returns projects that are not already configured.
func (a *App) DiscoverProjects(ctx context.Context, root string, opts discover.Options) (discover.Result, error) {
	existing := make(map[string]bool)
	for _, p := range a.projects {
		existing[p.Path] = true
	}
	return discover.FindProjects(root, opts, existing)
}

// AddProject adds a new directory to the config and reloads the project list.
func (a *App) AddProject(ctx context.Context, path, name string) error {
	if path == "" {
		return fmt.Errorf("path is required")
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}
	info, err := os.Stat(absPath)
	if err != nil {
		return fmt.Errorf("stat %s: %w", absPath, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", absPath)
	}

	for _, p := range a.projects {
		if p.Path == absPath {
			return fmt.Errorf("project with path %q already exists", absPath)
		}
	}

	if name == "" {
		name = filepath.Base(absPath)
	}
	name = uniqueProjectName(name, a.projects)

	d := config.Directory{Path: absPath, Name: name}
	a.cfg.Directories = append(a.cfg.Directories, d)
	a.dirs = append(a.dirs, directory.FromConfig(a.cfg.Index, d))
	a.projects = append(a.projects, project.New(name, absPath))

	if err := a.saveConfig(); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	if a.mcp != nil {
		a.mcp.ReloadProjects(a.projects)
	}
	return nil
}

// RemoveProject removes a project by name and deletes its indexed chunks.
func (a *App) RemoveProject(ctx context.Context, name string) error {
	idx := -1
	for i, p := range a.projects {
		if p.Name == name {
			idx = i
			break
		}
	}
	if idx == -1 {
		return fmt.Errorf("project %q not found", name)
	}

	path := a.projects[idx].Path

	if err := a.deleteChunksByPrefix(ctx, path); err != nil {
		return fmt.Errorf("delete project chunks: %w", err)
	}

	if err := a.symbolIndex.Save(); err != nil {
		return fmt.Errorf("save symbol index: %w", err)
	}

	// Remove from runtime lists.
	a.dirs = append(a.dirs[:idx], a.dirs[idx+1:]...)
	a.projects = append(a.projects[:idx], a.projects[idx+1:]...)

	// Remove from config.
	cfgIdx := -1
	for i, d := range a.cfg.Directories {
		if d.Name == name {
			cfgIdx = i
			break
		}
	}
	if cfgIdx != -1 {
		a.cfg.Directories = append(a.cfg.Directories[:cfgIdx], a.cfg.Directories[cfgIdx+1:]...)
	}

	if err := a.saveConfig(); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	if a.mcp != nil {
		a.mcp.ReloadProjects(a.projects)
	}
	return nil
}

func uniqueProjectName(name string, projects []project.Project) string {
	used := make(map[string]bool)
	for _, p := range projects {
		used[p.Name] = true
	}
	if !used[name] {
		return name
	}
	for n := 1; ; n++ {
		candidate := fmt.Sprintf("%s-%d", name, n)
		if !used[candidate] {
			return candidate
		}
	}
}

func (a *App) saveConfig() error {
	path := a.ConfigPath
	if path == "" {
		resolved, err := config.ResolvePath("")
		if err != nil {
			return fmt.Errorf("resolve config path: %w", err)
		}
		path = resolved
	}

	data, err := yaml.Marshal(a.cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}

	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

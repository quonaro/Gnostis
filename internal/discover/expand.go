package discover

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Expand recursively scans root and returns all discovered projects up to opts.Depth.
// When opts.Depth is 0, it defaults to 1. If no discovery flags are set, every
// directory is considered a project.
func Expand(root string, opts Options) ([]Project, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve root path: %w", err)
	}

	info, err := os.Stat(absRoot)
	if err != nil {
		return nil, fmt.Errorf("stat root: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("root is not a directory: %s", absRoot)
	}

	if opts.Depth == 0 {
		opts.Depth = 1
	}

	noFilter := !opts.Git && !opts.Go && !opts.NodeModules && !opts.Venv && !opts.Workspace
	var projects []Project
	seen := make(map[string]bool)

	err = filepath.WalkDir(absRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		if shouldSkip(path) {
			return nil
		}
		if seen[path] {
			return fs.SkipDir
		}
		relDepth := depthFromRoot(absRoot, path)
		if relDepth > opts.Depth {
			return fs.SkipDir
		}

		workspaceFound := false
		if opts.Workspace {
			ws, werr := findWorkspace(path)
			if werr != nil {
				return nil
			}
			if ws != "" {
				workspaceFound = true
				wsProjects, werr := parseWorkspaceFile(ws, absRoot, seen)
				if werr != nil {
					return nil
				}
				projects = append(projects, wsProjects...)
			}
		}

		if noFilter {
			if relDepth > 0 {
				projects = append(projects, projectFromPath(absRoot, path, ""))
				seen[path] = true
				return fs.SkipDir
			}
			return nil
		}

		if workspaceFound {
			return nil
		}

		if relDepth == 0 {
			if matchesFilter(path, opts, false) {
				projects = append(projects, projectFromPath(absRoot, path, ""))
				seen[path] = true
				return fs.SkipDir
			}
			return nil
		}

		if matchesFilter(path, opts, false) {
			projects = append(projects, projectFromPath(absRoot, path, ""))
			seen[path] = true
			return fs.SkipDir
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("expand root: %w", err)
	}

	return projects, nil
}

func depthFromRoot(root, path string) int {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." {
		return 0
	}
	return strings.Count(rel, string(filepath.Separator)) + 1
}

func projectFromPath(root, path, name string) Project {
	if name == "" {
		rel, err := filepath.Rel(root, path)
		if err != nil || rel == "." {
			rel = filepath.Base(path)
		}
		name = strings.ReplaceAll(filepath.ToSlash(rel), "/", "-")
	}
	return Project{Path: path, Name: name}
}

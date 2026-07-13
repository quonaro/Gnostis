// Package discover scans a parent directory and suggests project entries for the config.
package discover

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Options controls which marker files trigger project detection.
type Options struct {
	Git         bool
	Go          bool
	NodeModules bool
	Venv        bool
	Workspace   bool
	Depth       int
}

// Project represents a discovered directory entry.
type Project struct {
	Path string
	Name string
}

// Result separates newly discovered projects from already configured ones.
type Result struct {
	New          []Project
	AlreadyAdded []Project
}

// marker describes a single project marker file/directory.
type marker struct {
	flag    string
	path    string
	enabled func(opts Options) bool
}

var markers = []marker{
	{flag: "git", path: ".git", enabled: func(o Options) bool { return o.Git }},
	{flag: "go", path: "go.mod", enabled: func(o Options) bool { return o.Go }},
	{flag: "nm", path: "node_modules", enabled: func(o Options) bool { return o.NodeModules }},
	{flag: "venv", path: ".venv", enabled: func(o Options) bool { return o.Venv }},
}

// FindProjects scans root and returns projects that should be added.
// If no marker flags are set, every immediate subdirectory is considered a project.
func FindProjects(root string, opts Options, existing map[string]bool) (Result, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return Result{}, fmt.Errorf("resolve root path: %w", err)
	}

	if opts.Depth == 0 {
		opts.Depth = 1
	}

	all, err := Expand(root, opts)
	if err != nil {
		return Result{}, fmt.Errorf("expand root: %w", err)
	}

	var result Result
	for _, p := range all {
		if p.Path == absRoot {
			continue
		}
		if existing[p.Path] {
			result.AlreadyAdded = append(result.AlreadyAdded, p)
		} else {
			result.New = append(result.New, p)
		}
	}

	sort.Slice(result.New, func(i, j int) bool { return result.New[i].Path < result.New[j].Path })
	sort.Slice(result.AlreadyAdded, func(i, j int) bool { return result.AlreadyAdded[i].Path < result.AlreadyAdded[j].Path })

	return result, nil
}

func shouldSkip(path string) bool {
	base := filepath.Base(path)
	skip := []string{".git", "node_modules", ".venv", "vendor", "dist", "build", "tmp"}
	for _, s := range skip {
		if base == s {
			return true
		}
	}
	return false
}

func matchesFilter(projectPath string, opts Options, noFilter bool) bool {
	if noFilter {
		return true
	}

	for _, m := range markers {
		if !m.enabled(opts) {
			continue
		}
		if _, err := os.Stat(filepath.Join(projectPath, m.path)); err == nil {
			return true
		}
	}

	return false
}

// UniqueNames rewrites project names so that no name collides with an existing set.
func UniqueNames(projects []Project, existing map[string]bool) []Project {
	used := make(map[string]bool)
	for name := range existing {
		used[name] = true
	}

	out := make([]Project, len(projects))
	for i, p := range projects {
		name := p.Name
		if !used[name] {
			used[name] = true
			out[i] = p
			continue
		}

		for n := 1; ; n++ {
			candidate := fmt.Sprintf("%s-%d", name, n)
			if !used[candidate] {
				used[candidate] = true
				out[i] = Project{Path: p.Path, Name: candidate}
				break
			}
		}
	}
	return out
}

// ToYAML returns a YAML snippet for the given projects as config.Directory entries.
func ToYAML(projects []Project) string {
	if len(projects) == 0 {
		return ""
	}

	var b strings.Builder
	for _, p := range projects {
		fmt.Fprintf(&b, "  - path: %s\n    name: %s\n", p.Path, p.Name)
	}
	return b.String()
}

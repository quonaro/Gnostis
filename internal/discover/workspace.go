package discover

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// workspaceFile describes the parts of a VS Code workspace file we care about.
type workspaceFile struct {
	Folders []workspaceFolder `json:"folders"`
}

type workspaceFolder struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// findWorkspace returns the first *.code-workspace file in dir, or an empty string.
func findWorkspace(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("read directory %s: %w", dir, err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if filepath.Ext(e.Name()) == ".code-workspace" {
			return filepath.Join(dir, e.Name()), nil
		}
	}
	return "", nil
}

// parseWorkspaceFile reads a workspace file and returns a project for each folder.
func parseWorkspaceFile(path, root string, seen map[string]bool) ([]Project, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read workspace file %s: %w", path, err)
	}

	var ws workspaceFile
	if err := json.Unmarshal(data, &ws); err != nil {
		return nil, fmt.Errorf("parse workspace file %s: %w", path, err)
	}

	dir := filepath.Dir(path)
	var projects []Project
	for _, f := range ws.Folders {
		if f.Path == "" {
			continue
		}
		folderPath := f.Path
		if !filepath.IsAbs(folderPath) {
			folderPath = filepath.Join(dir, folderPath)
		}
		folderPath = filepath.Clean(folderPath)
		if seen[folderPath] {
			continue
		}
		info, err := os.Stat(folderPath)
		if err != nil || !info.IsDir() {
			continue
		}
		seen[folderPath] = true
		projects = append(projects, projectFromPath(root, folderPath, ""))
	}

	return projects, nil
}

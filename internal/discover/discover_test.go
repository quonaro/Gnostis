package discover

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindProjectsNoFilter(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "alpha"))
	mustMkdir(t, filepath.Join(root, "beta"))

	result, err := FindProjects(root, Options{}, nil)
	if err != nil {
		t.Fatalf("FindProjects: %v", err)
	}

	if len(result.New) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(result.New))
	}

	if result.New[0].Name != "alpha" && result.New[1].Name != "alpha" {
		t.Fatalf("expected alpha project, got %v", result.New)
	}
}

func TestFindProjectsWithGitFilter(t *testing.T) {
	root := t.TempDir()
	gitProject := filepath.Join(root, "with-git")
	plainProject := filepath.Join(root, "plain")
	mustMkdir(t, gitProject)
	mustMkdir(t, plainProject)
	mustMkdir(t, filepath.Join(gitProject, ".git"))

	result, err := FindProjects(root, Options{Git: true}, nil)
	if err != nil {
		t.Fatalf("FindProjects: %v", err)
	}

	if len(result.New) != 1 || result.New[0].Name != "with-git" {
		t.Fatalf("expected only with-git project, got %v", result.New)
	}
}

func TestFindProjectsExisting(t *testing.T) {
	root := t.TempDir()
	projectPath := filepath.Join(root, "existing")
	mustMkdir(t, projectPath)

	existing := map[string]bool{projectPath: true}
	result, err := FindProjects(root, Options{}, existing)
	if err != nil {
		t.Fatalf("FindProjects: %v", err)
	}

	if len(result.New) != 0 {
		t.Fatalf("expected 0 new projects, got %d", len(result.New))
	}
	if len(result.AlreadyAdded) != 1 {
		t.Fatalf("expected 1 already added project, got %d", len(result.AlreadyAdded))
	}
}

func TestUniqueNames(t *testing.T) {
	projects := []Project{
		{Path: "/a/foo", Name: "foo"},
		{Path: "/b/foo", Name: "foo"},
		{Path: "/c/bar", Name: "bar"},
	}
	existing := map[string]bool{"bar": true}

	named := UniqueNames(projects, existing)

	if named[0].Name != "foo" {
		t.Errorf("expected first foo unchanged, got %s", named[0].Name)
	}
	if named[1].Name != "foo-1" {
		t.Errorf("expected second foo renamed to foo-1, got %s", named[1].Name)
	}
	if named[2].Name != "bar-1" {
		t.Errorf("expected bar renamed to bar-1, got %s", named[2].Name)
	}
}

func TestToYAML(t *testing.T) {
	projects := []Project{
		{Path: "/projects/foo", Name: "foo"},
		{Path: "/projects/bar", Name: "bar"},
	}

	yaml := ToYAML(projects)
	expected := "  - path: /projects/foo\n    name: foo\n  - path: /projects/bar\n    name: bar\n"
	if yaml != expected {
		t.Fatalf("YAML mismatch:\nexpected:\n%s\ngot:\n%s", expected, yaml)
	}
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

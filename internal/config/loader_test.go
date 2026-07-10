package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInterpolateEnv(t *testing.T) {
	t.Setenv("TEST_VAR", "world")

	cases := []struct {
		input    string
		expected string
	}{
		{"hello ${TEST_VAR}", "hello world"},
		{"${MISSING:-default}", "default"},
		{"${TEST_VAR:-fallback}", "world"},
		{"no vars", "no vars"},
	}

	for _, tc := range cases {
		got := interpolateEnv(tc.input)
		if got != tc.expected {
			t.Errorf("interpolateEnv(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

func TestLoadDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	data := `
directories:
  - path: ` + dir + `
`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if cfg.MCP.Name != "gnostis" {
		t.Errorf("default server name = %q, want gnostis", cfg.MCP.Name)
	}
	if cfg.Embeddings.Provider != "ollama" {
		t.Errorf("default provider = %q, want ollama", cfg.Embeddings.Provider)
	}
	if cfg.Embeddings.BatchSize != 32 {
		t.Errorf("default batch size = %d, want 32", cfg.Embeddings.BatchSize)
	}
}

func TestLoadDataDirEnv(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(envDataDir, dir)

	path := filepath.Join(dir, "config.yaml")
	data := `
data_dir: /should/be/overridden
directories:
  - path: ` + dir + `
`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.DataDir != dir {
		t.Errorf("data_dir = %q, want %q", cfg.DataDir, dir)
	}
}

func TestLoadValidation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	data := `directories: []`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if _, err := Load(path); err == nil {
		t.Fatal("expected error for empty directories")
	}
}

func TestLoadFromCwd(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	data := `
log_level: debug
directories:
  - path: ` + dir + `
`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Chdir(dir)

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("load from cwd: %v", err)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("log_level = %q, want debug", cfg.LogLevel)
	}
}

func TestResolvePath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("directories:\n  - path: "+dir+"\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	resolved, err := ResolvePath(path)
	if err != nil {
		t.Fatalf("ResolvePath: %v", err)
	}
	if resolved != path {
		t.Errorf("ResolvePath = %q, want %q", resolved, path)
	}
}

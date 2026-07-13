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
		got := InterpolateEnv(tc.input)
		if got != tc.expected {
			t.Errorf("interpolateEnv(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

func TestInterpolateTilde(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("user home dir: %v", err)
	}

	if got := InterpolateEnv("~"); got != home {
		t.Errorf("InterpolateEnv(~) = %q, want %q", got, home)
	}
	want := filepath.Join(home, "foo", "bar")
	if got := InterpolateEnv("~/foo/bar"); got != want {
		t.Errorf("InterpolateEnv(~/foo/bar) = %q, want %q", got, want)
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

func TestLoadPortEnv(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GNOSTIS_PORT", "9090")

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
	if cfg.MCP.Address != "127.0.0.1:9090" {
		t.Errorf("mcp.address = %q, want 127.0.0.1:9090", cfg.MCP.Address)
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

func TestLoadMemoryCascadeDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	cascadeDir := filepath.Join(dir, "cascade-src")
	if err := os.MkdirAll(cascadeDir, 0o755); err != nil {
		t.Fatalf("create cascade dir: %v", err)
	}

	data := `
data_dir: ` + dir + `
directories:
  - path: ` + dir + `
memory:
  cascade:
    enabled: true
    source_dirs:
      - ` + cascadeDir + `
`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if !cfg.Memory.Cascade.Enabled {
		t.Fatal("memory.cascade.enabled = false, want true")
	}
	if cfg.Memory.Cascade.MinUserMessageLength != defaultMinUserMessageLength {
		t.Errorf("memory.cascade.min_user_message_length = %d, want %d", cfg.Memory.Cascade.MinUserMessageLength, defaultMinUserMessageLength)
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

func TestResolveDefaultPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("user home dir: %v", err)
	}

	resolved, err := ResolvePath("")
	if err != nil {
		t.Fatalf("ResolvePath default: %v", err)
	}

	want := filepath.Join(home, ".gnostis", "config.yaml")
	if resolved != want {
		t.Errorf("ResolvePath default = %q, want %q", resolved, want)
	}
}

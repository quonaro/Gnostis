package config

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

var envPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)(?::-([^}]*))?\}`)

// ResolvePath returns the absolute config path that Load would use.
func ResolvePath(path string) (string, error) {
	if path == "" {
		path = resolveDefaultConfigPath()
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve absolute config path: %w", err)
	}
	return abs, nil
}

// Load reads, interpolates, parses, and validates the configuration file.
func Load(path string) (Config, error) {
	path, err := ResolvePath(path)
	if err != nil {
		return Config{}, fmt.Errorf("resolve config path: %w", err)
	}
	slog.Info("loading config", "path", path)

	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config file %s: %w", path, err)
	}

	interpolated := interpolateEnv(string(data))

	var cfg Config
	if err := yaml.Unmarshal([]byte(interpolated), &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}

	applyDefaults(&cfg)
	slog.Debug("applied config defaults", "data_dir", cfg.DataDir, "provider", cfg.Embeddings.Provider, "model", cfg.Embeddings.Model)
	if err := validate(&cfg); err != nil {
		slog.Error("config validation failed", "error", err)
		return Config{}, fmt.Errorf("validate config: %w", err)
	}

	return cfg, nil
}

func resolveDefaultConfigPath() string {
	return interpolateEnv(defaultConfigPath)
}

func interpolateEnv(input string) string {
	return envPattern.ReplaceAllStringFunc(input, func(match string) string {
		parts := envPattern.FindStringSubmatch(match)
		if len(parts) < 2 {
			return match
		}

		name := parts[1]
		value := os.Getenv(name)
		if value != "" {
			return value
		}

		if len(parts) == 3 && parts[2] != "" {
			return parts[2]
		}

		return ""
	})
}

func applyDefaults(cfg *Config) {
	if cfg.LogLevel == "" {
		cfg.LogLevel = defaultLogLevel
	}
	cfg.LogLevel = strings.ToLower(cfg.LogLevel)

	if cfg.DataDir == "" {
		cfg.DataDir = interpolateEnv(defaultDataDir)
	}
	cfg.DataDir = filepath.Clean(cfg.DataDir)

	if cfg.Embeddings.Provider == "" {
		cfg.Embeddings.Provider = defaultProvider
	}
	if cfg.Embeddings.URL == "" {
		cfg.Embeddings.URL = defaultURL
	}
	if cfg.Embeddings.Model == "" {
		cfg.Embeddings.Model = defaultModel
	}
	if cfg.Embeddings.BatchSize == 0 {
		cfg.Embeddings.BatchSize = defaultBatchSize
	}

	if len(cfg.Index.DefaultExtensions) == 0 {
		cfg.Index.DefaultExtensions = []string{
			".go", ".py", ".js", ".ts", ".jsx", ".tsx", ".rs", ".md",
		}
	}
	if len(cfg.Index.DefaultExcludePatterns) == 0 {
		cfg.Index.DefaultExcludePatterns = []string{
			"node_modules/**", ".git/**", "vendor/**", "dist/**", "build/**", "__pycache__/**",
		}
	}

	if cfg.MCP.Name == "" {
		cfg.MCP.Name = defaultServerName
	}
	if cfg.MCP.Version == "" {
		cfg.MCP.Version = defaultVersion
	}
	if cfg.MCP.Address == "" {
		cfg.MCP.Address = defaultAddress
		if port := os.Getenv("GNOSTIS_PORT"); port != "" {
			cfg.MCP.Address = fmt.Sprintf("127.0.0.1:%s", port)
		}
	}

	for i := range cfg.Directories {
		if cfg.Directories[i].Name == "" {
			cfg.Directories[i].Name = filepath.Base(cfg.Directories[i].Path)
		}
		if cfg.Directories[i].MaxFileSizeMB == 0 {
			cfg.Directories[i].MaxFileSizeMB = 5
		}
	}
}

func validate(cfg *Config) error {
	switch cfg.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("unsupported log_level: %s", cfg.LogLevel)
	}

	if len(cfg.Directories) == 0 {
		return fmt.Errorf("at least one directory must be configured")
	}

	provider := strings.ToLower(cfg.Embeddings.Provider)
	if provider != "ollama" && provider != "openai" {
		return fmt.Errorf("unsupported embeddings provider: %s", cfg.Embeddings.Provider)
	}

	if cfg.Embeddings.Model == "" {
		return fmt.Errorf("embeddings model is required")
	}

	if cfg.Embeddings.BatchSize <= 0 {
		return fmt.Errorf("embeddings batch_size must be positive")
	}

	for i, dir := range cfg.Directories {
		if dir.Path == "" {
			return fmt.Errorf("directory %d: path is required", i)
		}

		info, err := os.Stat(dir.Path)
		if err != nil {
			return fmt.Errorf("directory %s: %w", dir.Path, err)
		}
		if !info.IsDir() {
			return fmt.Errorf("directory %s is not a directory", dir.Path)
		}

		if dir.Name == "" {
			return fmt.Errorf("directory %s: name is required", dir.Path)
		}
	}

	return nil
}

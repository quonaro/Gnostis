package config

// Config holds the complete application configuration.
type Config struct {
	LogLevel    string      `yaml:"log_level"`
	DataDir     string      `yaml:"data_dir"`
	Embeddings  Embeddings  `yaml:"embeddings"`
	Index       Index       `yaml:"index"`
	Directories []Directory `yaml:"directories"`
	MCP         MCP         `yaml:"mcp"`
}

// Embeddings configures the embedding provider.
type Embeddings struct {
	Provider  string `yaml:"provider"`
	URL       string `yaml:"url"`
	Model     string `yaml:"model"`
	APIKey    string `yaml:"api_key"`
	BatchSize int    `yaml:"batch_size"`
}

// Index configures global indexing defaults.
type Index struct {
	DefaultExtensions      []string `yaml:"default_extensions"`
	DefaultExcludePatterns []string `yaml:"default_exclude_patterns"`
}

// Directory configures a single indexed root.
type Directory struct {
	Path          string   `yaml:"path"`
	Name          string   `yaml:"name"`
	Extensions    []string `yaml:"extensions"`
	Include       []string `yaml:"include"`
	Exclude       []string `yaml:"exclude"`
	MaxFileSizeMB int      `yaml:"max_file_size_mb"`
}

// MCP configures the Model Context Protocol server.
type MCP struct {
	Name      string `yaml:"name"`
	Version   string `yaml:"version"`
	Transport string `yaml:"transport"`
}

const (
	defaultLogLevel   = "info"
	defaultDataDir    = "${HOME}/.gnostis/data"
	defaultProvider   = "ollama"
	defaultURL        = "http://localhost:11434/v1"
	defaultModel      = "nomic-embed-text"
	defaultBatchSize  = 32
	defaultServerName = "gnostis"
	defaultVersion    = "0.1.0"
	defaultTransport  = "stdio"
)

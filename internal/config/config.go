package config

// Config holds the complete application configuration.
type Config struct {
	LogLevel    string `yaml:"log_level"`
	DataDir     string
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
// When Auto is true, the directory is expanded into subprojects by discovery.
type Directory struct {
	Path          string   `yaml:"path"`
	Name          string   `yaml:"name"`
	Extensions    []string `yaml:"extensions"`
	Include       []string `yaml:"include"`
	Exclude       []string `yaml:"exclude"`
	MaxFileSizeMB int      `yaml:"max_file_size_mb"`
	Auto          bool     `yaml:"auto"`
	Depth         int      `yaml:"depth"`
	Discover      Discover `yaml:"discover"`
}

// Discover controls which markers trigger auto project detection.
type Discover struct {
	Git         bool `yaml:"git"`
	Go          bool `yaml:"go"`
	NodeModules bool `yaml:"node_modules"`
	Venv        bool `yaml:"venv"`
	Workspace   bool `yaml:"workspace"`
}

// MCP configures the Model Context Protocol server.
type MCP struct {
	Name    string `yaml:"name"`
	Version string `yaml:"version"`
	Address string `yaml:"address"`
	Token   string `yaml:"token"`
}

const (
	defaultLogLevel   = "info"
	defaultDataDir    = "${HOME}/.gnostis/data"
	defaultConfigPath = "${HOME}/.gnostis/config.yaml"
	defaultProvider   = "ollama"
	defaultURL        = "http://localhost:11434/v1"
	defaultModel      = "nomic-embed-text"
	defaultBatchSize  = 32
	defaultServerName = "gnostis"
	defaultVersion    = ""
	defaultAddress    = "127.0.0.1:8080"
)

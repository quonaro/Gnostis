package memory

import (
	"github.com/quonaro/gnostis/internal/chat_providers"
	"github.com/quonaro/gnostis/internal/config"
)

// Provider wraps a chat_providers.Provider together with its memory config.
type Provider struct {
	chat_providers.Provider
	cfg config.ProviderConfig
}

// NewProvider creates a memory provider from a chat provider and its config.
func NewProvider(p chat_providers.Provider, cfg config.ProviderConfig) *Provider {
	return &Provider{Provider: p, cfg: cfg}
}

// Enabled reports whether this provider is configured to run.
func (p *Provider) Enabled() bool {
	return p.cfg.Enabled
}

// Config returns the provider configuration.
func (p *Provider) Config() config.ProviderConfig {
	return p.cfg
}

// SourceDirs returns the configured source directories for this provider.
// If explicit directories are configured, those are returned; otherwise the
// provider's built-in discovery is used.
func (p *Provider) SourceDirs() []string {
	if len(p.cfg.SourceDirs) > 0 {
		return p.cfg.SourceDirs
	}
	return p.Discover()
}

// MinUserMessageLength returns the configured minimum user message length.
func (p *Provider) MinUserMessageLength() int {
	return p.cfg.MinUserMessageLength
}

// Exporter returns a chat_providers.Exporter configured for this provider.
func (p *Provider) Exporter() chat_providers.Exporter {
	return chat_providers.Exporter{
		MinUserMessageLength: p.cfg.MinUserMessageLength,
	}
}

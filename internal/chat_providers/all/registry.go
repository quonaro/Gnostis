package all

import (
	"github.com/quonaro/gnostis/internal/chat_providers"
	"github.com/quonaro/gnostis/internal/chat_providers/cascade"
	"github.com/quonaro/gnostis/internal/chat_providers/cursor"
)

// Registry lists all built-in chat providers.
type Registry struct {
	providers []chat_providers.Provider
}

// NewRegistry creates a registry with all built-in providers.
func NewRegistry() *Registry {
	return &Registry{
		providers: []chat_providers.Provider{
			cascade.NewProvider(),
			cursor.NewProvider(),
		},
	}
}

// Providers returns all registered providers.
func (r *Registry) Providers() []chat_providers.Provider {
	out := make([]chat_providers.Provider, len(r.providers))
	copy(out, r.providers)
	return out
}

// ByName returns the provider with the given name, or nil if not found.
func (r *Registry) ByName(name string) chat_providers.Provider {
	for _, p := range r.providers {
		if p.Name() == name {
			return p
		}
	}
	return nil
}

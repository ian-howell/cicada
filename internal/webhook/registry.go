package webhook

import "os"

// Registry holds all registered forge providers.
// It is not safe for concurrent use; providers are registered once at startup.
type Registry struct {
	providers map[string]ForgeProvider
}

// NewRegistry creates an empty registry.
func NewRegistry() *Registry {
	return &Registry{providers: make(map[string]ForgeProvider)}
}

// NewRegistryFromEnv creates a registry and registers providers based on environment variables.
// GitHub is registered when CICADA_GITHUB_WEBHOOK_SECRET is set.
func NewRegistryFromEnv() *Registry {
	r := NewRegistry()
	if secret := os.Getenv("CICADA_GITHUB_WEBHOOK_SECRET"); secret != "" {
		r.Register(NewGitHubProvider(secret))
	}
	return r
}

// Register adds a provider to the registry.
func (r *Registry) Register(p ForgeProvider) {
	r.providers[p.Name()] = p
}

// Get retrieves a provider by name.
func (r *Registry) Get(name string) (ForgeProvider, bool) {
	p, ok := r.providers[name]
	return p, ok
}

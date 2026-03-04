package providers

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/basmulder03/git-project-sync/internal/core/config"
)

type SourceRegistry struct {
	sources map[string]config.SourceConfig
}

func NewSourceRegistry(existing []config.SourceConfig) (*SourceRegistry, error) {
	registry := &SourceRegistry{sources: map[string]config.SourceConfig{}}
	for _, source := range existing {
		if err := registry.Add(source); err != nil {
			return nil, err
		}
	}
	return registry, nil
}

func (r *SourceRegistry) Add(source config.SourceConfig) error {
	if source.ID == "" {
		return errors.New("source id is required")
	}

	source.Provider = normalizeProvider(source.Provider)
	if source.Provider != "github" && source.Provider != "azuredevops" {
		return fmt.Errorf("unsupported source provider %q", source.Provider)
	}

	if source.Account == "" {
		return errors.New("source account is required")
	}

	if _, exists := r.sources[source.ID]; exists {
		return fmt.Errorf("source %q already exists", source.ID)
	}

	r.sources[source.ID] = source
	return nil
}

func (r *SourceRegistry) Remove(sourceID string) error {
	if _, exists := r.sources[sourceID]; !exists {
		return fmt.Errorf("source %q not found", sourceID)
	}

	delete(r.sources, sourceID)
	return nil
}

func (r *SourceRegistry) Get(sourceID string) (config.SourceConfig, bool) {
	source, ok := r.sources[sourceID]
	return source, ok
}

func (r *SourceRegistry) List() []config.SourceConfig {
	out := make([]config.SourceConfig, 0, len(r.sources))
	for _, source := range r.sources {
		out = append(out, source)
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})

	return out
}

func normalizeProvider(provider string) string {
	value := strings.ToLower(strings.TrimSpace(provider))
	if value == "azure" {
		return "azuredevops"
	}
	return value
}

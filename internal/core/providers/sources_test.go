package providers

import (
	"testing"

	"github.com/basmulder03/git-project-sync/internal/core/config"
)

func TestSourceRegistryAddListGetRemove(t *testing.T) {
	t.Parallel()

	registry, err := NewSourceRegistry(nil)
	if err != nil {
		t.Fatalf("new source registry: %v", err)
	}

	input := config.SourceConfig{
		ID:       "gh-work",
		Provider: "github",
		Account:  "jane-work",
		Host:     "github.com",
		Enabled:  true,
	}

	if err := registry.Add(input); err != nil {
		t.Fatalf("add source: %v", err)
	}

	source, ok := registry.Get("gh-work")
	if !ok {
		t.Fatal("expected source to exist")
	}
	if source.Provider != "github" {
		t.Fatalf("provider = %q, want github", source.Provider)
	}

	list := registry.List()
	if len(list) != 1 {
		t.Fatalf("list len = %d, want 1", len(list))
	}

	if err := registry.Remove("gh-work"); err != nil {
		t.Fatalf("remove source: %v", err)
	}
}

func TestSourceRegistryNormalizesAzureAlias(t *testing.T) {
	t.Parallel()

	registry, err := NewSourceRegistry(nil)
	if err != nil {
		t.Fatalf("new source registry: %v", err)
	}

	err = registry.Add(config.SourceConfig{
		ID:       "az-work",
		Provider: "azure",
		Account:  "contoso",
		Host:     "dev.azure.com",
		Enabled:  true,
	})
	if err != nil {
		t.Fatalf("add source: %v", err)
	}

	source, ok := registry.Get("az-work")
	if !ok {
		t.Fatal("expected source to exist")
	}
	if source.Provider != "azuredevops" {
		t.Fatalf("provider = %q, want azuredevops", source.Provider)
	}
}

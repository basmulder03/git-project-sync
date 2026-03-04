package workspace

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/basmulder03/git-project-sync/internal/core/config"
)

type Drift struct {
	RepoIndex     int
	RepoPath      string
	ExpectedPath  string
	SourceID      string
	ReasonCode    string
	ReasonMessage string
}

type Validator struct {
	resolver *LayoutResolver
	bySource map[string]config.SourceConfig
}

func NewValidator(cfg config.Config) (*Validator, error) {
	resolver, err := NewLayoutResolver(cfg.Workspace.Root)
	if err != nil {
		return nil, err
	}

	bySource := make(map[string]config.SourceConfig, len(cfg.Sources))
	for _, source := range cfg.Sources {
		bySource[source.ID] = source
	}

	return &Validator{resolver: resolver, bySource: bySource}, nil
}

func (v *Validator) Check(cfg config.Config) ([]Drift, error) {
	drifts := make([]Drift, 0)

	for index, repo := range cfg.Repos {
		source, ok := v.bySource[repo.SourceID]
		if !ok {
			drifts = append(drifts, Drift{
				RepoIndex:     index,
				RepoPath:      repo.Path,
				SourceID:      repo.SourceID,
				ReasonCode:    "source_missing",
				ReasonMessage: fmt.Sprintf("source %q not configured", repo.SourceID),
			})
			continue
		}

		expectedPath, err := v.resolver.ExpectedRepoPath(source, repo)
		if err != nil {
			return nil, err
		}

		if filepath.Clean(repo.Path) != filepath.Clean(expectedPath) {
			drifts = append(drifts, Drift{
				RepoIndex:     index,
				RepoPath:      repo.Path,
				ExpectedPath:  expectedPath,
				SourceID:      repo.SourceID,
				ReasonCode:    "path_mismatch",
				ReasonMessage: "repository path does not follow managed workspace layout",
			})
		}
	}

	return drifts, nil
}

func ApplyPathFixes(cfg *config.Config, drifts []Drift, createDirs bool) (int, error) {
	updates := 0
	for _, drift := range drifts {
		if drift.ReasonCode != "path_mismatch" || drift.ExpectedPath == "" {
			continue
		}

		cfg.Repos[drift.RepoIndex].Path = drift.ExpectedPath
		updates++

		if createDirs {
			if err := os.MkdirAll(drift.ExpectedPath, 0o755); err != nil {
				return updates, fmt.Errorf("create repo directory %q: %w", drift.ExpectedPath, err)
			}
		}
	}

	return updates, nil
}

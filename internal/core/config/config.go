package config

import (
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

const CurrentSchemaVersion = 1

type Config struct {
	SchemaVersion int             `yaml:"schema_version"`
	Workspace     WorkspaceConfig `yaml:"workspace"`
	State         StateConfig     `yaml:"state"`
	Daemon        DaemonConfig    `yaml:"daemon"`
	Update        UpdateConfig    `yaml:"update"`
	Logging       LoggingConfig   `yaml:"logging"`
	Cache         CacheConfig     `yaml:"cache"`
	Sources       []SourceConfig  `yaml:"sources"`
	Repos         []RepoConfig    `yaml:"repos"`
}

type WorkspaceConfig struct {
	Root               string `yaml:"root"`
	Layout             string `yaml:"layout"`
	CreateMissingPaths bool   `yaml:"create_missing_paths"`
}

type StateConfig struct {
	DBPath            string `yaml:"db_path"`
	PersistEventsDays int    `yaml:"persist_events_days"`
}

type DaemonConfig struct {
	IntervalSeconds         int         `yaml:"interval_seconds"`
	JitterSeconds           int         `yaml:"jitter_seconds"`
	MaxParallelRepos        int         `yaml:"max_parallel_repos"`
	OperationTimeoutSeconds int         `yaml:"operation_timeout_seconds"`
	Retry                   RetryConfig `yaml:"retry"`
}

type RetryConfig struct {
	MaxAttempts        int `yaml:"max_attempts"`
	BaseBackoffSeconds int `yaml:"base_backoff_seconds"`
}

type UpdateConfig struct {
	Channel   string `yaml:"channel"`
	AutoCheck bool   `yaml:"auto_check"`
	AutoApply bool   `yaml:"auto_apply"`
}

type LoggingConfig struct {
	Level         string `yaml:"level"`
	Format        string `yaml:"format"`
	RedactSecrets bool   `yaml:"redact_secrets"`
}

type CacheConfig struct {
	ProviderTTLSeconds int `yaml:"provider_ttl_seconds"`
	BranchTTLSeconds   int `yaml:"branch_ttl_seconds"`
}

type SourceConfig struct {
	ID            string `yaml:"id"`
	Provider      string `yaml:"provider"`
	Account       string `yaml:"account"`
	Organization  string `yaml:"organization"`
	Host          string `yaml:"host"`
	Enabled       bool   `yaml:"enabled"`
	CredentialRef string `yaml:"credential_ref"`
}

type RepoConfig struct {
	Path                       string `yaml:"path"`
	SourceID                   string `yaml:"source_id"`
	Remote                     string `yaml:"remote"`
	Enabled                    bool   `yaml:"enabled"`
	Provider                   string `yaml:"provider"`
	CleanupMergedLocalBranches bool   `yaml:"cleanup_merged_local_branches"`
	SkipIfDirty                bool   `yaml:"skip_if_dirty"`
}

func Default() Config {
	return Config{
		SchemaVersion: CurrentSchemaVersion,
		Workspace: WorkspaceConfig{
			Layout:             "provider-account-repo",
			CreateMissingPaths: true,
		},
		State: StateConfig{
			DBPath:            "state/sync.db",
			PersistEventsDays: 30,
		},
		Daemon: DaemonConfig{
			IntervalSeconds:         300,
			JitterSeconds:           30,
			MaxParallelRepos:        4,
			OperationTimeoutSeconds: 120,
			Retry: RetryConfig{
				MaxAttempts:        3,
				BaseBackoffSeconds: 2,
			},
		},
		Update: UpdateConfig{
			Channel:   "stable",
			AutoCheck: true,
		},
		Logging: LoggingConfig{
			Level:         "info",
			Format:        "json",
			RedactSecrets: true,
		},
		Cache: CacheConfig{
			ProviderTTLSeconds: 900,
			BranchTTLSeconds:   120,
		},
		Sources: []SourceConfig{},
		Repos:   []RepoConfig{},
	}
}

func Load(path string) (Config, error) {
	cfg := Default()
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func (c Config) Validate() error {
	if c.SchemaVersion != CurrentSchemaVersion {
		return fmt.Errorf("unsupported schema_version %d (expected %d)", c.SchemaVersion, CurrentSchemaVersion)
	}

	if c.Daemon.IntervalSeconds <= 0 {
		return errors.New("daemon.interval_seconds must be > 0")
	}

	if c.State.DBPath == "" {
		return errors.New("state.db_path must be set")
	}

	if c.State.PersistEventsDays <= 0 {
		return errors.New("state.persist_events_days must be > 0")
	}

	if c.Daemon.MaxParallelRepos <= 0 {
		return errors.New("daemon.max_parallel_repos must be > 0")
	}

	if c.Daemon.Retry.MaxAttempts <= 0 {
		return errors.New("daemon.retry.max_attempts must be > 0")
	}

	return nil
}

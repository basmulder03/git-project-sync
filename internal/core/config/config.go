package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const CurrentSchemaVersion = 1

type Config struct {
	SchemaVersion int              `yaml:"schema_version"`
	Workspace     WorkspaceConfig  `yaml:"workspace"`
	State         StateConfig      `yaml:"state"`
	Daemon        DaemonConfig     `yaml:"daemon"`
	Update        UpdateConfig     `yaml:"update"`
	Logging       LoggingConfig    `yaml:"logging"`
	Cache         CacheConfig      `yaml:"cache"`
	Governance    GovernanceConfig `yaml:"governance"`
	Sources       []SourceConfig   `yaml:"sources"`
	Repos         []RepoConfig     `yaml:"repos"`
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
	MaxParallelPerSource    int         `yaml:"max_parallel_per_source"`
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

type GovernanceConfig struct {
	DefaultPolicy  SyncPolicyConfig            `yaml:"default_policy"`
	SourcePolicies map[string]SyncPolicyConfig `yaml:"source_policies"`
}

type SyncPolicyConfig struct {
	IncludeRepoPatterns      []string           `yaml:"include_repo_patterns"`
	ExcludeRepoPatterns      []string           `yaml:"exclude_repo_patterns"`
	ProtectedRepoPatterns    []string           `yaml:"protected_repo_patterns"`
	AllowedSyncWindows       []SyncWindowConfig `yaml:"allowed_sync_windows"`
	AutoCloneEnabled         *bool              `yaml:"auto_clone_enabled"`          // pointer to allow nil = inherit
	AutoCloneMaxSizeMB       int                `yaml:"auto_clone_max_size_mb"`      // 0 = unlimited
	AutoCloneIncludeArchived bool               `yaml:"auto_clone_include_archived"` // default false
}

type SyncWindowConfig struct {
	Days  []string `yaml:"days"`
	Start string   `yaml:"start"`
	End   string   `yaml:"end"`
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

func (r *RepoConfig) UnmarshalYAML(value *yaml.Node) error {
	type rawRepoConfig RepoConfig
	aux := rawRepoConfig{
		Enabled:                    true,
		Remote:                     "origin",
		Provider:                   "auto",
		CleanupMergedLocalBranches: true,
		SkipIfDirty:                true,
	}
	if err := value.Decode(&aux); err != nil {
		return err
	}
	*r = RepoConfig(aux)
	return nil
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
			MaxParallelPerSource:    2,
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
		Governance: GovernanceConfig{
			DefaultPolicy: SyncPolicyConfig{
				AutoCloneEnabled:         boolPtr(true),
				AutoCloneMaxSizeMB:       2048, // 2GB default max
				AutoCloneIncludeArchived: false,
			},
			SourcePolicies: map[string]SyncPolicyConfig{},
		},
		Sources: []SourceConfig{},
		Repos:   []RepoConfig{},
	}
}

// boolPtr returns a pointer to a bool value
func boolPtr(b bool) *bool {
	return &b
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

func Save(path string, cfg Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}

	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	return nil
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

	if c.Daemon.MaxParallelPerSource <= 0 {
		return errors.New("daemon.max_parallel_per_source must be > 0")
	}

	if c.Daemon.Retry.MaxAttempts <= 0 {
		return errors.New("daemon.retry.max_attempts must be > 0")
	}

	if err := validatePolicy(c.Governance.DefaultPolicy, "governance.default_policy"); err != nil {
		return err
	}

	for sourceID, policy := range c.Governance.SourcePolicies {
		if strings.TrimSpace(sourceID) == "" {
			return errors.New("governance.source_policies keys must be non-empty")
		}
		if err := validatePolicy(policy, fmt.Sprintf("governance.source_policies[%q]", sourceID)); err != nil {
			return err
		}
	}

	return nil
}

func validatePolicy(policy SyncPolicyConfig, path string) error {
	for i, pattern := range append(append([]string{}, policy.IncludeRepoPatterns...), append(policy.ExcludeRepoPatterns, policy.ProtectedRepoPatterns...)...) {
		if strings.TrimSpace(pattern) == "" {
			return fmt.Errorf("%s contains empty repository pattern at index %d", path, i)
		}
		if _, err := regexp.Compile(pattern); err != nil {
			return fmt.Errorf("%s invalid repository pattern %q: %w", path, pattern, err)
		}
	}

	for i, window := range policy.AllowedSyncWindows {
		if len(window.Days) == 0 {
			return fmt.Errorf("%s.allowed_sync_windows[%d].days must not be empty", path, i)
		}
		for _, day := range window.Days {
			if _, ok := dayNameToWeekday(day); !ok {
				return fmt.Errorf("%s.allowed_sync_windows[%d].days contains invalid day %q", path, i, day)
			}
		}
		if _, err := parseClock(window.Start); err != nil {
			return fmt.Errorf("%s.allowed_sync_windows[%d].start: %w", path, i, err)
		}
		if _, err := parseClock(window.End); err != nil {
			return fmt.Errorf("%s.allowed_sync_windows[%d].end: %w", path, i, err)
		}
	}

	return nil
}

func parseClock(raw string) (time.Duration, error) {
	if strings.TrimSpace(raw) == "" {
		return 0, errors.New("time must be set as HH:MM")
	}
	t, err := time.Parse("15:04", raw)
	if err != nil {
		return 0, errors.New("time must use 24h HH:MM format")
	}
	return time.Duration(t.Hour())*time.Hour + time.Duration(t.Minute())*time.Minute, nil
}

func dayNameToWeekday(day string) (time.Weekday, bool) {
	switch strings.ToLower(strings.TrimSpace(day)) {
	case "sun", "sunday":
		return time.Sunday, true
	case "mon", "monday":
		return time.Monday, true
	case "tue", "tuesday":
		return time.Tuesday, true
	case "wed", "wednesday":
		return time.Wednesday, true
	case "thu", "thursday":
		return time.Thursday, true
	case "fri", "friday":
		return time.Friday, true
	case "sat", "saturday":
		return time.Saturday, true
	default:
		return time.Sunday, false
	}
}

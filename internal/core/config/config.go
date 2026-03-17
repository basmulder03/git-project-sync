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
	SchemaVersion int                 `yaml:"schema_version"`
	Workspace     WorkspaceConfig     `yaml:"workspace"`
	State         StateConfig         `yaml:"state"`
	Daemon        DaemonConfig        `yaml:"daemon"`
	Update        UpdateConfig        `yaml:"update"`
	Logging       LoggingConfig       `yaml:"logging"`
	Cache         CacheConfig         `yaml:"cache"`
	Governance    GovernanceConfig    `yaml:"governance"`
	SSH           SSHConfig           `yaml:"ssh"`
	Notifications NotificationsConfig `yaml:"notifications"`
	Sources       []SourceConfig      `yaml:"sources"`
	Repos         []RepoConfig        `yaml:"repos"`
}

// NotificationsConfig holds optional outbound notification sinks.
// Payloads are always redacted: no token values, paths, or secrets.
type NotificationsConfig struct {
	// Sinks is the ordered list of outbound notification targets.
	Sinks []NotificationSinkConfig `yaml:"sinks"`
}

// NotificationSinkConfig configures a single outbound notification sink.
type NotificationSinkConfig struct {
	// Name is a human-readable label for this sink (used in logs).
	Name string `yaml:"name"`

	// Type selects the sink implementation. Supported: "webhook", "slack", "teams".
	Type string `yaml:"type"`

	// URL is the endpoint to POST the event payload to.
	// Must not contain embedded credentials. Secrets must be in the OS keyring.
	URL string `yaml:"url"`

	// MinSeverity controls the minimum event level that triggers a notification.
	// Accepted values: "info", "warn", "error". Defaults to "error".
	MinSeverity string `yaml:"min_severity"`

	// ReasonCodes, when non-empty, restricts notifications to events whose
	// ReasonCode matches one of the listed values.  Empty means all reasons pass.
	ReasonCodes []string `yaml:"reason_codes"`

	// Enabled allows a sink to be temporarily disabled without removing it.
	Enabled bool `yaml:"enabled"`
}

type WorkspaceConfig struct {
	Root               string `yaml:"root"`
	Layout             string `yaml:"layout"`
	CreateMissingPaths bool   `yaml:"create_missing_paths"`
}

// SSHConfig holds global SSH preferences for git-project-sync.
type SSHConfig struct {
	// Enabled controls whether SSH is the preferred transport for git operations.
	// When true (default), SSH URLs and SSH keys are used for clone/fetch/push.
	// When false, HTTPS with token-based auth is used instead.
	Enabled bool `yaml:"enabled"`

	// KeyDir is the directory where managed SSH private keys are stored.
	// Defaults to <app-config-dir>/ssh.
	KeyDir string `yaml:"key_dir"`

	// SSHConfigPath is the user SSH config file to update with Host blocks.
	// Defaults to ~/.ssh/config.
	SSHConfigPath string `yaml:"ssh_config_path"`

	// GitHubOAuthClientID overrides the default OAuth app client ID used for
	// the GitHub device-flow SSH key upload.  Leave empty to use the built-in
	// default (github-cli compatible).
	GitHubOAuthClientID string `yaml:"github_oauth_client_id"`

	// MigrationOptIn records whether the user accepted the SSH migration offer
	// on first startup.  "accepted", "declined", or "" (not yet prompted).
	MigrationOptIn string `yaml:"migration_opt_in"`

	// WSL holds settings for WSL ↔ native Windows SSH interoperability.
	// These only take effect when the daemon/CLI is running inside WSL.
	WSL WSLSSHConfig `yaml:"wsl"`
}

// WSLSSHConfig controls SSH interoperability between WSL and native Windows.
// When the tool runs inside WSL, SSH keys and configuration can be stored on
// the Windows filesystem so that both the WSL git client and the native
// Windows git client share the same keys without duplication.
type WSLSSHConfig struct {
	// SyncToWindows, when true (default when running in WSL), mirrors every
	// gps-* Host block written to the WSL SSH config into the Windows user SSH
	// config (C:\Users\<user>\.ssh\config expressed as /mnt/c/...).
	// Set to false to manage the two configs independently.
	SyncToWindows *bool `yaml:"sync_to_windows"`

	// UseWindowsKeyDir, when true (default when running in WSL), stores SSH
	// private keys under the Windows LOCALAPPDATA directory
	// (/mnt/c/Users/<user>/AppData/Local/git-project-sync/ssh expressed as a
	// WSL path).  This means native Windows git can also read the keys without
	// any duplication.  Set to false to store keys in the Linux-side KeyDir.
	UseWindowsKeyDir *bool `yaml:"use_windows_key_dir"`

	// WindowsKeyDir overrides the computed Windows key directory.  Leave empty
	// to use the auto-detected Windows LOCALAPPDATA path.
	WindowsKeyDir string `yaml:"windows_key_dir"`

	// WindowsSSHConfigPath overrides the computed Windows SSH config path.
	// Leave empty to use the auto-detected path (/mnt/c/Users/<user>/.ssh/config).
	WindowsSSHConfigPath string `yaml:"windows_ssh_config_path"`
}

type StateConfig struct {
	DBPath            string `yaml:"db_path"`
	PersistEventsDays int    `yaml:"persist_events_days"`
}

type DaemonConfig struct {
	IntervalSeconds          int                 `yaml:"interval_seconds"`
	DiscoveryIntervalSeconds int                 `yaml:"discovery_interval_seconds"` // How often to run discovery+clone (0 = only at startup)
	JitterSeconds            int                 `yaml:"jitter_seconds"`
	MaxParallelRepos         int                 `yaml:"max_parallel_repos"`
	MaxParallelPerSource     int                 `yaml:"max_parallel_per_source"`
	OperationTimeoutSeconds  int                 `yaml:"operation_timeout_seconds"`
	Retry                    RetryConfig         `yaml:"retry"`
	MaintenanceWindows       []MaintenanceWindow `yaml:"maintenance_windows"`
}

// MaintenanceWindow defines a recurring calendar window during which all
// mutating sync operations are suppressed.  Unlike governance AllowedSyncWindows
// (which restrict when sync is allowed), a MaintenanceWindow explicitly BLOCKS
// sync. It applies globally across all sources and repos.
type MaintenanceWindow struct {
	// Name is a short human-readable label emitted in skip-reason log lines.
	Name string `yaml:"name"`

	// Days lists the calendar days this window applies to.
	// Valid values: "monday" .. "sunday" (or three-letter abbreviations).
	Days []string `yaml:"days"`

	// Start and End are 24-hour HH:MM clock times (inclusive start, exclusive end).
	Start string `yaml:"start"`
	End   string `yaml:"end"`
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
	ID            string          `yaml:"id"`
	Provider      string          `yaml:"provider"`
	Account       string          `yaml:"account"`
	Organization  string          `yaml:"organization"`
	Host          string          `yaml:"host"`
	Enabled       bool            `yaml:"enabled"`
	CredentialRef string          `yaml:"credential_ref"`
	SSH           SourceSSHConfig `yaml:"ssh"`
}

// SourceSSHConfig holds per-source SSH settings.
type SourceSSHConfig struct {
	// Enabled overrides the global ssh.enabled setting for this source.
	// nil means "inherit from global".
	Enabled *bool `yaml:"enabled"`

	// KeyPath is the path to the private key for this source.
	// If empty, the managed key path is used (derived from the source ID).
	KeyPath string `yaml:"key_path"`

	// SSHHost is the SSH hostname for this source (overrides the default for
	// the provider, e.g. for GitHub Enterprise that uses a non-standard host).
	SSHHost string `yaml:"ssh_host"`

	// KeyUploaded records whether the public key has been successfully uploaded
	// to the provider.  Used to skip re-upload on subsequent starts.
	KeyUploaded bool `yaml:"key_uploaded"`
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
			DBPath:            DefaultDBPath(),
			PersistEventsDays: 30,
		},
		Daemon: DaemonConfig{
			IntervalSeconds:          300,
			DiscoveryIntervalSeconds: 3600, // Run discovery every hour (vs sync every 5 minutes)
			JitterSeconds:            30,
			MaxParallelRepos:         4,
			MaxParallelPerSource:     2,
			OperationTimeoutSeconds:  120,
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
		SSH: SSHConfig{
			Enabled: true, // SSH is the preferred transport by default.
		},
		Sources: []SourceConfig{},
		Repos:   []RepoConfig{},
	}
}

// BoolPtr returns a pointer to a bool value (exported for use in tests and other packages)
func BoolPtr(b bool) *bool {
	return &b
}

// boolPtr is an internal alias for BoolPtr
func boolPtr(b bool) *bool {
	return BoolPtr(b)
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

	// Run configuration migrations after loading and validation
	if err := RunMigrations(&cfg); err != nil {
		return Config{}, fmt.Errorf("run migrations: %w", err)
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

	for i, mw := range c.Daemon.MaintenanceWindows {
		if len(mw.Days) == 0 {
			return fmt.Errorf("daemon.maintenance_windows[%d].days must not be empty", i)
		}
		for _, day := range mw.Days {
			if _, ok := dayNameToWeekday(day); !ok {
				return fmt.Errorf("daemon.maintenance_windows[%d].days contains invalid day %q", i, day)
			}
		}
		if _, err := parseClock(mw.Start); err != nil {
			return fmt.Errorf("daemon.maintenance_windows[%d].start: %w", i, err)
		}
		if _, err := parseClock(mw.End); err != nil {
			return fmt.Errorf("daemon.maintenance_windows[%d].end: %w", i, err)
		}
	}

	for i, sink := range c.Notifications.Sinks {
		if strings.TrimSpace(sink.Name) == "" {
			return fmt.Errorf("notifications.sinks[%d].name must not be empty", i)
		}
		switch strings.ToLower(strings.TrimSpace(sink.Type)) {
		case "webhook", "slack", "teams":
		default:
			return fmt.Errorf("notifications.sinks[%d].type %q is not supported (use webhook, slack, or teams)", i, sink.Type)
		}
		if strings.TrimSpace(sink.URL) == "" {
			return fmt.Errorf("notifications.sinks[%d].url must not be empty", i)
		}
		switch strings.ToLower(strings.TrimSpace(sink.MinSeverity)) {
		case "", "info", "warn", "error":
		default:
			return fmt.Errorf("notifications.sinks[%d].min_severity %q is not valid (use info, warn, or error)", i, sink.MinSeverity)
		}
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

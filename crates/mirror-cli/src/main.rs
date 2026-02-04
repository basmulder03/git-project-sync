use anyhow::Context;
use clap::{Parser, ValueEnum};
use mirror_core::cache::{
    RepoCacheEntry, backoff_until, update_target_failure, update_target_success,
};
use mirror_core::config::{
    AppConfigV2, TargetConfig, default_cache_path, default_config_path, default_lock_path,
    load_or_migrate, target_id,
};
use mirror_core::deleted::{DeletedRepoAction, MissingRemotePolicy};
use mirror_core::audit::{AuditContext, AuditLogger, AuditStatus};
use mirror_core::model::{ProviderKind, ProviderScope, ProviderTarget};
use mirror_core::scheduler::{bucket_for_repo_id, current_day_bucket};
use mirror_core::sync_engine::{SyncSummary, run_sync_filtered};
use mirror_providers::auth;
use mirror_providers::azure_devops::{AZDO_DEFAULT_OAUTH_SCOPE, AzureDevOpsProvider};
use mirror_providers::spec::{host_or_default, pat_help, spec_for};
use mirror_providers::ProviderRegistry;
use tracing::warn;
use reqwest::StatusCode;
use reqwest::blocking::Client;
use serde::Deserialize;
use std::io::{self, Write};
use std::path::{Path, PathBuf};
use tracing_subscriber::EnvFilter;
mod tray;
mod tui;

#[derive(Parser)]
#[command(author, version, about)]
struct Cli {
    #[command(subcommand)]
    command: Commands,
}

#[derive(clap::Subcommand)]
enum Commands {
    #[command(about = "Manage config")]
    Config(ConfigArgs),
    #[command(about = "Manage targets")]
    Target(TargetArgs),
    #[command(about = "Manage auth tokens")]
    Token(TokenArgs),
    #[command(about = "Sync repos")]
    Sync(SyncArgs),
    #[command(about = "Run the daemon loop")]
    Daemon(DaemonArgs),
    #[command(about = "Install or uninstall background service helpers (placeholder)")]
    Service(ServiceArgs),
    #[command(about = "Validate provider auth and scope")]
    Health(HealthArgs),
    #[command(about = "Manage webhooks")]
    Webhook(WebhookArgs),
    #[command(about = "OAuth helpers")]
    Oauth(OauthArgs),
    #[command(about = "Manage cache")]
    Cache(CacheArgs),
    #[command(about = "Launch terminal UI")]
    Tui(TuiArgs),
    #[command(about = "Run system tray UI")]
    Tray,
}

#[derive(Parser)]
struct ConfigArgs {
    #[command(subcommand)]
    command: ConfigCommands,
}

#[derive(clap::Subcommand)]
enum ConfigCommands {
    #[command(about = "Initialize config with a mirror root path")]
    Init(InitArgs),
}

#[derive(Parser)]
struct InitArgs {
    #[arg(long)]
    root: PathBuf,
}

#[derive(Parser)]
struct TargetArgs {
    #[command(subcommand)]
    command: TargetCommands,
}

#[derive(clap::Subcommand)]
enum TargetCommands {
    #[command(about = "Add a provider target to the config")]
    Add(AddTargetArgs),
    #[command(about = "List configured targets")]
    List,
    #[command(about = "Remove a target by id")]
    Remove(RemoveTargetArgs),
}

#[derive(Parser)]
struct AddTargetArgs {
    #[arg(long, value_enum)]
    provider: ProviderKindValue,
    #[arg(long, required = true)]
    scope: Vec<String>,
    #[arg(long)]
    host: Option<String>,
    #[arg(long, value_delimiter = ',')]
    label: Vec<String>,
}

#[derive(Parser)]
struct RemoveTargetArgs {
    #[arg(long)]
    id: String,
}

#[derive(Parser)]
struct TokenArgs {
    #[command(subcommand)]
    command: TokenCommands,
}

#[derive(clap::Subcommand)]
enum TokenCommands {
    #[command(about = "Store an auth token for a provider target")]
    Set(SetTokenArgs),
    #[command(about = "Show PAT guidance for a provider")]
    Guide(GuideTokenArgs),
    #[command(about = "Validate token scopes when supported")]
    Validate(ValidateTokenArgs),
}

#[derive(Parser)]
struct SetTokenArgs {
    #[arg(long, value_enum)]
    provider: ProviderKindValue,
    #[arg(long, required = true)]
    scope: Vec<String>,
    #[arg(long)]
    host: Option<String>,
    #[arg(long)]
    token: String,
}

#[derive(Parser)]
struct GuideTokenArgs {
    #[arg(long, value_enum)]
    provider: ProviderKindValue,
    #[arg(long, required = true)]
    scope: Vec<String>,
    #[arg(long)]
    host: Option<String>,
}

#[derive(Parser)]
struct ValidateTokenArgs {
    #[arg(long, value_enum)]
    provider: ProviderKindValue,
    #[arg(long, required = true)]
    scope: Vec<String>,
    #[arg(long)]
    host: Option<String>,
}

#[derive(Parser)]
struct SyncArgs {
    #[arg(long)]
    target_id: Option<String>,
    #[arg(long, value_enum)]
    provider: Option<ProviderKindValue>,
    #[arg(long)]
    scope: Vec<String>,
    #[arg(long)]
    repo: Option<String>,
    #[arg(long)]
    refresh: bool,
    #[arg(long)]
    include_archived: bool,
    #[arg(long)]
    verify: bool,
    #[arg(long)]
    non_interactive: bool,
    #[arg(long, value_enum, default_value = "prompt")]
    missing_remote: MissingRemotePolicyValue,
    #[arg(long)]
    config: Option<PathBuf>,
    #[arg(long)]
    cache: Option<PathBuf>,
}

#[derive(Parser)]
struct DaemonArgs {
    #[arg(long)]
    lock: Option<PathBuf>,
    #[arg(long, default_value = "3600")]
    interval_seconds: u64,
    #[arg(long)]
    run_once: bool,
    #[arg(long, value_enum, default_value = "skip")]
    missing_remote: MissingRemotePolicyValue,
    #[arg(long)]
    config: Option<PathBuf>,
    #[arg(long)]
    cache: Option<PathBuf>,
}

#[derive(Parser)]
struct ServiceArgs {
    #[arg(value_enum)]
    action: ServiceAction,
}

#[derive(Parser)]
struct WebhookArgs {
    #[command(subcommand)]
    command: WebhookCommands,
}

#[derive(clap::Subcommand)]
enum WebhookCommands {
    #[command(about = "Register a webhook for a provider target")]
    Register(WebhookRegisterArgs),
}

#[derive(Parser)]
struct WebhookRegisterArgs {
    #[arg(long, value_enum)]
    provider: ProviderKindValue,
    #[arg(long, required = true)]
    scope: Vec<String>,
    #[arg(long)]
    host: Option<String>,
    #[arg(long)]
    url: String,
    #[arg(long)]
    secret: Option<String>,
}

#[derive(Parser)]
struct CacheArgs {
    #[command(subcommand)]
    command: CacheCommands,
}

#[derive(clap::Subcommand)]
enum CacheCommands {
    #[command(about = "Prune cache entries for missing targets")]
    Prune(CachePruneArgs),
}

#[derive(Parser)]
struct CachePruneArgs {
    #[arg(long)]
    config: Option<PathBuf>,
    #[arg(long)]
    cache: Option<PathBuf>,
}

#[derive(Parser)]
struct OauthArgs {
    #[command(subcommand)]
    command: OauthCommands,
}

#[derive(clap::Subcommand)]
enum OauthCommands {
    #[command(about = "Start OAuth device flow")]
    Device(DeviceFlowArgs),
    #[command(about = "Revoke stored OAuth token")]
    Revoke(RevokeOauthArgs),
}

#[derive(Parser)]
struct DeviceFlowArgs {
    #[arg(long, value_enum)]
    provider: ProviderKindValue,
    #[arg(long, required = true)]
    scope: Vec<String>,
    #[arg(long)]
    host: Option<String>,
    #[arg(long)]
    client_id: String,
    #[arg(long)]
    tenant: Option<String>,
    #[arg(long, value_name = "OAUTH_SCOPE")]
    oauth_scope: Vec<String>,
}

#[derive(Parser)]
struct RevokeOauthArgs {
    #[arg(long, value_enum)]
    provider: ProviderKindValue,
    #[arg(long, required = true)]
    scope: Vec<String>,
    #[arg(long)]
    host: Option<String>,
}

#[derive(Parser)]
struct HealthArgs {
    #[arg(long)]
    target_id: Option<String>,
    #[arg(long, value_enum)]
    provider: Option<ProviderKindValue>,
    #[arg(long)]
    scope: Vec<String>,
    #[arg(long)]
    config: Option<PathBuf>,
}

#[derive(Parser)]
struct TuiArgs {
    #[arg(long)]
    dashboard: bool,
}

#[derive(Clone, Copy, ValueEnum)]
enum ServiceAction {
    Install,
    Uninstall,
}

#[derive(Clone, Copy, ValueEnum)]
enum ProviderKindValue {
    AzureDevOps,
    GitHub,
    GitLab,
}

impl From<ProviderKindValue> for ProviderKind {
    fn from(value: ProviderKindValue) -> Self {
        match value {
            ProviderKindValue::AzureDevOps => ProviderKind::AzureDevOps,
            ProviderKindValue::GitHub => ProviderKind::GitHub,
            ProviderKindValue::GitLab => ProviderKind::GitLab,
        }
    }
}

#[derive(Clone, Copy, ValueEnum, PartialEq, Eq)]
enum MissingRemotePolicyValue {
    Prompt,
    Archive,
    Remove,
    Skip,
}

impl From<MissingRemotePolicyValue> for MissingRemotePolicy {
    fn from(value: MissingRemotePolicyValue) -> Self {
        match value {
            MissingRemotePolicyValue::Prompt => MissingRemotePolicy::Prompt,
            MissingRemotePolicyValue::Archive => MissingRemotePolicy::Archive,
            MissingRemotePolicyValue::Remove => MissingRemotePolicy::Remove,
            MissingRemotePolicyValue::Skip => MissingRemotePolicy::Skip,
        }
    }
}

fn main() -> anyhow::Result<()> {
    tracing_subscriber::fmt()
        .with_env_filter(EnvFilter::from_default_env())
        .init();

    let audit = AuditLogger::new()?;
    let _ = audit.record("app.start", AuditStatus::Ok, None, None, None)?;
    auth::set_audit_logger(audit.clone());

    let cli = Cli::parse();
    let result = match cli.command {
        Commands::Config(args) => handle_config(args, &audit),
        Commands::Target(args) => handle_target(args, &audit),
        Commands::Token(args) => handle_token(args, &audit),
        Commands::Sync(args) => handle_sync(args, &audit),
        Commands::Daemon(args) => handle_daemon(args, &audit),
        Commands::Service(args) => handle_service(args, &audit),
        Commands::Health(args) => handle_health(args, &audit),
        Commands::Webhook(args) => handle_webhook(args, &audit),
        Commands::Oauth(args) => handle_oauth(args, &audit),
        Commands::Cache(args) => handle_cache(args, &audit),
        Commands::Tui(args) => tui::run_tui(&audit, args.dashboard),
        Commands::Tray => tray::run_tray(&audit),
    };

    if let Err(err) = &result {
        let _ = audit.record(
            "app.error",
            AuditStatus::Failed,
            None,
            None,
            Some(&err.to_string()),
        );
    }

    result
}

fn handle_config(args: ConfigArgs, audit: &AuditLogger) -> anyhow::Result<()> {
    match args.command {
        ConfigCommands::Init(args) => handle_init(args, audit),
    }
}

fn handle_init(args: InitArgs, audit: &AuditLogger) -> anyhow::Result<()> {
    let result: anyhow::Result<()> = (|| {
        let config_path = default_config_path()?;
        let (mut config, migrated) = load_or_migrate(&config_path)?;
        config.root = Some(args.root);
        config.save(&config_path)?;
        if migrated {
            println!("Config migrated and saved to {}", config_path.display());
        } else {
            println!("Config saved to {}", config_path.display());
        }
        Ok(())
    })();

    if let Err(err) = &result {
        let _ = audit.record(
            "config.init",
            AuditStatus::Failed,
            Some("config.init"),
            None,
            Some(&err.to_string()),
        );
    } else {
        let audit_id = audit.record(
            "config.init",
            AuditStatus::Ok,
            Some("config.init"),
            None,
            None,
        )?;
        println!("Audit ID: {audit_id}");
    }
    result
}

fn handle_target(args: TargetArgs, audit: &AuditLogger) -> anyhow::Result<()> {
    match args.command {
        TargetCommands::Add(args) => handle_add_target(args, audit),
        TargetCommands::List => handle_list_targets(audit),
        TargetCommands::Remove(args) => handle_remove_target(args, audit),
    }
}

fn handle_add_target(args: AddTargetArgs, audit: &AuditLogger) -> anyhow::Result<()> {
    let result: anyhow::Result<()> = (|| {
        let config_path = default_config_path()?;
        let (mut config, migrated) = load_or_migrate(&config_path)?;
        let provider: ProviderKind = args.provider.into();
        let spec = spec_for(provider.clone());
        let scope = spec.parse_scope(args.scope)?;

        let host = args.host.as_ref().map(|value| value.trim_end_matches('/').to_string());
        let id = target_id(provider.clone(), host.as_deref(), &scope);

        if config.targets.iter().any(|target| target.id == id) {
            println!("Target already exists: {id}");
            let audit_id = audit.record_with_context(
                "target.add",
                AuditStatus::Skipped,
                Some("target.add"),
                AuditContext {
                    provider: Some(provider.as_prefix().to_string()),
                    scope: Some(scope.segments().join("/")),
                    repo_id: None,
                    path: None,
                },
                None,
                Some("target already exists"),
            )?;
            println!("Audit ID: {audit_id}");
            return Ok(());
        }

        let target = TargetConfig {
            id: id.clone(),
            provider: provider.clone(),
            scope: scope.clone(),
            host,
            labels: args.label,
        };

        config.targets.push(target);
        config.save(&config_path)?;
        if migrated {
            println!("Config migrated and target added to {}", config_path.display());
        } else {
            println!("Target added to {}", config_path.display());
        }
        let audit_id = audit.record_with_context(
            "target.add",
            AuditStatus::Ok,
            Some("target.add"),
            AuditContext {
                provider: Some(provider.as_prefix().to_string()),
                scope: Some(scope.segments().join("/")),
                repo_id: Some(id),
                path: None,
            },
            None,
            None,
        )?;
        println!("Audit ID: {audit_id}");
        Ok(())
    })();

    if let Err(err) = &result {
        let _ = audit.record(
            "target.add",
            AuditStatus::Failed,
            Some("target.add"),
            None,
            Some(&err.to_string()),
        );
    }
    result
}

fn handle_list_targets(audit: &AuditLogger) -> anyhow::Result<()> {
    let result: anyhow::Result<()> = (|| {
        let config_path = default_config_path()?;
        let (config, migrated) = load_or_migrate(&config_path)?;
        if migrated {
            config.save(&config_path)?;
        }

        if config.targets.is_empty() {
            println!("No targets configured.");
            return Ok(());
        }

        for target in config.targets {
            let host = target.host.clone().unwrap_or_else(|| "(default)".to_string());
            println!(
                "{} | {} | {} | {}",
                target.id,
                target.provider.as_prefix(),
                target.scope.segments().join("/"),
                host
            );
        }
        Ok(())
    })();

    if let Err(err) = &result {
        let _ = audit.record(
            "target.list",
            AuditStatus::Failed,
            Some("target.list"),
            None,
            Some(&err.to_string()),
        );
    } else {
        let audit_id =
            audit.record("target.list", AuditStatus::Ok, Some("target.list"), None, None)?;
        println!("Audit ID: {audit_id}");
    }
    result
}

fn handle_remove_target(args: RemoveTargetArgs, audit: &AuditLogger) -> anyhow::Result<()> {
    let result: anyhow::Result<()> = (|| {
        let config_path = default_config_path()?;
        let (mut config, migrated) = load_or_migrate(&config_path)?;
        let before = config.targets.len();
        config.targets.retain(|target| target.id != args.id);
        let after = config.targets.len();
        if before == after {
            println!("No target found with id {}", args.id);
            let audit_id = audit.record(
                "target.remove",
                AuditStatus::Skipped,
                Some("target.remove"),
                None,
                Some("target not found"),
            )?;
            println!("Audit ID: {audit_id}");
            return Ok(());
        }
        config.save(&config_path)?;
        if migrated {
            println!("Config migrated and target removed from {}", config_path.display());
        } else {
            println!("Target removed from {}", config_path.display());
        }
        let audit_id = audit.record(
            "target.remove",
            AuditStatus::Ok,
            Some("target.remove"),
            None,
            None,
        )?;
        println!("Audit ID: {audit_id}");
        Ok(())
    })();

    if let Err(err) = &result {
        let _ = audit.record(
            "target.remove",
            AuditStatus::Failed,
            Some("target.remove"),
            None,
            Some(&err.to_string()),
        );
    }
    result
}

fn handle_token(args: TokenArgs, audit: &AuditLogger) -> anyhow::Result<()> {
    match args.command {
        TokenCommands::Set(args) => handle_set_token(args, audit),
        TokenCommands::Guide(args) => handle_guide_token(args, audit),
        TokenCommands::Validate(args) => handle_validate_token(args, audit),
    }
}

fn handle_set_token(args: SetTokenArgs, audit: &AuditLogger) -> anyhow::Result<()> {
    let result: anyhow::Result<()> = (|| {
        let provider: ProviderKind = args.provider.into();
        let spec = spec_for(provider.clone());
        let scope = spec.parse_scope(args.scope)?;
        let host = host_or_default(args.host.as_deref(), spec.as_ref());
        let account = spec.account_key(&host, &scope)?;
        auth::set_pat(&account, &args.token)?;
        println!("Token stored for {account}");
        let audit_id = audit.record_with_context(
            "token.set",
            AuditStatus::Ok,
            Some("token.set"),
            AuditContext {
                provider: Some(provider.as_prefix().to_string()),
                scope: Some(scope.segments().join("/")),
                repo_id: None,
                path: None,
            },
            None,
            None,
        )?;
        println!("Audit ID: {audit_id}");
        Ok(())
    })();

    if let Err(err) = &result {
        let _ = audit.record(
            "token.set",
            AuditStatus::Failed,
            Some("token.set"),
            None,
            Some(&err.to_string()),
        );
    }
    result
}

fn handle_guide_token(args: GuideTokenArgs, audit: &AuditLogger) -> anyhow::Result<()> {
    let result: anyhow::Result<()> = (|| {
        let provider: ProviderKind = args.provider.into();
        let spec = spec_for(provider.clone());
        let scope = spec.parse_scope(args.scope)?;
        let help = mirror_providers::spec::pat_help(provider.clone());
        println!("Provider: {}", provider.as_prefix());
        println!("Scope: {}", scope.segments().join("/"));
        println!("Create PAT at: {}", help.url);
        println!("Required scopes:");
        for scope in help.scopes {
            println!("  - {scope}");
        }
        let audit_id = audit.record_with_context(
            "token.guide",
            AuditStatus::Ok,
            Some("token.guide"),
            AuditContext {
                provider: Some(provider.as_prefix().to_string()),
                scope: Some(scope.segments().join("/")),
                repo_id: None,
                path: None,
            },
            None,
            None,
        )?;
        println!("Audit ID: {audit_id}");
        Ok(())
    })();

    if let Err(err) = &result {
        let _ = audit.record(
            "token.guide",
            AuditStatus::Failed,
            Some("token.guide"),
            None,
            Some(&err.to_string()),
        );
    }
    result
}

fn handle_validate_token(args: ValidateTokenArgs, audit: &AuditLogger) -> anyhow::Result<()> {
    let result: anyhow::Result<()> = (|| {
        let provider: ProviderKind = args.provider.into();
        let spec = spec_for(provider.clone());
        let scope = spec.parse_scope(args.scope)?;
        let host = args.host.as_ref().map(|value| value.trim_end_matches('/').to_string());
        let runtime_target = ProviderTarget {
            provider: provider.clone(),
            scope: scope.clone(),
            host,
        };
        let registry = ProviderRegistry::new();
        let adapter = registry.provider(provider.clone())?;
        let scopes = adapter.token_scopes(&runtime_target)?;
        let help = mirror_providers::spec::pat_help(provider.clone());
        match scopes {
            Some(scopes) => {
                let missing: Vec<&str> = help
                    .scopes
                    .iter()
                    .copied()
                    .filter(|required| !scopes.iter().any(|s| s == required))
                    .collect();
                if missing.is_empty() {
                    println!("Token scopes valid for {}", provider.as_prefix());
                    let audit_id = audit.record_with_context(
                        "token.validate",
                        AuditStatus::Ok,
                        Some("token.validate"),
                        AuditContext {
                            provider: Some(provider.as_prefix().to_string()),
                            scope: Some(scope.segments().join("/")),
                            repo_id: None,
                            path: None,
                        },
                        None,
                        None,
                    )?;
                    println!("Audit ID: {audit_id}");
                } else {
                    println!("Missing scopes:");
                    for scope in missing {
                        println!("  - {scope}");
                    }
                    let audit_id = audit.record_with_context(
                        "token.validate",
                        AuditStatus::Failed,
                        Some("token.validate"),
                        AuditContext {
                            provider: Some(provider.as_prefix().to_string()),
                            scope: Some(scope.segments().join("/")),
                            repo_id: None,
                            path: None,
                        },
                        None,
                        Some("missing scopes"),
                    )?;
                    println!("Audit ID: {audit_id}");
                }
            }
            None => {
                println!("Scope validation not supported for {}", provider.as_prefix());
                let audit_id = audit.record_with_context(
                    "token.validate",
                    AuditStatus::Skipped,
                    Some("token.validate"),
                    AuditContext {
                        provider: Some(provider.as_prefix().to_string()),
                        scope: Some(scope.segments().join("/")),
                        repo_id: None,
                        path: None,
                    },
                    None,
                    Some("validation not supported"),
                )?;
                println!("Audit ID: {audit_id}");
            }
        }
        Ok(())
    })();

    if let Err(err) = &result {
        let _ = audit.record(
            "token.validate",
            AuditStatus::Failed,
            Some("token.validate"),
            None,
            Some(&err.to_string()),
        );
    }
    result
}

fn handle_sync(args: SyncArgs, audit: &AuditLogger) -> anyhow::Result<()> {
    let result: anyhow::Result<()> = (|| {
        if args.non_interactive && args.missing_remote == MissingRemotePolicyValue::Prompt {
            anyhow::bail!("non-interactive mode requires --missing-remote policy");
        }

        let config_path = args.config.unwrap_or(default_config_path()?);
        let cache_path = args.cache.unwrap_or(default_cache_path()?);
        let (config, migrated) = load_or_migrate(&config_path)?;
        if migrated {
            config.save(&config_path)?;
        }
        let root = config
            .root
            .as_ref()
            .context("config missing root; run config init")?;

        let targets = select_targets(&config, args.target_id.as_deref(), args.provider, &args.scope)?;
        if targets.is_empty() {
            println!("No matching targets found.");
            let audit_id = audit.record(
                "sync.run",
                AuditStatus::Skipped,
                Some("sync"),
                None,
                Some("no matching targets"),
            )?;
            println!("Audit ID: {audit_id}");
            return Ok(());
        }

        let policy: MissingRemotePolicy = args.missing_remote.into();
        let decider = if policy == MissingRemotePolicy::Prompt {
            Some(&prompt_action as &dyn Fn(&RepoCacheEntry) -> anyhow::Result<DeletedRepoAction>)
        } else {
            None
        };

        let registry = ProviderRegistry::new();
        let repo_name = args.repo.clone();
        let mut total = SyncSummary::default();

        let target_count = targets.len();
        for target in targets {
            let provider_kind = target.provider.clone();
            let provider = registry.provider(provider_kind.clone())?;
            let runtime_target = ProviderTarget {
                provider: provider_kind,
                scope: target.scope.clone(),
                host: target.host.clone(),
            };
            let repo_filter = repo_name.as_ref().map(|repo| {
                let repo = repo.clone();
                move |remote: &mirror_core::model::RemoteRepo| {
                    remote.name == repo || remote.id == repo
                }
            });

        let include_archived = args.include_archived;
        let summary = if let Some(filter) = repo_filter.as_ref() {
            run_sync_filtered(
                provider.as_ref(),
                &runtime_target,
                root,
                &cache_path,
                policy,
                decider,
                Some(&|repo| {
                    let allowed = include_archived || !repo.archived;
                    allowed && filter(repo)
                }),
                false,
                args.refresh,
                args.verify,
            )
            .or_else(|err| map_azdo_error(&runtime_target, err))?
        } else {
            run_sync_filtered(
                provider.as_ref(),
                &runtime_target,
                root,
                &cache_path,
                policy,
                decider,
                Some(&|repo| include_archived || !repo.archived),
                true,
                args.refresh,
                args.verify,
            )
            .or_else(|err| map_azdo_error(&runtime_target, err))?
        };

            print_summary(&target, summary);
            let details = serde_json::json!({
                "cloned": summary.cloned,
                "fast_forwarded": summary.fast_forwarded,
                "up_to_date": summary.up_to_date,
                "dirty": summary.dirty,
                "diverged": summary.diverged,
                "failed": summary.failed,
                "missing_archived": summary.missing_archived,
                "missing_removed": summary.missing_removed,
                "missing_skipped": summary.missing_skipped,
            });
            let context = AuditContext {
                provider: Some(target.provider.as_prefix().to_string()),
                scope: Some(target.scope.segments().join("/")),
                repo_id: Some(target.id.clone()),
                path: None,
            };
            let audit_id = audit.record_with_context(
                "sync.target",
                AuditStatus::Ok,
                Some("sync"),
                context,
                Some(details),
                None,
            )?;
            println!("Audit ID: {audit_id}");
            accumulate_summary(&mut total, summary);
        }

        if target_count > 1 {
            println!(
                "Total: cloned={} fast_forwarded={} up_to_date={} dirty={} diverged={} failed={} missing_archived={} missing_removed={} missing_skipped={}",
                total.cloned,
                total.fast_forwarded,
                total.up_to_date,
                total.dirty,
                total.diverged,
                total.failed,
                total.missing_archived,
                total.missing_removed,
                total.missing_skipped
            );
        }

        let totals = serde_json::json!({
            "targets": target_count,
            "cloned": total.cloned,
            "fast_forwarded": total.fast_forwarded,
            "up_to_date": total.up_to_date,
            "dirty": total.dirty,
            "diverged": total.diverged,
            "failed": total.failed,
            "missing_archived": total.missing_archived,
            "missing_removed": total.missing_removed,
            "missing_skipped": total.missing_skipped,
        });
        let audit_id =
            audit.record("sync.run", AuditStatus::Ok, Some("sync"), Some(totals), None)?;
        println!("Audit ID: {audit_id}");

        Ok(())
    })();

    if let Err(err) = &result {
        let _ = audit.record(
            "sync.run",
            AuditStatus::Failed,
            Some("sync"),
            None,
            Some(&err.to_string()),
        );
    }
    result
}

fn handle_daemon(args: DaemonArgs, audit: &AuditLogger) -> anyhow::Result<()> {
    let result: anyhow::Result<()> = (|| {
        let interval = std::time::Duration::from_secs(args.interval_seconds);
        let lock_path = match args.lock {
            Some(path) => path,
            None => default_lock_path()?,
        };
        if args.missing_remote == MissingRemotePolicyValue::Prompt {
            anyhow::bail!("daemon mode requires --missing-remote policy");
        }
        let config_path = args.config.unwrap_or(default_config_path()?);
        let cache_path = args.cache.unwrap_or(default_cache_path()?);
        let policy: MissingRemotePolicy = args.missing_remote.into();
        let audit_id = audit.record(
            "daemon.start",
            AuditStatus::Ok,
            Some("daemon"),
            None,
            None,
        )?;
        println!("Audit ID: {audit_id}");
        let job = || run_sync_job(&config_path, &cache_path, policy);
        if args.run_once {
            mirror_core::daemon::run_once_with_lock(&lock_path, job)?;
            let audit_id = audit.record(
                "daemon.run_once",
                AuditStatus::Ok,
                Some("daemon"),
                None,
                None,
            )?;
            println!("Audit ID: {audit_id}");
            return Ok(());
        }
        mirror_core::daemon::run_daemon(&lock_path, interval, job)
    })();

    if let Err(err) = &result {
        let _ = audit.record(
            "daemon.start",
            AuditStatus::Failed,
            Some("daemon"),
            None,
            Some(&err.to_string()),
        );
    }
    result
}

fn handle_service(args: ServiceArgs, audit: &AuditLogger) -> anyhow::Result<()> {
    let result: anyhow::Result<()> = (|| {
        let exe = std::env::current_exe().context("resolve current executable")?;
        match args.action {
            ServiceAction::Install => {
                mirror_core::service::install_service(&exe)?;
                println!("Service installed for {}", exe.display());
                let audit_id = audit.record(
                    "service.install",
                    AuditStatus::Ok,
                    Some("service.install"),
                    None,
                    None,
                )?;
                println!("Audit ID: {audit_id}");
            }
            ServiceAction::Uninstall => {
                mirror_core::service::uninstall_service()?;
                println!("Service uninstalled.");
                let audit_id = audit.record(
                    "service.uninstall",
                    AuditStatus::Ok,
                    Some("service.uninstall"),
                    None,
                    None,
                )?;
                println!("Audit ID: {audit_id}");
            }
        }
        Ok(())
    })();

    if let Err(err) = &result {
        let action = match args.action {
            ServiceAction::Install => "service.install",
            ServiceAction::Uninstall => "service.uninstall",
        };
        let _ = audit.record(
            action,
            AuditStatus::Failed,
            Some(action),
            None,
            Some(&err.to_string()),
        );
    }
    result
}

fn handle_health(args: HealthArgs, audit: &AuditLogger) -> anyhow::Result<()> {
    let result: anyhow::Result<()> = (|| {
        let config_path = args.config.unwrap_or(default_config_path()?);
        let (config, migrated) = load_or_migrate(&config_path)?;
        if migrated {
            config.save(&config_path)?;
        }

        let targets = select_targets(&config, args.target_id.as_deref(), args.provider, &args.scope)?;
        if targets.is_empty() {
            println!("No matching targets found.");
            let audit_id = audit.record(
                "health.check",
                AuditStatus::Skipped,
                Some("health"),
                None,
                Some("no matching targets"),
            )?;
            println!("Audit ID: {audit_id}");
            return Ok(());
        }

        let registry = ProviderRegistry::new();
        for target in targets {
            let provider_kind = target.provider.clone();
            let provider = registry.provider(provider_kind.clone())?;
            let runtime_target = ProviderTarget {
                provider: provider_kind,
                scope: target.scope.clone(),
                host: target.host.clone(),
            };

            let outcome = provider
                .health_check(&runtime_target)
                .or_else(|err| map_provider_error(&runtime_target, err));
            match outcome {
                Ok(()) => {
                    println!(
                        "Health OK: {} {}",
                        target.provider.as_prefix(),
                        target.scope.segments().join("/")
                    );
                    let audit_id = audit.record_with_context(
                        "health.check",
                        AuditStatus::Ok,
                        Some("health"),
                        AuditContext {
                            provider: Some(target.provider.as_prefix().to_string()),
                            scope: Some(target.scope.segments().join("/")),
                            repo_id: Some(target.id.clone()),
                            path: None,
                        },
                        None,
                        None,
                    )?;
                    println!("Audit ID: {audit_id}");
                }
                Err(err) => {
                    eprintln!(
                        "Health FAILED: {} {} -> {err}",
                        target.provider.as_prefix(),
                        target.scope.segments().join("/")
                    );
                    let audit_id = audit.record_with_context(
                        "health.check",
                        AuditStatus::Failed,
                        Some("health"),
                        AuditContext {
                            provider: Some(target.provider.as_prefix().to_string()),
                            scope: Some(target.scope.segments().join("/")),
                            repo_id: Some(target.id.clone()),
                            path: None,
                        },
                        None,
                        Some(&err.to_string()),
                    )?;
                    println!("Audit ID: {audit_id}");
                }
            }
        }
        Ok(())
    })();

    if let Err(err) = &result {
        let _ = audit.record(
            "health.check",
            AuditStatus::Failed,
            Some("health"),
            None,
            Some(&err.to_string()),
        );
    }

    result
}

fn handle_webhook(args: WebhookArgs, audit: &AuditLogger) -> anyhow::Result<()> {
    match args.command {
        WebhookCommands::Register(args) => handle_webhook_register(args, audit),
    }
}

fn handle_webhook_register(args: WebhookRegisterArgs, audit: &AuditLogger) -> anyhow::Result<()> {
    let result: anyhow::Result<()> = (|| {
        let provider: ProviderKind = args.provider.into();
        let spec = spec_for(provider.clone());
        let scope = spec.parse_scope(args.scope)?;
        let host = args.host.as_ref().map(|value| value.trim_end_matches('/').to_string());
        let runtime_target = ProviderTarget {
            provider: provider.clone(),
            scope: scope.clone(),
            host: host.clone(),
        };

        let registry = ProviderRegistry::new();
        let adapter = registry.provider(provider.clone())?;
        adapter
            .register_webhook(&runtime_target, &args.url, args.secret.as_deref())
            .or_else(|err| map_provider_error(&runtime_target, err))?;

        println!(
            "Webhook registered for {} {}",
            provider.as_prefix(),
            scope.segments().join("/")
        );
        let audit_id = audit.record_with_context(
            "webhook.register",
            AuditStatus::Ok,
            Some("webhook.register"),
            AuditContext {
                provider: Some(provider.as_prefix().to_string()),
                scope: Some(scope.segments().join("/")),
                repo_id: None,
                path: None,
            },
            None,
            None,
        )?;
        println!("Audit ID: {audit_id}");
        Ok(())
    })();

    if let Err(err) = &result {
        let _ = audit.record(
            "webhook.register",
            AuditStatus::Failed,
            Some("webhook.register"),
            None,
            Some(&err.to_string()),
        );
    }
    result
}

fn handle_cache(args: CacheArgs, audit: &AuditLogger) -> anyhow::Result<()> {
    match args.command {
        CacheCommands::Prune(args) => handle_cache_prune(args, audit),
    }
}

fn handle_oauth(args: OauthArgs, audit: &AuditLogger) -> anyhow::Result<()> {
    match args.command {
        OauthCommands::Device(args) => handle_device_flow(args, audit),
        OauthCommands::Revoke(args) => handle_revoke_oauth(args, audit),
    }
}

fn handle_device_flow(args: DeviceFlowArgs, audit: &AuditLogger) -> anyhow::Result<()> {
    let result: anyhow::Result<()> = (|| {
        let provider: ProviderKind = args.provider.into();
        let spec = spec_for(provider.clone());
        let scope = spec.parse_scope(args.scope)?;
        let host = host_or_default(args.host.as_deref(), spec.as_ref());
        let oauth_host = oauth_gate_host(&provider, &host);
        if !auth::oauth_allowed(provider.as_prefix(), &oauth_host) {
            let message = format!(
                "OAuth not enabled for {} at {}. Set {} to override.",
                provider.as_prefix(),
                oauth_host,
                "GIT_PROJECT_SYNC_OAUTH_ALLOW"
            );
            let _ = audit.record_with_context(
                "oauth.device.start",
                AuditStatus::Failed,
                Some("oauth.device"),
                AuditContext {
                    provider: Some(provider.as_prefix().to_string()),
                    scope: Some(scope.segments().join("/")),
                    repo_id: None,
                    path: None,
                },
                None,
                Some(&message),
            );
            anyhow::bail!(message);
        }
        let start_audit = audit.record_with_context(
            "oauth.device.start",
            AuditStatus::Ok,
            Some("oauth.device"),
            AuditContext {
                provider: Some(provider.as_prefix().to_string()),
                scope: Some(scope.segments().join("/")),
                repo_id: None,
                path: None,
            },
            None,
            None,
        )?;
        println!("Audit ID: {start_audit}");
        let client = Client::new();
        let account = spec.account_key(&host, &scope)?;
        match provider {
            ProviderKind::GitHub => {
                let base = github_oauth_base(&host);
                let default_scopes = pat_help(provider.clone()).scopes.join(" ");
                let scope_string = if args.oauth_scope.is_empty() {
                    default_scopes
                } else {
                    args.oauth_scope.join(" ")
                };
                println!("Requested OAuth scopes: {scope_string}");
                let device_resp: DeviceCodeResponse = client
                    .post(format!("{base}/login/device/code"))
                    .header("Accept", "application/json")
                    .form(&[
                        ("client_id", args.client_id.as_str()),
                        ("scope", scope_string.as_str()),
                    ])
                    .send()?
                    .error_for_status()?
                    .json()?;

                println!("Open: {}", device_resp.verification_uri);
                println!("Code: {}", device_resp.user_code);
                println!("Expires in: {}s", device_resp.expires_in);
                println!("Polling for authorization...");

                let mut interval = device_resp.interval.unwrap_or(5);
                let token_response = poll_device_token(
                    &client,
                    &format!("{base}/login/oauth/access_token"),
                    &args.client_id,
                    &device_resp.device_code,
                    &mut interval,
                    None,
                )?;
                let token = extract_access_token(&token_response)?;

                auth::set_pat(&account, &token)?;
                println!("Token stored for {account}");
                let _ = audit.record_with_context(
                    "oauth.device.approved",
                    AuditStatus::Ok,
                    Some("oauth.device"),
                    AuditContext {
                        provider: Some(provider.as_prefix().to_string()),
                        scope: Some(scope.segments().join("/")),
                        repo_id: None,
                        path: None,
                    },
                    None,
                    None,
                );
            }
            ProviderKind::AzureDevOps => {
                let tenant = args.tenant.as_deref().unwrap_or("common");
                let oauth_scope = if args.oauth_scope.is_empty() {
                    AZDO_DEFAULT_OAUTH_SCOPE.to_string()
                } else {
                    args.oauth_scope.join(" ")
                };
                println!("Tenant: {tenant}");
                println!("Requested OAuth scopes: {oauth_scope}");
                let device_endpoint = AzureDevOpsProvider::oauth_device_code_endpoint(tenant);
                let token_endpoint = AzureDevOpsProvider::oauth_token_endpoint(tenant);
                let device_resp: DeviceCodeResponse = client
                    .post(&device_endpoint)
                    .header("Accept", "application/json")
                    .form(&[
                        ("client_id", args.client_id.as_str()),
                        ("scope", oauth_scope.as_str()),
                    ])
                    .send()?
                    .error_for_status()?
                    .json()?;

                if let Some(message) = device_resp.message.as_deref() {
                    println!("{message}");
                }
                let verification = device_resp
                    .verification_uri_complete
                    .as_deref()
                    .unwrap_or(device_resp.verification_uri.as_str());
                println!("Open: {verification}");
                println!("Code: {}", device_resp.user_code);
                println!("Expires in: {}s", device_resp.expires_in);
                println!("Polling for authorization...");

                let mut interval = device_resp.interval.unwrap_or(5);
                let token_response = poll_device_token(
                    &client,
                    &token_endpoint,
                    &args.client_id,
                    &device_resp.device_code,
                    &mut interval,
                    Some(oauth_scope.as_str()),
                )?;
                let token = extract_access_token(&token_response)?;
                let expires_at = token_response
                    .expires_in
                    .map(|secs| current_epoch_seconds() + secs as i64);
                auth::set_oauth_token(
                    &account,
                    auth::OAuthToken {
                        access_token: token,
                        refresh_token: token_response.refresh_token.clone(),
                        expires_at,
                        token_endpoint: token_endpoint.clone(),
                        revocation_endpoint: None,
                        client_id: args.client_id.clone(),
                        scope: Some(oauth_scope),
                    },
                )?;
                println!("OAuth token stored for {account}");
                let _ = audit.record_with_context(
                    "oauth.device.approved",
                    AuditStatus::Ok,
                    Some("oauth.device"),
                    AuditContext {
                        provider: Some(provider.as_prefix().to_string()),
                        scope: Some(scope.segments().join("/")),
                        repo_id: None,
                        path: None,
                    },
                    None,
                    None,
                );
            }
            ProviderKind::GitLab => {
                anyhow::bail!("device flow is not supported for GitLab yet");
            }
        }

        let audit_id = audit.record_with_context(
            "oauth.device",
            AuditStatus::Ok,
            Some("oauth.device"),
            AuditContext {
                provider: Some(provider.as_prefix().to_string()),
                scope: Some(scope.segments().join("/")),
                repo_id: None,
                path: None,
            },
            None,
            None,
        )?;
        println!("Audit ID: {audit_id}");
        Ok(())
    })();

    if let Err(err) = &result {
        let _ = audit.record(
            "oauth.device",
            AuditStatus::Failed,
            Some("oauth.device"),
            None,
            Some(&err.to_string()),
        );
    }
    result
}

fn handle_revoke_oauth(args: RevokeOauthArgs, audit: &AuditLogger) -> anyhow::Result<()> {
    let result: anyhow::Result<()> = (|| {
        let provider: ProviderKind = args.provider.into();
        let spec = spec_for(provider.clone());
        let scope = spec.parse_scope(args.scope)?;
        let host = host_or_default(args.host.as_deref(), spec.as_ref());
        let account = spec.account_key(&host, &scope)?;
        auth::revoke_oauth_token(&account)?;
        println!("OAuth token revoked for {account}");
        let audit_id = audit.record_with_context(
            "oauth.revoke",
            AuditStatus::Ok,
            Some("oauth.revoke"),
            AuditContext {
                provider: Some(provider.as_prefix().to_string()),
                scope: Some(scope.segments().join("/")),
                repo_id: None,
                path: None,
            },
            None,
            None,
        )?;
        println!("Audit ID: {audit_id}");
        Ok(())
    })();

    if let Err(err) = &result {
        let _ = audit.record(
            "oauth.revoke",
            AuditStatus::Failed,
            Some("oauth.revoke"),
            None,
            Some(&err.to_string()),
        );
    }
    result
}

#[derive(Deserialize)]
struct DeviceCodeResponse {
    device_code: String,
    user_code: String,
    verification_uri: String,
    expires_in: u64,
    interval: Option<u64>,
    #[serde(default)]
    verification_uri_complete: Option<String>,
    #[serde(default)]
    message: Option<String>,
}

#[derive(Deserialize)]
struct DeviceTokenResponse {
    access_token: Option<String>,
    refresh_token: Option<String>,
    expires_in: Option<u64>,
    error: Option<String>,
    error_description: Option<String>,
}

fn poll_device_token(
    client: &Client,
    token_endpoint: &str,
    client_id: &str,
    device_code: &str,
    interval: &mut u64,
    scope: Option<&str>,
) -> anyhow::Result<DeviceTokenResponse> {
    loop {
        std::thread::sleep(std::time::Duration::from_secs(*interval));
        let mut form = vec![
            ("client_id", client_id.to_string()),
            ("device_code", device_code.to_string()),
            (
                "grant_type",
                "urn:ietf:params:oauth:grant-type:device_code".to_string(),
            ),
        ];
        if let Some(scope) = scope {
            form.push(("scope", scope.to_string()));
        }
        let response: DeviceTokenResponse = client
            .post(token_endpoint)
            .header("Accept", "application/json")
            .form(&form)
            .send()?
            .error_for_status()?
            .json()?;
        if response.access_token.is_some() {
            return Ok(response);
        }
        match response.error.as_deref() {
            Some("authorization_pending") => continue,
            Some("slow_down") => {
                *interval += 5;
            }
            Some("expired_token") => anyhow::bail!("device code expired"),
            Some("access_denied") => anyhow::bail!("access denied"),
            Some(other) => anyhow::bail!(
                "device flow failed: {}",
                response.error_description.unwrap_or_else(|| other.to_string())
            ),
            None => anyhow::bail!("device flow failed without error"),
        }
    }
}

fn github_oauth_base(host: &str) -> String {
    if host.contains("github.com") {
        "https://github.com".to_string()
    } else if let Some(stripped) = host.strip_suffix("/api/v3") {
        stripped.to_string()
    } else if let Some(stripped) = host.strip_suffix("/api") {
        stripped.to_string()
    } else {
        host.trim_end_matches('/').to_string()
    }
}

fn oauth_gate_host(provider: &ProviderKind, host: &str) -> String {
    match provider {
        ProviderKind::GitHub => github_oauth_base(host),
        _ => host.to_string(),
    }
}

fn extract_access_token(response: &DeviceTokenResponse) -> anyhow::Result<String> {
    response
        .access_token
        .clone()
        .ok_or_else(|| anyhow::anyhow!("device flow did not return access token"))
}

fn current_epoch_seconds() -> i64 {
    use std::time::{SystemTime, UNIX_EPOCH};
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap_or_default()
        .as_secs() as i64
}

fn handle_cache_prune(args: CachePruneArgs, audit: &AuditLogger) -> anyhow::Result<()> {
    let result: anyhow::Result<()> = (|| {
        let config_path = args.config.unwrap_or(default_config_path()?);
        let cache_path = args.cache.unwrap_or(default_cache_path()?);
        let (config, migrated) = load_or_migrate(&config_path)?;
        if migrated {
            config.save(&config_path)?;
        }
        let target_ids: Vec<String> = config.targets.iter().map(|t| t.id.clone()).collect();
        let removed = mirror_core::cache::prune_cache_for_targets(&cache_path, &target_ids)?;
        println!("Pruned {removed} cache entries.");
        let audit_id = audit.record(
            "cache.prune",
            AuditStatus::Ok,
            Some("cache.prune"),
            None,
            None,
        )?;
        println!("Audit ID: {audit_id}");
        Ok(())
    })();

    if let Err(err) = &result {
        let _ = audit.record(
            "cache.prune",
            AuditStatus::Failed,
            Some("cache.prune"),
            None,
            Some(&err.to_string()),
        );
    }

    result
}

fn run_sync_job(
    config_path: &Path,
    cache_path: &Path,
    policy: MissingRemotePolicy,
) -> anyhow::Result<()> {
    let (config, migrated) = load_or_migrate(config_path)?;
    if migrated {
        config.save(config_path)?;
    }
    let root = config
        .root
        .as_ref()
        .context("config missing root; run config init")?;
    let registry = ProviderRegistry::new();
    let day_bucket = current_day_bucket();
    let now = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .unwrap_or_default()
        .as_secs();
    let cache_snapshot = mirror_core::cache::RepoCache::load(cache_path).unwrap_or_default();
    let mut had_failure = false;
    for target in config.targets {
        let target_key = target_id(
            target.provider.clone(),
            target.host.as_deref(),
            &target.scope,
        );
        if let Some(until) = backoff_until(&cache_snapshot, &target_key) {
            if until > now {
                warn!(
                    provider = %target.provider,
                    scope = ?target.scope,
                    until = until,
                    "skipping target due to backoff"
                );
                continue;
            }
        }
        let provider_kind = target.provider.clone();
        let provider = registry.provider(provider_kind.clone())?;
        let runtime_target = ProviderTarget {
            provider: provider_kind,
            scope: target.scope.clone(),
            host: target.host.clone(),
        };
        let bucketed = |repo: &mirror_core::model::RemoteRepo| {
            !repo.archived && bucket_for_repo_id(&repo.id) == day_bucket
        };
        let result = run_sync_filtered(
            provider.as_ref(),
            &runtime_target,
            root,
            cache_path,
            policy,
            None,
            Some(&bucketed),
            true,
            false,
            false,
        )
        .or_else(|err| map_azdo_error(&runtime_target, err));
        match result {
            Ok(_) => {
                let _ = update_target_success(cache_path, &target_key, now);
            }
            Err(err) => {
                had_failure = true;
                let _ = update_target_failure(cache_path, &target_key, now);
                warn!(
                    provider = %runtime_target.provider,
                    scope = ?runtime_target.scope,
                    error = %err,
                    "target sync failed"
                );
            }
        }
    }
    if had_failure {
        anyhow::bail!("one or more targets failed");
    }
    Ok(())
}

fn select_targets(
    config: &AppConfigV2,
    target_id: Option<&str>,
    provider: Option<ProviderKindValue>,
    scope: &[String],
) -> anyhow::Result<Vec<TargetConfig>> {
    let mut targets = config.targets.clone();

    if let Some(target_id) = target_id {
        targets.retain(|target| target.id == target_id);
        return Ok(targets);
    }

    let provider_kind = provider.map(ProviderKind::from);

    if let Some(provider_kind) = provider_kind.as_ref() {
        targets.retain(|target| target.provider == *provider_kind);
    } else if !scope.is_empty() {
        anyhow::bail!("--scope requires --provider when selecting targets");
    }

    if !scope.is_empty() {
        let provider_kind = provider_kind.clone().context("provider required")?;
        let spec = spec_for(provider_kind);
        let scope = spec.parse_scope(scope.to_vec())?;
        targets.retain(|target| target.scope == scope);
    }

    Ok(targets)
}

fn prompt_action(entry: &RepoCacheEntry) -> anyhow::Result<DeletedRepoAction> {
    println!(
        "Remote repo missing: {} (path: {}). Choose [a]rchive / [r]emove / [s]kip:",
        entry.name, entry.path
    );
    loop {
        print!("> ");
        io::stdout().flush().ok();
        let mut input = String::new();
        io::stdin().read_line(&mut input)?;
        match input.trim().to_lowercase().as_str() {
            "a" | "archive" => return Ok(DeletedRepoAction::Archive),
            "r" | "remove" => return Ok(DeletedRepoAction::Remove),
            "s" | "skip" => return Ok(DeletedRepoAction::Skip),
            _ => println!("Please enter a, r, or s."),
        }
    }
}

fn print_summary(target: &TargetConfig, summary: SyncSummary) {
    println!(
        "Target {}: cloned={} fast_forwarded={} up_to_date={} dirty={} diverged={} failed={} missing_archived={} missing_removed={} missing_skipped={}",
        target.id,
        summary.cloned,
        summary.fast_forwarded,
        summary.up_to_date,
        summary.dirty,
        summary.diverged,
        summary.failed,
        summary.missing_archived,
        summary.missing_removed,
        summary.missing_skipped
    );
}

fn accumulate_summary(total: &mut SyncSummary, summary: SyncSummary) {
    total.cloned += summary.cloned;
    total.fast_forwarded += summary.fast_forwarded;
    total.up_to_date += summary.up_to_date;
    total.dirty += summary.dirty;
    total.diverged += summary.diverged;
    total.failed += summary.failed;
    total.missing_archived += summary.missing_archived;
    total.missing_removed += summary.missing_removed;
    total.missing_skipped += summary.missing_skipped;
}

fn map_azdo_error(
    target: &ProviderTarget,
    err: anyhow::Error,
) -> anyhow::Result<SyncSummary> {
    if target.provider == ProviderKind::AzureDevOps {
        if let Some(reqwest_err) = err.downcast_ref::<reqwest::Error>() {
            if let Some(status) = reqwest_err.status() {
                if let Some(message) = azdo_message_for_status(target, status) {
                    return Err(anyhow::anyhow!(message));
                }
            }
        }
    }
    Err(err)
}

fn map_provider_error(
    target: &ProviderTarget,
    err: anyhow::Error,
) -> anyhow::Result<()> {
    if let Some(reqwest_err) = err.downcast_ref::<reqwest::Error>() {
        if let Some(status) = reqwest_err.status() {
            let scope = target.scope.segments().join("/");
            let message = match target.provider {
                ProviderKind::AzureDevOps => azdo_status_message(&scope, status),
                ProviderKind::GitHub => github_status_message(&scope, status),
                ProviderKind::GitLab => gitlab_status_message(&scope, status),
            };
            if let Some(message) = message {
                return Err(anyhow::anyhow!(message));
            }
        }
    }
    Err(err)
}

fn azdo_message_for_status(
    target: &ProviderTarget,
    status: StatusCode,
) -> Option<String> {
    let scope = target.scope.segments().join("/");
    azdo_status_message(&scope, status)
}

fn azdo_status_message(scope: &str, status: StatusCode) -> Option<String> {
    match status {
        StatusCode::UNAUTHORIZED | StatusCode::FORBIDDEN => Some(format!(
            "Azure DevOps authentication failed for scope {scope} (HTTP {status}). Check your PAT.",
        )),
        StatusCode::NOT_FOUND => Some(format!(
            "Azure DevOps scope not found: {scope} (HTTP {status}). Check org/project.",
        )),
        _ => None,
    }
}

fn github_status_message(scope: &str, status: StatusCode) -> Option<String> {
    match status {
        StatusCode::UNAUTHORIZED | StatusCode::FORBIDDEN => Some(format!(
            "GitHub authentication failed for scope {scope} (HTTP {status}). Check your token and org access.",
        )),
        StatusCode::NOT_FOUND => Some(format!(
            "GitHub scope not found: {scope} (HTTP {status}). Check org/user.",
        )),
        _ => None,
    }
}

fn gitlab_status_message(scope: &str, status: StatusCode) -> Option<String> {
    match status {
        StatusCode::UNAUTHORIZED | StatusCode::FORBIDDEN => Some(format!(
            "GitLab authentication failed for scope {scope} (HTTP {status}). Check your token and group access.",
        )),
        StatusCode::NOT_FOUND => Some(format!(
            "GitLab scope not found: {scope} (HTTP {status}). Check group path.",
        )),
        _ => None,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn azdo_message_for_auth_errors() {
        let scope = ProviderScope::new(vec!["org".into()]).unwrap();
        let target = ProviderTarget {
            provider: ProviderKind::AzureDevOps,
            scope,
            host: None,
        };
        let message = azdo_message_for_status(&target, StatusCode::UNAUTHORIZED).unwrap();
        assert!(message.contains("authentication failed"));
        let message = azdo_message_for_status(&target, StatusCode::FORBIDDEN).unwrap();
        assert!(message.contains("authentication failed"));
    }

    #[test]
    fn azdo_message_for_not_found() {
        let scope = ProviderScope::new(vec!["org".into(), "proj".into()]).unwrap();
        let target = ProviderTarget {
            provider: ProviderKind::AzureDevOps,
            scope,
            host: None,
        };
        let message = azdo_message_for_status(&target, StatusCode::NOT_FOUND).unwrap();
        assert!(message.contains("scope not found"));
        assert!(message.contains("org/proj"));
    }

    #[test]
    fn github_status_messages() {
        let scope = "org";
        let message = github_status_message(scope, StatusCode::UNAUTHORIZED).unwrap();
        assert!(message.contains("GitHub authentication failed"));
        let message = github_status_message(scope, StatusCode::NOT_FOUND).unwrap();
        assert!(message.contains("scope not found"));
    }

    #[test]
    fn gitlab_status_messages() {
        let scope = "group";
        let message = gitlab_status_message(scope, StatusCode::FORBIDDEN).unwrap();
        assert!(message.contains("GitLab authentication failed"));
        let message = gitlab_status_message(scope, StatusCode::NOT_FOUND).unwrap();
        assert!(message.contains("scope not found"));
    }

    #[test]
    fn github_oauth_base_for_enterprise() {
        let host = "https://github.example.com/api/v3";
        assert_eq!(github_oauth_base(host), "https://github.example.com");
    }

    #[test]
    fn extract_access_token_happy_path() {
        let response = DeviceTokenResponse {
            access_token: Some("token-123".to_string()),
            refresh_token: None,
            expires_in: None,
            error: None,
            error_description: None,
        };
        assert_eq!(extract_access_token(&response).unwrap(), "token-123");
    }
}

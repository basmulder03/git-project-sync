use anyhow::Context;
use clap::{Parser, ValueEnum};
use mirror_core::cache::RepoCacheEntry;
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
use mirror_providers::spec::{host_or_default, spec_for};
use mirror_providers::ProviderRegistry;
use reqwest::StatusCode;
use std::io::{self, Write};
use std::path::{Path, PathBuf};
use tracing_subscriber::EnvFilter;
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
    #[command(about = "Launch terminal UI")]
    Tui,
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

    let cli = Cli::parse();
    let result = match cli.command {
        Commands::Config(args) => handle_config(args, &audit),
        Commands::Target(args) => handle_target(args, &audit),
        Commands::Token(args) => handle_token(args, &audit),
        Commands::Sync(args) => handle_sync(args, &audit),
        Commands::Daemon(args) => handle_daemon(args, &audit),
        Commands::Service(args) => handle_service(args, &audit),
        Commands::Health(args) => handle_health(args, &audit),
        Commands::Tui => tui::run_tui(&audit),
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
    let result = (|| {
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
    let result = (|| {
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
            id,
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
    let result = (|| {
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
    let result = (|| {
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
    }
}

fn handle_set_token(args: SetTokenArgs, audit: &AuditLogger) -> anyhow::Result<()> {
    let result = (|| {
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

fn handle_sync(args: SyncArgs, audit: &AuditLogger) -> anyhow::Result<()> {
    let result = (|| {
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
    let result = (|| {
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
    let result = (|| {
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
    let result = (|| {
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
    for target in config.targets {
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
        let _ = run_sync_filtered(
            provider.as_ref(),
            &runtime_target,
            root,
            cache_path,
            policy,
            None,
            Some(&bucketed),
            true,
            false,
        )
        .or_else(|err| map_azdo_error(&runtime_target, err))?;
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
}

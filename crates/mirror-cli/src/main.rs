use anyhow::Context;
use clap::{Parser, ValueEnum};
use mirror_core::cache::RepoCacheEntry;
use mirror_core::config::{
    AppConfigV2, TargetConfig, default_cache_path, default_config_path, default_lock_path,
    load_or_migrate, target_id,
};
use mirror_core::deleted::{DeletedRepoAction, MissingRemotePolicy};
use mirror_core::model::{ProviderKind, ProviderTarget};
use mirror_core::scheduler::{bucket_for_repo_id, current_day_bucket};
use mirror_core::sync_engine::{SyncSummary, run_sync_filtered};
use mirror_providers::auth;
use mirror_providers::spec::{host_or_default, spec_for};
use mirror_providers::ProviderRegistry;
use std::io::{self, Write};
use std::path::{Path, PathBuf};
use tracing_subscriber::EnvFilter;

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

    let cli = Cli::parse();
    match cli.command {
        Commands::Config(args) => handle_config(args),
        Commands::Target(args) => handle_target(args),
        Commands::Token(args) => handle_token(args),
        Commands::Sync(args) => handle_sync(args),
        Commands::Daemon(args) => handle_daemon(args),
        Commands::Service(args) => handle_service(args),
    }
}

fn handle_config(args: ConfigArgs) -> anyhow::Result<()> {
    match args.command {
        ConfigCommands::Init(args) => handle_init(args),
    }
}

fn handle_init(args: InitArgs) -> anyhow::Result<()> {
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
}

fn handle_target(args: TargetArgs) -> anyhow::Result<()> {
    match args.command {
        TargetCommands::Add(args) => handle_add_target(args),
        TargetCommands::List => handle_list_targets(),
        TargetCommands::Remove(args) => handle_remove_target(args),
    }
}

fn handle_add_target(args: AddTargetArgs) -> anyhow::Result<()> {
    let config_path = default_config_path()?;
    let (mut config, migrated) = load_or_migrate(&config_path)?;
    let provider: ProviderKind = args.provider.into();
    let spec = spec_for(provider);
    let scope = spec.parse_scope(args.scope)?;

    let host = args.host.as_ref().map(|value| value.trim_end_matches('/').to_string());
    let id = target_id(provider, host.as_deref(), &scope);

    if config.targets.iter().any(|target| target.id == id) {
        println!("Target already exists: {id}");
        return Ok(());
    }

    let target = TargetConfig {
        id,
        provider,
        scope,
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
    Ok(())
}

fn handle_list_targets() -> anyhow::Result<()> {
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
}

fn handle_remove_target(args: RemoveTargetArgs) -> anyhow::Result<()> {
    let config_path = default_config_path()?;
    let (mut config, migrated) = load_or_migrate(&config_path)?;
    let before = config.targets.len();
    config.targets.retain(|target| target.id != args.id);
    let after = config.targets.len();
    if before == after {
        println!("No target found with id {}", args.id);
        return Ok(());
    }
    config.save(&config_path)?;
    if migrated {
        println!("Config migrated and target removed from {}", config_path.display());
    } else {
        println!("Target removed from {}", config_path.display());
    }
    Ok(())
}

fn handle_token(args: TokenArgs) -> anyhow::Result<()> {
    match args.command {
        TokenCommands::Set(args) => handle_set_token(args),
    }
}

fn handle_set_token(args: SetTokenArgs) -> anyhow::Result<()> {
    let provider: ProviderKind = args.provider.into();
    let spec = spec_for(provider);
    let scope = spec.parse_scope(args.scope)?;
    let host = host_or_default(args.host.as_deref(), spec.as_ref());
    let account = spec.account_key(&host, &scope)?;
    auth::set_pat(&account, &args.token)?;
    println!("Token stored for {account}");
    Ok(())
}

fn handle_sync(args: SyncArgs) -> anyhow::Result<()> {
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
        let provider = registry.provider(target.provider)?;
        let runtime_target = ProviderTarget {
            provider: target.provider,
            scope: target.scope.clone(),
            host: target.host.clone(),
        };
        let repo_filter = repo_name.as_ref().map(|repo| {
            let repo = repo.clone();
            move |remote: &mirror_core::model::RemoteRepo| {
                remote.name == repo || remote.id == repo
            }
        });

        let summary = if let Some(filter) = repo_filter.as_ref() {
            run_sync_filtered(
                provider.as_ref(),
                &runtime_target,
                root,
                &cache_path,
                policy,
                decider,
                Some(filter),
                false,
            )?
        } else {
            run_sync_filtered(
                provider.as_ref(),
                &runtime_target,
                root,
                &cache_path,
                policy,
                decider,
                None,
                true,
            )?
        };

        print_summary(&target, summary);
        accumulate_summary(&mut total, summary);
    }

    if target_count > 1 {
        println!(
            "Total: cloned={} fast_forwarded={} up_to_date={} dirty={} diverged={} failed={} ",
            total.cloned,
            total.fast_forwarded,
            total.up_to_date,
            total.dirty,
            total.diverged,
            total.failed
        );
    }

    Ok(())
}

fn handle_daemon(args: DaemonArgs) -> anyhow::Result<()> {
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
    let job = || run_sync_job(&config_path, &cache_path, policy);
    if args.run_once {
        mirror_core::daemon::run_once_with_lock(&lock_path, job)?;
        return Ok(());
    }
    mirror_core::daemon::run_daemon(&lock_path, interval, job)
}

fn handle_service(args: ServiceArgs) -> anyhow::Result<()> {
    match args.action {
        ServiceAction::Install => {
            println!("Service install not implemented yet.");
        }
        ServiceAction::Uninstall => {
            println!("Service uninstall not implemented yet.");
        }
    }
    Ok(())
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
        let provider = registry.provider(target.provider)?;
        let runtime_target = ProviderTarget {
            provider: target.provider,
            scope: target.scope.clone(),
            host: target.host.clone(),
        };
        let bucketed = |repo: &mirror_core::model::RemoteRepo| {
            bucket_for_repo_id(&repo.id) == day_bucket
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
        )?;
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

    if let Some(provider_kind) = provider_kind {
        targets.retain(|target| target.provider == provider_kind);
    } else if !scope.is_empty() {
        anyhow::bail!("--scope requires --provider when selecting targets");
    }

    if !scope.is_empty() {
        let provider_kind = provider_kind.context("provider required")?;
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
        "Target {}: cloned={} fast_forwarded={} up_to_date={} dirty={} diverged={} failed={}",
        target.id,
        summary.cloned,
        summary.fast_forwarded,
        summary.up_to_date,
        summary.dirty,
        summary.diverged,
        summary.failed
    );
}

fn accumulate_summary(total: &mut SyncSummary, summary: SyncSummary) {
    total.cloned += summary.cloned;
    total.fast_forwarded += summary.fast_forwarded;
    total.up_to_date += summary.up_to_date;
    total.dirty += summary.dirty;
    total.diverged += summary.diverged;
    total.failed += summary.failed;
}

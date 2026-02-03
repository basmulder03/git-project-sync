use anyhow::Context;
use clap::{Parser, ValueEnum};
use mirror_core::archive::{archive_repo, remove_repo};
use mirror_core::cache::{RepoCache, RepoCacheEntry};
use mirror_core::config::{AppConfig, default_cache_path, default_config_path, default_lock_path};
use mirror_core::deleted::{DeletedRepoAction, MissingRemotePolicy, detect_deleted_repos};
use mirror_core::model::{ProviderKind, ProviderScope, ProviderTarget};
use mirror_core::sync_engine::run_sync_filtered;
use mirror_providers::auth;
use mirror_providers::azure_devops::AzureDevOpsProvider;
use mirror_providers::github::GitHubProvider;
use mirror_providers::gitlab::GitLabProvider;
use mirror_providers::RepoProvider;
use std::collections::HashSet;
use std::fs;
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
    #[command(about = "Initialize config with a mirror root path")]
    Init(InitArgs),
    #[command(about = "Add a provider target to the config")]
    AddTarget(AddTargetArgs),
    #[command(about = "Store an auth token for a provider target")]
    SetToken(SetTokenArgs),
    #[command(about = "Sync all targets, a single target, or a single repo")]
    Sync(SyncArgs),
    #[command(about = "Handle missing-remote repos using cache and a current repo id list")]
    MissingRemote(MissingRemoteArgs),
    #[command(about = "Run the daemon loop (placeholder sync job)")]
    Daemon(DaemonArgs),
    #[command(about = "Install or uninstall background service helpers (placeholder)")]
    Service(ServiceArgs),
}

#[derive(Parser)]
struct InitArgs {
    #[arg(long)]
    root: PathBuf,
}

#[derive(Parser)]
struct AddTargetArgs {
    #[arg(long, value_enum)]
    provider: ProviderKindValue,
    #[arg(long, required = true)]
    scope: Vec<String>,
    #[arg(long)]
    host: Option<String>,
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
struct MissingRemoteArgs {
    #[arg(long)]
    cache: PathBuf,
    #[arg(long)]
    root: PathBuf,
    #[arg(
        long,
        help = "Path to newline-delimited repo ids from the remote provider"
    )]
    current: PathBuf,
    #[arg(long)]
    non_interactive: bool,
    #[arg(long, value_enum, default_value = "prompt")]
    missing_remote: MissingRemotePolicyValue,
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

#[derive(Clone, Copy, ValueEnum)]
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
        Commands::Init(args) => handle_init(args),
        Commands::AddTarget(args) => handle_add_target(args),
        Commands::SetToken(args) => handle_set_token(args),
        Commands::Sync(args) => handle_sync(args),
        Commands::MissingRemote(args) => handle_missing_remote(args),
        Commands::Daemon(args) => handle_daemon(args),
        Commands::Service(args) => handle_service(args),
    }
}

fn handle_init(args: InitArgs) -> anyhow::Result<()> {
    let config_path = default_config_path()?;
    let mut config = AppConfig::load(&config_path)?;
    config.root = Some(args.root);
    config.save(&config_path)?;
    println!("Config saved to {}", config_path.display());
    Ok(())
}

fn handle_add_target(args: AddTargetArgs) -> anyhow::Result<()> {
    let config_path = default_config_path()?;
    let mut config = AppConfig::load(&config_path)?;

    let scope = ProviderScope::new(args.scope)?;
    let target = ProviderTarget {
        provider: args.provider.into(),
        scope,
        host: args.host,
    };

    if config.targets.contains(&target) {
        println!("Target already exists.");
        return Ok(());
    }

    config.targets.push(target);
    config.save(&config_path)?;
    println!("Target added to {}", config_path.display());
    Ok(())
}

fn handle_set_token(args: SetTokenArgs) -> anyhow::Result<()> {
    let provider: ProviderKind = args.provider.into();
    let scope = ProviderScope::new(args.scope)?;
    let host = args.host.unwrap_or_else(|| default_host(provider).to_string());
    let account = account_key(provider, &host, &scope)?;
    auth::set_pat(&account, &args.token)?;
    println!("Token stored for {}", account);
    Ok(())
}

fn handle_sync(args: SyncArgs) -> anyhow::Result<()> {
    let config_path = args.config.unwrap_or(default_config_path()?);
    let cache_path = args.cache.unwrap_or(default_cache_path()?);
    let config = AppConfig::load(&config_path)?;
    let root = config
        .root
        .as_ref()
        .context("config missing root; run init")?;

    if args.non_interactive && args.missing_remote == MissingRemotePolicyValue::Prompt {
        anyhow::bail!("non-interactive mode requires --missing-remote policy");
    }

    let targets = select_targets(&config, args.provider, &args.scope)?;
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

    let repo_name = args.repo.clone();
    for target in targets {
        let provider = provider_for(target.provider)?;
        let repo_filter = repo_name.as_ref().map(|repo| {
            let repo = repo.clone();
            move |remote: &mirror_core::model::RemoteRepo| {
                remote.name == repo || remote.id == repo
            }
        });
        if let Some(filter) = repo_filter.as_ref() {
            run_sync_filtered(
                provider.as_ref(),
                &target,
                root,
                &cache_path,
                policy,
                decider,
                Some(filter),
            )?;
        } else {
            run_sync_filtered(
                provider.as_ref(),
                &target,
                root,
                &cache_path,
                policy,
                decider,
                None,
            )?;
        }
    }

    Ok(())
}

fn handle_missing_remote(args: MissingRemoteArgs) -> anyhow::Result<()> {
    let cache = RepoCache::load(&args.cache).context("load cache")?;
    let current_ids = load_current_ids(&args.current)?;
    let missing = detect_deleted_repos(&cache, &current_ids);
    if missing.is_empty() {
        println!("No missing remote repos detected.");
        return Ok(());
    }

    let policy: MissingRemotePolicy = args.missing_remote.into();
    if args.non_interactive && policy == MissingRemotePolicy::Prompt {
        anyhow::bail!("non-interactive mode requires --missing-remote policy");
    }

    for missing_repo in missing {
        let entry = missing_repo.entry;
        let action = decide_action(entry, policy, args.non_interactive)?;
        match action {
            DeletedRepoAction::Archive => {
                let destination = archive_repo(
                    &args.root,
                    entry.provider.clone(),
                    &entry.scope,
                    &entry.name,
                )?;
                println!("Archived {} -> {}", entry.name, destination.display());
            }
            DeletedRepoAction::Remove => {
                remove_repo(
                    &args.root,
                    entry.provider.clone(),
                    &entry.scope,
                    &entry.name,
                )?;
                println!("Removed {}", entry.name);
            }
            DeletedRepoAction::Skip => {
                println!("Skipped {}", entry.name);
            }
        }
    }

    Ok(())
}

fn decide_action(
    entry: &RepoCacheEntry,
    policy: MissingRemotePolicy,
    non_interactive: bool,
) -> anyhow::Result<DeletedRepoAction> {
    if policy != MissingRemotePolicy::Prompt {
        return Ok(map_policy(policy));
    }
    if non_interactive {
        anyhow::bail!("non-interactive mode cannot prompt");
    }
    prompt_action(entry)
}

fn map_policy(policy: MissingRemotePolicy) -> DeletedRepoAction {
    match policy {
        MissingRemotePolicy::Prompt => DeletedRepoAction::Skip,
        MissingRemotePolicy::Archive => DeletedRepoAction::Archive,
        MissingRemotePolicy::Remove => DeletedRepoAction::Remove,
        MissingRemotePolicy::Skip => DeletedRepoAction::Skip,
    }
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

fn load_current_ids(path: &Path) -> anyhow::Result<HashSet<String>> {
    let data = fs::read_to_string(path).context("read current repo ids")?;
    let ids = data
        .lines()
        .map(|line| line.trim())
        .filter(|line| !line.is_empty())
        .map(str::to_string)
        .collect::<HashSet<_>>();
    Ok(ids)
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
    let config = AppConfig::load(config_path)?;
    let root = config
        .root
        .as_ref()
        .context("config missing root; run init")?;
    for target in config.targets {
        let provider = provider_for(target.provider)?;
        run_sync_filtered(
            provider.as_ref(),
            &target,
            root,
            cache_path,
            policy,
            None,
            None,
        )?;
    }
    Ok(())
}

fn select_targets(
    config: &AppConfig,
    provider: Option<ProviderKindValue>,
    scope: &[String],
) -> anyhow::Result<Vec<ProviderTarget>> {
    let mut targets = config.targets.clone();
    if let Some(provider) = provider {
        let provider: ProviderKind = provider.into();
        targets.retain(|target| target.provider == provider);
    } else if !scope.is_empty() {
        anyhow::bail!("--scope requires --provider");
    }

    if !scope.is_empty() {
        let scope = ProviderScope::new(scope.to_vec())?;
        targets.retain(|target| target.scope == scope);
    }

    Ok(targets)
}

fn provider_for(kind: ProviderKind) -> anyhow::Result<Box<dyn RepoProvider>> {
    match kind {
        ProviderKind::AzureDevOps => Ok(Box::new(AzureDevOpsProvider::new()?)),
        ProviderKind::GitHub => Ok(Box::new(GitHubProvider::new()?)),
        ProviderKind::GitLab => Ok(Box::new(GitLabProvider::new()?)),
    }
}

fn default_host(provider: ProviderKind) -> &'static str {
    match provider {
        ProviderKind::AzureDevOps => "https://dev.azure.com",
        ProviderKind::GitHub => "https://api.github.com",
        ProviderKind::GitLab => "https://gitlab.com/api/v4",
    }
}

fn account_key(
    provider: ProviderKind,
    host: &str,
    scope: &ProviderScope,
) -> anyhow::Result<String> {
    let segments = scope.segments();
    match provider {
        ProviderKind::AzureDevOps => {
            if segments.len() != 2 {
                anyhow::bail!("azure devops scope requires org and project segments");
            }
            Ok(format!("azdo:{host}:{}", segments[0]))
        }
        ProviderKind::GitHub => {
            if segments.len() != 1 {
                anyhow::bail!("github scope requires a single org/user segment");
            }
            Ok(format!("github:{host}:{}", segments[0]))
        }
        ProviderKind::GitLab => {
            if segments.is_empty() {
                anyhow::bail!("gitlab scope requires at least one group segment");
            }
            Ok(format!("gitlab:{host}:{}", segments.join("/")))
        }
    }
}

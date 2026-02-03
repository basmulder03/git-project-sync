use anyhow::Context;
use clap::{Parser, ValueEnum};
use mirror_core::archive::{archive_repo, remove_repo};
use mirror_core::cache::{RepoCache, RepoCacheEntry};
use mirror_core::deleted::{MissingRemotePolicy, detect_deleted_repos};
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
    #[command(about = "Handle missing-remote repos using cache and a current repo id list")]
    MissingRemote(MissingRemoteArgs),
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
        Commands::MissingRemote(args) => handle_missing_remote(args),
    }
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

    for entry in missing {
        let action = decide_action(entry, policy, args.non_interactive)?;
        match action {
            MissingRemotePolicy::Archive => {
                let destination = archive_repo(
                    &args.root,
                    entry.provider.clone(),
                    &entry.scope,
                    &entry.name,
                )?;
                println!("Archived {} -> {}", entry.name, destination.display());
            }
            MissingRemotePolicy::Remove => {
                remove_repo(
                    &args.root,
                    entry.provider.clone(),
                    &entry.scope,
                    &entry.name,
                )?;
                println!("Removed {}", entry.name);
            }
            MissingRemotePolicy::Skip => {
                println!("Skipped {}", entry.name);
            }
            MissingRemotePolicy::Prompt => unreachable!("prompt should be resolved to action"),
        }
    }

    Ok(())
}

fn decide_action(
    entry: &RepoCacheEntry,
    policy: MissingRemotePolicy,
    non_interactive: bool,
) -> anyhow::Result<MissingRemotePolicy> {
    if policy != MissingRemotePolicy::Prompt {
        return Ok(policy);
    }
    if non_interactive {
        anyhow::bail!("non-interactive mode cannot prompt");
    }
    prompt_action(entry)
}

fn prompt_action(entry: &RepoCacheEntry) -> anyhow::Result<MissingRemotePolicy> {
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
            "a" | "archive" => return Ok(MissingRemotePolicy::Archive),
            "r" | "remove" => return Ok(MissingRemotePolicy::Remove),
            "s" | "skip" => return Ok(MissingRemotePolicy::Skip),
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

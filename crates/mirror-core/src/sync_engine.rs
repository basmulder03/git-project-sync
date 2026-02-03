use crate::archive::{archive_repo, remove_repo};
use crate::cache::RepoCache;
use crate::deleted::{DeletedRepoAction, MissingRemotePolicy, detect_deleted_repos};
use crate::git_sync::{SyncOutcome, sync_repo};
use crate::paths::repo_path;
use crate::provider::RepoProvider;
use crate::model::{ProviderTarget, RemoteRepo};
use anyhow::Context;
use std::collections::HashSet;
use std::path::Path;
use std::time::{SystemTime, UNIX_EPOCH};
use tracing::{info, warn};

#[derive(Debug, Default, Clone, Copy)]
pub struct SyncSummary {
    pub cloned: u32,
    pub fast_forwarded: u32,
    pub up_to_date: u32,
    pub dirty: u32,
    pub diverged: u32,
    pub failed: u32,
}

impl SyncSummary {
    fn record(&mut self, outcome: SyncOutcome) {
        match outcome {
            SyncOutcome::Cloned => self.cloned += 1,
            SyncOutcome::FastForwarded => self.fast_forwarded += 1,
            SyncOutcome::UpToDate => self.up_to_date += 1,
            SyncOutcome::Dirty => self.dirty += 1,
            SyncOutcome::Diverged => self.diverged += 1,
        }
    }
}

pub fn run_sync(
    provider: &dyn RepoProvider,
    target: &ProviderTarget,
    root: &Path,
    cache_path: &Path,
    missing_policy: MissingRemotePolicy,
    missing_decider: Option<&dyn Fn(&crate::cache::RepoCacheEntry) -> anyhow::Result<DeletedRepoAction>>,
) -> anyhow::Result<SyncSummary> {
    run_sync_filtered(
        provider,
        target,
        root,
        cache_path,
        missing_policy,
        missing_decider,
        None,
        true,
    )
}

pub fn run_sync_filtered(
    provider: &dyn RepoProvider,
    target: &ProviderTarget,
    root: &Path,
    cache_path: &Path,
    missing_policy: MissingRemotePolicy,
    missing_decider: Option<&dyn Fn(&crate::cache::RepoCacheEntry) -> anyhow::Result<DeletedRepoAction>>,
    repo_filter: Option<&dyn Fn(&RemoteRepo) -> bool>,
    detect_missing: bool,
) -> anyhow::Result<SyncSummary> {
    provider
        .validate_auth(target)
        .context("validate provider auth")?;
    let mut cache = RepoCache::load(cache_path).context("load cache")?;
    let mut summary = SyncSummary::default();

    info!(
        provider = %target.provider,
        scope = ?target.scope,
        "starting sync for target"
    );

    let mut repos = provider.list_repos(target).context("list repos")?;
    if detect_missing {
        let current_ids: HashSet<String> = repos.iter().map(|repo| repo.id.clone()).collect();
        handle_missing_repos(
            &mut cache,
            root,
            &current_ids,
            missing_policy,
            missing_decider,
        )?;
    }
    if let Some(filter) = repo_filter {
        repos.retain(|repo| filter(repo));
    }

    for repo in repos {
        let path = repo_path(root, &repo.provider, &repo.scope, &repo.name);
        let outcome = match sync_repo(
            &path,
            &repo.clone_url,
            &repo.default_branch,
            repo.auth.as_ref(),
        ) {
            Ok(outcome) => outcome,
            Err(err) => {
                summary.failed += 1;
                warn!(
                    provider = %repo.provider,
                    scope = ?repo.scope,
                    repo_id = %repo.id,
                    path = %path.display(),
                    error = %err,
                    "repo sync failed"
                );
                cache.record_repo(
                    repo.id.clone(),
                    repo.name.clone(),
                    repo.provider.clone(),
                    repo.scope.clone(),
                    path.display().to_string(),
                );
                continue;
            }
        };

        info!(
            provider = %repo.provider,
            scope = ?repo.scope,
            repo_id = %repo.id,
            path = %path.display(),
            outcome = ?outcome,
            "repo sync outcome"
        );
        summary.record(outcome);

        cache.record_repo(
            repo.id.clone(),
            repo.name.clone(),
            repo.provider.clone(),
            repo.scope.clone(),
            path.display().to_string(),
        );

        if should_record_sync(outcome) {
            cache
                .last_sync
                .insert(repo.id.clone(), current_timestamp());
        }
    }

    cache.save(cache_path).context("save cache")?;
    Ok(summary)
}

fn handle_missing_repos(
    cache: &mut RepoCache,
    root: &Path,
    current_repo_ids: &HashSet<String>,
    policy: MissingRemotePolicy,
    decider: Option<&dyn Fn(&crate::cache::RepoCacheEntry) -> anyhow::Result<DeletedRepoAction>>,
) -> anyhow::Result<()> {
    let missing = detect_deleted_repos(cache, current_repo_ids);
    if missing.is_empty() {
        return Ok(());
    }

    let mut remove_ids: Vec<String> = Vec::new();
    for repo in missing {
        let action = resolve_action(repo.entry, policy, decider)?;
        match action {
            DeletedRepoAction::Archive => {
                let destination = archive_repo(
                    root,
                    repo.entry.provider.clone(),
                    &repo.entry.scope,
                    &repo.entry.name,
                )?;
                info!(
                    provider = %repo.entry.provider,
                    scope = ?repo.entry.scope,
                    repo_id = %repo.repo_id,
                    path = %destination.display(),
                    "archived missing repo"
                );
                remove_ids.push(repo.repo_id.to_string());
            }
            DeletedRepoAction::Remove => {
                remove_repo(
                    root,
                    repo.entry.provider.clone(),
                    &repo.entry.scope,
                    &repo.entry.name,
                )?;
                info!(
                    provider = %repo.entry.provider,
                    scope = ?repo.entry.scope,
                    repo_id = %repo.repo_id,
                    "removed missing repo"
                );
                remove_ids.push(repo.repo_id.to_string());
            }
            DeletedRepoAction::Skip => {
                warn!(
                    provider = %repo.entry.provider,
                    scope = ?repo.entry.scope,
                    repo_id = %repo.repo_id,
                    "skipped missing repo"
                );
            }
        }
    }

    for repo_id in remove_ids {
        cache.repos.remove(&repo_id);
    }

    Ok(())
}

fn resolve_action(
    entry: &crate::cache::RepoCacheEntry,
    policy: MissingRemotePolicy,
    decider: Option<&dyn Fn(&crate::cache::RepoCacheEntry) -> anyhow::Result<DeletedRepoAction>>,
) -> anyhow::Result<DeletedRepoAction> {
    match policy {
        MissingRemotePolicy::Prompt => {
            let decider = decider.context("missing-remote prompt requires a decider")?;
            decider(entry)
        }
        MissingRemotePolicy::Archive => Ok(DeletedRepoAction::Archive),
        MissingRemotePolicy::Remove => Ok(DeletedRepoAction::Remove),
        MissingRemotePolicy::Skip => Ok(DeletedRepoAction::Skip),
    }
}

fn should_record_sync(outcome: SyncOutcome) -> bool {
    matches!(
        outcome,
        SyncOutcome::Cloned | SyncOutcome::FastForwarded | SyncOutcome::UpToDate
    )
}

fn current_timestamp() -> String {
    let since_epoch = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap_or_default();
    since_epoch.as_secs().to_string()
}

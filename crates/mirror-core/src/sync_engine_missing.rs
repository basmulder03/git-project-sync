use crate::archive::{archive_repo, remove_repo};
use crate::cache::{RepoCache, RepoCacheEntry};
use crate::deleted::{DeletedRepoAction, MissingRemotePolicy, detect_deleted_repos};
use crate::sync_engine::MissingDecider;
use crate::sync_engine_types::MissingSummary;
use anyhow::Context;
use std::collections::HashSet;
use std::path::Path;
use tracing::{info, warn};

#[allow(clippy::type_complexity)]
pub(crate) fn handle_missing_repos(
    cache: &mut RepoCache,
    root: &Path,
    current_repo_ids: &HashSet<String>,
    policy: MissingRemotePolicy,
    decider: Option<&MissingDecider>,
    mut on_action: Option<&mut dyn FnMut(DeletedRepoAction, &crate::deleted::DeletedRepo)>,
) -> anyhow::Result<MissingSummary> {
    let missing = detect_deleted_repos(cache, current_repo_ids);
    if missing.is_empty() {
        return Ok(MissingSummary::default());
    }

    let mut remove_ids: Vec<String> = Vec::new();
    let mut summary = MissingSummary::default();
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
                if let Some(callback) = on_action.as_mut() {
                    callback(DeletedRepoAction::Archive, &repo);
                }
                remove_ids.push(repo.repo_id.to_string());
                summary.archived += 1;
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
                if let Some(callback) = on_action.as_mut() {
                    callback(DeletedRepoAction::Remove, &repo);
                }
                remove_ids.push(repo.repo_id.to_string());
                summary.removed += 1;
            }
            DeletedRepoAction::Skip => {
                warn!(
                    provider = %repo.entry.provider,
                    scope = ?repo.entry.scope,
                    repo_id = %repo.repo_id,
                    "skipped missing repo"
                );
                if let Some(callback) = on_action.as_mut() {
                    callback(DeletedRepoAction::Skip, &repo);
                }
                summary.skipped += 1;
            }
        }
    }

    for repo_id in remove_ids {
        cache.repos.remove(&repo_id);
    }

    Ok(summary)
}

fn resolve_action(
    entry: &RepoCacheEntry,
    policy: MissingRemotePolicy,
    decider: Option<&MissingDecider>,
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

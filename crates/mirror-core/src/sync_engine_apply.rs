use crate::cache::RepoCache;
use crate::git_sync::SyncOutcome;
use crate::sync_engine::{SyncAction, SyncProgressReporter, SyncSummary};
use crate::sync_engine_status::{
    action_from_outcome, current_timestamp, emit_sync_status, should_record_sync,
};
use crate::sync_engine_types::{RepoWorkItem, StatusEmitterState};
use std::path::Path;
use tracing::{info, warn};

pub(crate) fn apply_success_outcome(
    cache: &mut RepoCache,
    cache_path: &Path,
    progress: Option<&SyncProgressReporter<'_>>,
    state: &mut StatusEmitterState,
    summary: &mut SyncSummary,
    item: &RepoWorkItem,
    outcome: SyncOutcome,
) -> anyhow::Result<()> {
    info!(
        provider = %item.repo.provider,
        scope = ?item.repo.scope,
        repo_id = %item.repo.id,
        path = %item.path.display(),
        outcome = ?outcome,
        "repo sync outcome"
    );
    summary.record(outcome);
    state.processed_repos += 1;
    emit_sync_status(
        cache,
        cache_path,
        progress,
        state,
        action_from_outcome(outcome),
        Some(&item.repo.name),
        Some(&item.repo.id),
        true,
        *summary,
    )?;
    record_repo(cache, item);
    if should_record_sync(outcome) {
        cache
            .last_sync
            .insert(item.repo.id.clone(), current_timestamp());
    }
    Ok(())
}

pub(crate) fn apply_failed_outcome(
    cache: &mut RepoCache,
    cache_path: &Path,
    progress: Option<&SyncProgressReporter<'_>>,
    state: &mut StatusEmitterState,
    summary: &mut SyncSummary,
    item: &RepoWorkItem,
    err: &anyhow::Error,
) -> anyhow::Result<()> {
    summary.failed += 1;
    state.processed_repos += 1;
    emit_sync_status(
        cache,
        cache_path,
        progress,
        state,
        SyncAction::Failed,
        Some(&item.repo.name),
        Some(&item.repo.id),
        true,
        *summary,
    )?;
    warn!(
        provider = %item.repo.provider,
        scope = ?item.repo.scope,
        repo_id = %item.repo.id,
        path = %item.path.display(),
        error = %err,
        "repo sync failed"
    );
    record_repo(cache, item);
    Ok(())
}

fn record_repo(cache: &mut RepoCache, item: &RepoWorkItem) {
    cache.record_repo(
        item.repo.id.clone(),
        item.repo.name.clone(),
        item.repo.provider.clone(),
        item.repo.scope.clone(),
        item.path.display().to_string(),
    );
}

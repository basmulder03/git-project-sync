use crate::cache::{RepoCache, SyncSummarySnapshot};
use crate::git_sync::SyncOutcome;
use crate::sync_engine::{SyncAction, SyncProgress, SyncProgressReporter, SyncSummary};
use crate::sync_engine_types::StatusEmitterState;
use anyhow::Context;
use std::path::Path;
use std::time::{Duration, Instant, SystemTime, UNIX_EPOCH};

pub(crate) fn action_from_outcome(outcome: SyncOutcome) -> SyncAction {
    match outcome {
        SyncOutcome::Cloned => SyncAction::Cloned,
        SyncOutcome::FastForwarded => SyncAction::FastForwarded,
        SyncOutcome::UpToDate => SyncAction::UpToDate,
        SyncOutcome::Dirty => SyncAction::Dirty,
        SyncOutcome::Diverged => SyncAction::Diverged,
    }
}

pub(crate) fn should_record_sync(outcome: SyncOutcome) -> bool {
    matches!(
        outcome,
        SyncOutcome::Cloned | SyncOutcome::FastForwarded | SyncOutcome::UpToDate
    )
}

pub(crate) fn current_timestamp() -> String {
    let since_epoch = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap_or_default();
    since_epoch.as_secs().to_string()
}

pub(crate) fn current_timestamp_secs() -> u64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap_or_default()
        .as_secs()
}

fn summary_snapshot(summary: SyncSummary) -> SyncSummarySnapshot {
    SyncSummarySnapshot {
        cloned: summary.cloned,
        fast_forwarded: summary.fast_forwarded,
        up_to_date: summary.up_to_date,
        dirty: summary.dirty,
        diverged: summary.diverged,
        failed: summary.failed,
        missing_archived: summary.missing_archived,
        missing_removed: summary.missing_removed,
        missing_skipped: summary.missing_skipped,
    }
}

#[allow(clippy::too_many_arguments)]
pub(crate) fn emit_sync_status(
    cache: &mut RepoCache,
    cache_path: &Path,
    progress: Option<&SyncProgressReporter<'_>>,
    state: &mut StatusEmitterState,
    action: SyncAction,
    repo_name: Option<&str>,
    repo_id: Option<&str>,
    in_progress: bool,
    summary: SyncSummary,
) -> anyhow::Result<()> {
    let entry = cache
        .target_sync_status
        .entry(state.target_key.clone())
        .or_default();
    entry.in_progress = in_progress;
    entry.last_action = Some(action.as_str().to_string());
    entry.last_repo = repo_name.map(|value| value.to_string());
    entry.last_repo_id = repo_id.map(|value| value.to_string());
    entry.last_updated = current_timestamp_secs();
    entry.total_repos = state.total_repos;
    entry.processed_repos = state.processed_repos;
    entry.summary = summary_snapshot(summary);
    state.status_dirty = true;

    if let Some(progress) = progress {
        progress(SyncProgress {
            target_id: state.target_key.clone(),
            total_repos: state.total_repos,
            processed_repos: state.processed_repos,
            action,
            repo_id: repo_id.map(|value| value.to_string()),
            repo_name: repo_name.map(|value| value.to_string()),
            summary,
            in_progress,
        });
    }

    if state.status_dirty
        && (!in_progress || state.last_status_flush.elapsed() >= Duration::from_millis(500))
    {
        cache.save(cache_path).context("save cache")?;
        state.status_dirty = false;
        state.last_status_flush = Instant::now();
    }
    Ok(())
}

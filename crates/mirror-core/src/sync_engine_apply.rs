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

#[cfg(test)]
mod tests {
    use super::*;
    use crate::model::{ProviderKind, ProviderScope, RemoteRepo};
    use crate::sync_engine_types::StatusEmitterState;
    use anyhow::anyhow;
    use std::time::Instant;
    use tempfile::TempDir;

    #[test]
    fn apply_success_updates_summary_status_and_cache() {
        let tmp = TempDir::new().unwrap();
        let cache_path = tmp.path().join("cache.json");
        let mut cache = RepoCache::new();
        let mut summary = SyncSummary::default();
        let mut state = StatusEmitterState {
            target_key: "target-1".into(),
            last_status_flush: Instant::now(),
            status_dirty: false,
            total_repos: 1,
            processed_repos: 0,
        };
        let item = work_item(tmp.path());

        apply_success_outcome(
            &mut cache,
            &cache_path,
            None,
            &mut state,
            &mut summary,
            &item,
            SyncOutcome::UpToDate,
        )
        .unwrap();

        assert_eq!(summary.up_to_date, 1);
        assert_eq!(state.processed_repos, 1);
        assert!(cache.repos.contains_key("repo-1"));
        assert!(cache.last_sync.contains_key("repo-1"));
        assert_eq!(
            cache
                .target_sync_status
                .get("target-1")
                .and_then(|status| status.last_action.as_deref()),
            Some("up_to_date")
        );
    }

    #[test]
    fn apply_failure_updates_failed_count_without_last_sync() {
        let tmp = TempDir::new().unwrap();
        let cache_path = tmp.path().join("cache.json");
        let mut cache = RepoCache::new();
        let mut summary = SyncSummary::default();
        let mut state = StatusEmitterState {
            target_key: "target-1".into(),
            last_status_flush: Instant::now(),
            status_dirty: false,
            total_repos: 1,
            processed_repos: 0,
        };
        let item = work_item(tmp.path());

        apply_failed_outcome(
            &mut cache,
            &cache_path,
            None,
            &mut state,
            &mut summary,
            &item,
            &anyhow!("sync failed"),
        )
        .unwrap();

        assert_eq!(summary.failed, 1);
        assert_eq!(state.processed_repos, 1);
        assert!(cache.repos.contains_key("repo-1"));
        assert!(!cache.last_sync.contains_key("repo-1"));
        assert_eq!(
            cache
                .target_sync_status
                .get("target-1")
                .and_then(|status| status.last_action.as_deref()),
            Some("failed")
        );
    }

    fn work_item(root: &Path) -> RepoWorkItem {
        RepoWorkItem {
            repo: RemoteRepo {
                id: "repo-1".into(),
                name: "Repo One".into(),
                clone_url: "https://example.com/repo-1.git".into(),
                default_branch: "main".into(),
                archived: false,
                provider: ProviderKind::GitHub,
                scope: ProviderScope::new(vec!["org".into()]).unwrap(),
            },
            path: root.join("repo-1"),
        }
    }
}

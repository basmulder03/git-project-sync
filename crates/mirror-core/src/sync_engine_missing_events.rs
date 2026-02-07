use crate::cache::RepoCache;
use crate::deleted::{DeletedRepoAction, MissingRemotePolicy};
use crate::model::RemoteRepo;
use crate::sync_engine::{MissingDecider, SyncAction, SyncProgressReporter, SyncSummary};
use crate::sync_engine_missing::handle_missing_repos;
use crate::sync_engine_status::emit_sync_status;
use crate::sync_engine_types::StatusEmitterState;
use std::collections::HashSet;
use std::path::Path;

pub(crate) struct MissingStatusSink<'a, 'p> {
    pub(crate) cache_path: &'a Path,
    pub(crate) progress: Option<&'p SyncProgressReporter<'p>>,
    pub(crate) state: &'a mut StatusEmitterState,
    pub(crate) summary: &'a mut SyncSummary,
}

pub(crate) fn detect_and_emit_missing_repos(
    cache: &mut RepoCache,
    root: &Path,
    repos: &[RemoteRepo],
    missing_policy: MissingRemotePolicy,
    missing_decider: Option<&MissingDecider>,
    sink: MissingStatusSink<'_, '_>,
) -> anyhow::Result<()> {
    let current_ids: HashSet<String> = repos.iter().map(|repo| repo.id.clone()).collect();
    let mut events: Vec<(DeletedRepoAction, String, String)> = Vec::new();
    let mut on_missing = |action: DeletedRepoAction, repo: &crate::deleted::DeletedRepo| {
        events.push((action, repo.repo_id.to_string(), repo.entry.name.clone()));
    };
    let missing_summary = handle_missing_repos(
        cache,
        root,
        &current_ids,
        missing_policy,
        missing_decider,
        Some(&mut on_missing),
    )?;
    sink.summary.record_missing(missing_summary);
    emit_missing_events(cache, sink, events)?;
    Ok(())
}

fn emit_missing_events(
    cache: &mut RepoCache,
    sink: MissingStatusSink<'_, '_>,
    events: Vec<(DeletedRepoAction, String, String)>,
) -> anyhow::Result<()> {
    for (action, repo_id, repo_name) in events {
        emit_sync_status(
            cache,
            sink.cache_path,
            sink.progress,
            sink.state,
            action_for_missing(action),
            Some(&repo_name),
            Some(&repo_id),
            true,
            *sink.summary,
        )?;
    }
    Ok(())
}

fn action_for_missing(action: DeletedRepoAction) -> SyncAction {
    match action {
        DeletedRepoAction::Archive => SyncAction::MissingArchived,
        DeletedRepoAction::Remove => SyncAction::MissingRemoved,
        DeletedRepoAction::Skip => SyncAction::MissingSkipped,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn maps_missing_actions_to_sync_actions() {
        assert!(matches!(
            action_for_missing(DeletedRepoAction::Archive),
            SyncAction::MissingArchived
        ));
        assert!(matches!(
            action_for_missing(DeletedRepoAction::Remove),
            SyncAction::MissingRemoved
        ));
        assert!(matches!(
            action_for_missing(DeletedRepoAction::Skip),
            SyncAction::MissingSkipped
        ));
    }
}

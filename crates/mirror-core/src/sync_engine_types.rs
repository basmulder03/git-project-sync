use crate::model::RemoteRepo;
use crate::sync_engine::SyncSummary;
use std::path::PathBuf;
use std::time::Instant;

#[derive(Debug, Default, Clone, Copy)]
pub(crate) struct MissingSummary {
    pub(crate) archived: u32,
    pub(crate) removed: u32,
    pub(crate) skipped: u32,
}

pub(crate) struct StatusEmitterState {
    pub(crate) target_key: String,
    pub(crate) last_status_flush: Instant,
    pub(crate) status_dirty: bool,
    pub(crate) total_repos: usize,
    pub(crate) processed_repos: usize,
}

pub(crate) struct RepoWorkItem {
    pub(crate) repo: RemoteRepo,
    pub(crate) path: PathBuf,
}

pub(crate) enum RepoEvent {
    Started {
        repo_id: String,
        repo_name: String,
    },
    Finished {
        item: RepoWorkItem,
        outcome: anyhow::Result<crate::git_sync::SyncOutcome>,
    },
}

pub(crate) fn merge_missing_summary(summary: &mut SyncSummary, missing: MissingSummary) {
    summary.missing_archived += missing.archived;
    summary.missing_removed += missing.removed;
    summary.missing_skipped += missing.skipped;
}

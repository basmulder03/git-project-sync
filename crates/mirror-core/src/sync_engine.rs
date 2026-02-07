use crate::deleted::{DeletedRepoAction, MissingRemotePolicy};
use crate::git_sync::SyncOutcome;
use crate::model::{ProviderTarget, RemoteRepo};
use crate::provider::RepoProvider;
use crate::sync_engine_types::{MissingSummary, merge_missing_summary};
use std::path::Path;

pub type MissingDecider =
    dyn Fn(&crate::cache::RepoCacheEntry) -> anyhow::Result<DeletedRepoAction>;
pub type RepoFilter = dyn Fn(&RemoteRepo) -> bool;
pub type SyncProgressReporter<'a> = dyn Fn(SyncProgress) + 'a;

#[derive(Clone, Copy, Debug)]
pub enum SyncAction {
    Starting,
    Syncing,
    Cloned,
    FastForwarded,
    UpToDate,
    Dirty,
    Diverged,
    Failed,
    MissingArchived,
    MissingRemoved,
    MissingSkipped,
    Done,
}

impl SyncAction {
    pub fn as_str(&self) -> &'static str {
        match self {
            SyncAction::Starting => "starting",
            SyncAction::Syncing => "syncing",
            SyncAction::Cloned => "cloned",
            SyncAction::FastForwarded => "fast_forwarded",
            SyncAction::UpToDate => "up_to_date",
            SyncAction::Dirty => "dirty",
            SyncAction::Diverged => "diverged",
            SyncAction::Failed => "failed",
            SyncAction::MissingArchived => "missing_archived",
            SyncAction::MissingRemoved => "missing_removed",
            SyncAction::MissingSkipped => "missing_skipped",
            SyncAction::Done => "done",
        }
    }
}

#[derive(Clone, Debug)]
pub struct SyncProgress {
    pub target_id: String,
    pub total_repos: usize,
    pub processed_repos: usize,
    pub action: SyncAction,
    pub repo_id: Option<String>,
    pub repo_name: Option<String>,
    pub summary: SyncSummary,
    pub in_progress: bool,
}

#[derive(Debug, Default, Clone, Copy)]
pub struct SyncSummary {
    pub cloned: u32,
    pub fast_forwarded: u32,
    pub up_to_date: u32,
    pub dirty: u32,
    pub diverged: u32,
    pub failed: u32,
    pub missing_archived: u32,
    pub missing_removed: u32,
    pub missing_skipped: u32,
}

impl SyncSummary {
    pub(crate) fn record(&mut self, outcome: SyncOutcome) {
        match outcome {
            SyncOutcome::Cloned => self.cloned += 1,
            SyncOutcome::FastForwarded => self.fast_forwarded += 1,
            SyncOutcome::UpToDate => self.up_to_date += 1,
            SyncOutcome::Dirty => self.dirty += 1,
            SyncOutcome::Diverged => self.diverged += 1,
        }
    }

    pub(crate) fn record_missing(&mut self, missing: MissingSummary) {
        merge_missing_summary(self, missing);
    }
}

pub async fn run_sync(
    provider: &dyn RepoProvider,
    target: &ProviderTarget,
    root: &Path,
    cache_path: &Path,
    missing_policy: MissingRemotePolicy,
    missing_decider: Option<&MissingDecider>,
) -> anyhow::Result<SyncSummary> {
    let options = RunSyncOptions {
        missing_policy,
        missing_decider,
        ..RunSyncOptions::default()
    };
    run_sync_filtered(provider, target, root, cache_path, options).await
}

#[derive(Clone, Copy)]
pub struct RunSyncOptions<'a, 'b, 'c> {
    pub missing_policy: MissingRemotePolicy,
    pub missing_decider: Option<&'a MissingDecider>,
    pub repo_filter: Option<&'b RepoFilter>,
    pub progress: Option<&'c SyncProgressReporter<'c>>,
    pub jobs: usize,
    pub detect_missing: bool,
    pub refresh: bool,
    pub verify: bool,
}

impl Default for RunSyncOptions<'_, '_, '_> {
    fn default() -> Self {
        Self {
            missing_policy: MissingRemotePolicy::Skip,
            missing_decider: None,
            repo_filter: None,
            progress: None,
            jobs: 1,
            detect_missing: true,
            refresh: false,
            verify: false,
        }
    }
}

pub async fn run_sync_filtered(
    provider: &dyn RepoProvider,
    target: &ProviderTarget,
    root: &Path,
    cache_path: &Path,
    options: RunSyncOptions<'_, '_, '_>,
) -> anyhow::Result<SyncSummary> {
    crate::sync_engine_orchestrator::run_sync_filtered_orchestrated(
        provider, target, root, cache_path, options,
    )
    .await
}

#[cfg(test)]
mod tests {
    use crate::cache::RepoInventoryEntry;
    use tempfile::TempDir;

    #[test]
    fn cache_is_valid_within_ttl() {
        let entry = RepoInventoryEntry {
            fetched_at: 100,
            repos: Vec::new(),
        };
        assert!(crate::sync_engine_inventory::cache_is_valid(
            &entry,
            100 + 60
        ));
        assert!(!crate::sync_engine_inventory::cache_is_valid(
            &entry,
            100 + 16 * 60
        ));
    }

    #[test]
    fn move_repo_path_moves_directory() {
        let tmp = TempDir::new().unwrap();
        let from = tmp.path().join("old");
        let to = tmp.path().join("new");
        std::fs::create_dir_all(&from).unwrap();
        std::fs::write(from.join("file.txt"), "data").unwrap();

        crate::sync_engine_fs::move_repo_path(&from, &to).unwrap();
        assert!(!from.exists());
        assert!(to.join("file.txt").exists());
    }

    #[test]
    fn current_timestamp_secs_is_monotonicish() {
        let first = crate::sync_engine_status::current_timestamp_secs();
        let second = crate::sync_engine_status::current_timestamp_secs();
        assert!(second >= first);
    }

    #[test]
    fn normalized_jobs_is_bounded_by_work_items() {
        assert_eq!(crate::sync_engine_orchestrator::normalized_jobs(0, 0), 1);
        assert_eq!(crate::sync_engine_orchestrator::normalized_jobs(1, 0), 1);
        assert_eq!(crate::sync_engine_orchestrator::normalized_jobs(8, 3), 3);
        assert_eq!(crate::sync_engine_orchestrator::normalized_jobs(2, 5), 2);
    }
}

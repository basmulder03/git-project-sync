use crate::cache::RepoCache;
use crate::config::target_id;
use crate::deleted::{DeletedRepoAction, MissingRemotePolicy};
use crate::git_sync::SyncOutcome;
use crate::model::{ProviderTarget, RemoteRepo, RepoAuth};
use crate::provider::RepoProvider;
use crate::sync_engine_inventory::load_repos_with_cache;
use crate::sync_engine_missing_events::{MissingStatusSink, detect_and_emit_missing_repos};
use crate::sync_engine_status::emit_sync_status;
use crate::sync_engine_types::{MissingSummary, StatusEmitterState, merge_missing_summary};
use crate::sync_engine_work_items::build_work_items;
use crate::sync_engine_workers::run_work_items;
use anyhow::Context;
use std::path::Path;
use std::time::Instant;
use tracing::info;

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
    let target_auth = preflight_auth(provider, target).await?;
    let mut state = load_run_state(cache_path, target)?;

    emit_lifecycle_status(
        &mut state,
        cache_path,
        options.progress,
        SyncAction::Starting,
        true,
    )?;

    info!(
        provider = %target.provider,
        scope = ?target.scope,
        "starting sync for target"
    );

    let repos =
        prepare_repos_phase(provider, target, root, cache_path, options, &mut state).await?;

    execute_repos_phase(root, cache_path, options, target_auth, repos, &mut state)?;

    finalize_sync_phase(cache_path, options.progress, &mut state)
}

async fn preflight_auth(
    provider: &dyn RepoProvider,
    target: &ProviderTarget,
) -> anyhow::Result<Option<RepoAuth>> {
    provider
        .validate_auth(target)
        .await
        .context("validate provider auth")?;
    provider
        .auth_for_target(target)
        .await
        .context("resolve provider auth for target")
}

fn load_run_state(cache_path: &Path, target: &ProviderTarget) -> anyhow::Result<SyncRunState> {
    let cache = RepoCache::load(cache_path).context("load cache")?;
    let summary = SyncSummary::default();
    let target_key = target_id(
        target.provider.clone(),
        target.host.as_deref(),
        &target.scope,
    );
    let status_state = StatusEmitterState {
        target_key,
        last_status_flush: Instant::now(),
        status_dirty: false,
        total_repos: 0,
        processed_repos: 0,
    };
    Ok(SyncRunState {
        cache,
        summary,
        status_state,
    })
}

fn emit_lifecycle_status(
    state: &mut SyncRunState,
    cache_path: &Path,
    progress: Option<&SyncProgressReporter<'_>>,
    action: SyncAction,
    in_progress: bool,
) -> anyhow::Result<()> {
    emit_sync_status(
        &mut state.cache,
        cache_path,
        progress,
        &mut state.status_state,
        action,
        None,
        None,
        in_progress,
        state.summary,
    )
}

async fn prepare_repos_phase(
    provider: &dyn RepoProvider,
    target: &ProviderTarget,
    root: &Path,
    cache_path: &Path,
    options: RunSyncOptions<'_, '_, '_>,
    state: &mut SyncRunState,
) -> anyhow::Result<Vec<RemoteRepo>> {
    let (mut repos, used_cache) =
        load_repos_with_cache(provider, target, &mut state.cache, options.refresh).await?;
    if options.detect_missing && !used_cache {
        detect_and_emit_missing_repos(
            &mut state.cache,
            root,
            &repos,
            options.missing_policy,
            options.missing_decider,
            MissingStatusSink {
                cache_path,
                progress: options.progress,
                state: &mut state.status_state,
                summary: &mut state.summary,
            },
        )?;
    }
    if let Some(filter) = options.repo_filter {
        repos.retain(|repo| filter(repo));
    }
    state.status_state.total_repos = repos.len();
    emit_lifecycle_status(
        state,
        cache_path,
        options.progress,
        SyncAction::Syncing,
        true,
    )?;
    Ok(repos)
}

fn execute_repos_phase(
    root: &Path,
    cache_path: &Path,
    options: RunSyncOptions<'_, '_, '_>,
    target_auth: Option<RepoAuth>,
    repos: Vec<RemoteRepo>,
    state: &mut SyncRunState,
) -> anyhow::Result<()> {
    let work_items = build_work_items(&state.cache, root, repos);
    let jobs = normalized_jobs(options.jobs, work_items.len());
    run_work_items(
        &mut state.cache,
        cache_path,
        options.progress,
        &mut state.status_state,
        &mut state.summary,
        work_items,
        jobs,
        target_auth,
        options.verify,
    )
}

fn finalize_sync_phase(
    cache_path: &Path,
    progress: Option<&SyncProgressReporter<'_>>,
    state: &mut SyncRunState,
) -> anyhow::Result<SyncSummary> {
    emit_lifecycle_status(state, cache_path, progress, SyncAction::Done, false)?;
    state.cache.save(cache_path).context("save cache")?;
    Ok(state.summary)
}

fn normalized_jobs(requested_jobs: usize, work_item_count: usize) -> usize {
    requested_jobs.max(1).min(work_item_count.max(1))
}

struct SyncRunState {
    cache: RepoCache,
    summary: SyncSummary,
    status_state: StatusEmitterState,
}

#[cfg(test)]
mod tests {
    use super::normalized_jobs;
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
        assert_eq!(normalized_jobs(0, 0), 1);
        assert_eq!(normalized_jobs(1, 0), 1);
        assert_eq!(normalized_jobs(8, 3), 3);
        assert_eq!(normalized_jobs(2, 5), 2);
    }
}

use crate::cache::RepoCache;
use crate::config::target_id;
use crate::model::{ProviderTarget, RemoteRepo, RepoAuth};
use crate::provider::RepoProvider;
use crate::sync_engine::{RunSyncOptions, SyncAction, SyncProgressReporter, SyncSummary};
use crate::sync_engine_inventory::load_repos_with_cache;
use crate::sync_engine_missing_events::{MissingStatusSink, detect_and_emit_missing_repos};
use crate::sync_engine_status::emit_sync_status;
use crate::sync_engine_types::StatusEmitterState;
use crate::sync_engine_work_items::build_work_items;
use crate::sync_engine_workers::run_work_items;
use anyhow::Context;
use std::path::Path;
use std::time::Instant;
use tracing::info;

pub(crate) async fn run_sync_filtered_orchestrated(
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

pub(crate) fn normalized_jobs(requested_jobs: usize, work_item_count: usize) -> usize {
    requested_jobs.max(1).min(work_item_count.max(1))
}

struct SyncRunState {
    cache: RepoCache,
    summary: SyncSummary,
    status_state: StatusEmitterState,
}

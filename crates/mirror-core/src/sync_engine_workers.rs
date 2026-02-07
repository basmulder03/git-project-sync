use crate::cache::RepoCache;
use crate::git_sync::sync_repo;
use crate::model::RepoAuth;
use crate::sync_engine::{SyncAction, SyncProgressReporter, SyncSummary};
use crate::sync_engine_apply::{apply_failed_outcome, apply_success_outcome};
use crate::sync_engine_status::emit_sync_status;
use crate::sync_engine_types::{RepoEvent, RepoWorkItem, StatusEmitterState};
use std::path::Path;
use std::sync::{Arc, Mutex, mpsc};

#[allow(clippy::too_many_arguments)]
pub(crate) fn run_work_items(
    cache: &mut RepoCache,
    cache_path: &Path,
    progress: Option<&SyncProgressReporter<'_>>,
    state: &mut StatusEmitterState,
    summary: &mut SyncSummary,
    work_items: Vec<RepoWorkItem>,
    jobs: usize,
    target_auth: Option<RepoAuth>,
    verify: bool,
) -> anyhow::Result<()> {
    if jobs <= 1 {
        return run_work_items_serial(
            cache,
            cache_path,
            progress,
            state,
            summary,
            work_items,
            target_auth,
            verify,
        );
    }
    run_work_items_parallel(
        cache,
        cache_path,
        progress,
        state,
        summary,
        work_items,
        jobs,
        target_auth,
        verify,
    )
}

#[allow(clippy::too_many_arguments)]
fn run_work_items_serial(
    cache: &mut RepoCache,
    cache_path: &Path,
    progress: Option<&SyncProgressReporter<'_>>,
    state: &mut StatusEmitterState,
    summary: &mut SyncSummary,
    work_items: Vec<RepoWorkItem>,
    target_auth: Option<RepoAuth>,
    verify: bool,
) -> anyhow::Result<()> {
    for item in work_items {
        emit_sync_status(
            cache,
            cache_path,
            progress,
            state,
            SyncAction::Syncing,
            Some(&item.repo.name),
            Some(&item.repo.id),
            true,
            *summary,
        )?;
        let outcome = sync_repo(
            &item.path,
            &item.repo.clone_url,
            &item.repo.default_branch,
            target_auth.as_ref(),
            verify,
        );
        match outcome {
            Ok(outcome) => {
                apply_success_outcome(cache, cache_path, progress, state, summary, &item, outcome)?
            }
            Err(err) => {
                apply_failed_outcome(cache, cache_path, progress, state, summary, &item, &err)?
            }
        }
    }
    Ok(())
}

#[allow(clippy::too_many_arguments)]
fn run_work_items_parallel(
    cache: &mut RepoCache,
    cache_path: &Path,
    progress: Option<&SyncProgressReporter<'_>>,
    state: &mut StatusEmitterState,
    summary: &mut SyncSummary,
    work_items: Vec<RepoWorkItem>,
    jobs: usize,
    target_auth: Option<RepoAuth>,
    verify: bool,
) -> anyhow::Result<()> {
    let queue = Arc::new(Mutex::new(work_items));
    let (tx, rx) = mpsc::channel::<RepoEvent>();
    for _ in 0..jobs {
        let queue = Arc::clone(&queue);
        let tx = tx.clone();
        let target_auth = target_auth.clone();
        std::thread::spawn(move || {
            loop {
                let next = {
                    let mut guard = queue.lock().unwrap();
                    guard.pop()
                };
                let Some(item) = next else {
                    break;
                };
                let _ = tx.send(RepoEvent::Started {
                    repo_id: item.repo.id.clone(),
                    repo_name: item.repo.name.clone(),
                });
                let outcome = sync_repo(
                    &item.path,
                    &item.repo.clone_url,
                    &item.repo.default_branch,
                    target_auth.as_ref(),
                    verify,
                );
                let _ = tx.send(RepoEvent::Finished { item, outcome });
            }
        });
    }
    drop(tx);

    while let Ok(event) = rx.recv() {
        match event {
            RepoEvent::Started { repo_id, repo_name } => {
                emit_sync_status(
                    cache,
                    cache_path,
                    progress,
                    state,
                    SyncAction::Syncing,
                    Some(&repo_name),
                    Some(&repo_id),
                    true,
                    *summary,
                )?;
            }
            RepoEvent::Finished { item, outcome } => match outcome {
                Ok(outcome) => apply_success_outcome(
                    cache, cache_path, progress, state, summary, &item, outcome,
                )?,
                Err(err) => {
                    apply_failed_outcome(cache, cache_path, progress, state, summary, &item, &err)?
                }
            },
        }
    }

    Ok(())
}

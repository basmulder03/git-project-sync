use crate::cache::RepoCache;
use crate::config::target_id;
use crate::deleted::{DeletedRepoAction, MissingRemotePolicy};
use crate::git_sync::{SyncOutcome, sync_repo};
use crate::model::{ProviderTarget, RemoteRepo};
use crate::paths::repo_path;
use crate::provider::RepoProvider;
use crate::sync_engine_fs::move_repo_path;
use crate::sync_engine_inventory::load_repos_with_cache;
use crate::sync_engine_missing::handle_missing_repos;
use crate::sync_engine_status::{
    action_from_outcome, current_timestamp, emit_sync_status, should_record_sync,
};
use crate::sync_engine_types::{
    MissingSummary, RepoEvent, RepoWorkItem, StatusEmitterState, merge_missing_summary,
};
use anyhow::Context;
use std::collections::HashSet;
use std::path::{Path, PathBuf};
use std::sync::{Arc, Mutex, mpsc};
use std::time::Instant;
use tracing::{info, warn};

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
    fn record(&mut self, outcome: SyncOutcome) {
        match outcome {
            SyncOutcome::Cloned => self.cloned += 1,
            SyncOutcome::FastForwarded => self.fast_forwarded += 1,
            SyncOutcome::UpToDate => self.up_to_date += 1,
            SyncOutcome::Dirty => self.dirty += 1,
            SyncOutcome::Diverged => self.diverged += 1,
        }
    }

    fn record_missing(&mut self, missing: MissingSummary) {
        merge_missing_summary(self, missing);
    }
}

pub fn run_sync(
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
    run_sync_filtered(provider, target, root, cache_path, options)
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

pub fn run_sync_filtered(
    provider: &dyn RepoProvider,
    target: &ProviderTarget,
    root: &Path,
    cache_path: &Path,
    options: RunSyncOptions<'_, '_, '_>,
) -> anyhow::Result<SyncSummary> {
    provider
        .validate_auth(target)
        .context("validate provider auth")?;
    let mut cache = RepoCache::load(cache_path).context("load cache")?;
    let mut summary = SyncSummary::default();
    let target_key = target_id(
        target.provider.clone(),
        target.host.as_deref(),
        &target.scope,
    );
    let mut status_state = StatusEmitterState {
        target_key,
        last_status_flush: Instant::now(),
        status_dirty: false,
        total_repos: 0,
        processed_repos: 0,
    };

    emit_sync_status(
        &mut cache,
        cache_path,
        options.progress,
        &mut status_state,
        SyncAction::Starting,
        None,
        None,
        true,
        summary,
    )?;

    info!(
        provider = %target.provider,
        scope = ?target.scope,
        "starting sync for target"
    );

    let (mut repos, used_cache) =
        load_repos_with_cache(provider, target, &mut cache, options.refresh)?;
    if options.detect_missing && !used_cache {
        let current_ids: HashSet<String> = repos.iter().map(|repo| repo.id.clone()).collect();
        let mut missing_events: Vec<(DeletedRepoAction, String, String)> = Vec::new();
        let mut on_missing = |action: DeletedRepoAction, repo: &crate::deleted::DeletedRepo| {
            missing_events.push((action, repo.repo_id.to_string(), repo.entry.name.clone()));
        };
        let missing_summary = handle_missing_repos(
            &mut cache,
            root,
            &current_ids,
            options.missing_policy,
            options.missing_decider,
            Some(&mut on_missing),
        )?;
        summary.record_missing(missing_summary);
        for (action, repo_id, repo_name) in missing_events {
            let sync_action = match action {
                DeletedRepoAction::Archive => SyncAction::MissingArchived,
                DeletedRepoAction::Remove => SyncAction::MissingRemoved,
                DeletedRepoAction::Skip => SyncAction::MissingSkipped,
            };
            emit_sync_status(
                &mut cache,
                cache_path,
                options.progress,
                &mut status_state,
                sync_action,
                Some(&repo_name),
                Some(&repo_id),
                true,
                summary,
            )?;
        }
    }
    if let Some(filter) = options.repo_filter {
        repos.retain(|repo| filter(repo));
    }
    status_state.total_repos = repos.len();
    emit_sync_status(
        &mut cache,
        cache_path,
        options.progress,
        &mut status_state,
        SyncAction::Syncing,
        None,
        None,
        true,
        summary,
    )?;

    let mut work_items: Vec<RepoWorkItem> = Vec::new();
    for repo in repos {
        let path = repo_path(root, &repo.provider, &repo.scope, &repo.name);
        if let Some(entry) = cache.repos.get(&repo.id) {
            let cached_path = PathBuf::from(&entry.path);
            if cached_path != path {
                if cached_path.exists() && !path.exists() {
                    if let Err(err) = move_repo_path(&cached_path, &path) {
                        warn!(
                            repo_id = %repo.id,
                            from = %cached_path.display(),
                            to = %path.display(),
                            error = %err,
                            "failed to move repo after rename"
                        );
                    } else {
                        info!(
                            repo_id = %repo.id,
                            from = %cached_path.display(),
                            to = %path.display(),
                            "moved repo to match rename"
                        );
                    }
                } else if !cached_path.exists() {
                    info!(
                        repo_id = %repo.id,
                        from = %cached_path.display(),
                        to = %path.display(),
                        "cached repo path missing; updating to new path"
                    );
                }
            }
        }
        work_items.push(RepoWorkItem { repo, path });
    }

    let jobs = options.jobs.max(1).min(work_items.len().max(1));
    if jobs <= 1 {
        for item in work_items {
            emit_sync_status(
                &mut cache,
                cache_path,
                options.progress,
                &mut status_state,
                SyncAction::Syncing,
                Some(&item.repo.name),
                Some(&item.repo.id),
                true,
                summary,
            )?;
            let outcome = match sync_repo(
                &item.path,
                &item.repo.clone_url,
                &item.repo.default_branch,
                item.repo.auth.as_ref(),
                options.verify,
            ) {
                Ok(outcome) => outcome,
                Err(err) => {
                    summary.failed += 1;
                    status_state.processed_repos += 1;
                    emit_sync_status(
                        &mut cache,
                        cache_path,
                        options.progress,
                        &mut status_state,
                        SyncAction::Failed,
                        Some(&item.repo.name),
                        Some(&item.repo.id),
                        true,
                        summary,
                    )?;
                    warn!(
                        provider = %item.repo.provider,
                        scope = ?item.repo.scope,
                        repo_id = %item.repo.id,
                        path = %item.path.display(),
                        error = %err,
                        "repo sync failed"
                    );
                    cache.record_repo(
                        item.repo.id.clone(),
                        item.repo.name.clone(),
                        item.repo.provider.clone(),
                        item.repo.scope.clone(),
                        item.path.display().to_string(),
                    );
                    continue;
                }
            };

            info!(
                provider = %item.repo.provider,
                scope = ?item.repo.scope,
                repo_id = %item.repo.id,
                path = %item.path.display(),
                outcome = ?outcome,
                "repo sync outcome"
            );
            summary.record(outcome);
            status_state.processed_repos += 1;
            emit_sync_status(
                &mut cache,
                cache_path,
                options.progress,
                &mut status_state,
                action_from_outcome(outcome),
                Some(&item.repo.name),
                Some(&item.repo.id),
                true,
                summary,
            )?;

            cache.record_repo(
                item.repo.id.clone(),
                item.repo.name.clone(),
                item.repo.provider.clone(),
                item.repo.scope.clone(),
                item.path.display().to_string(),
            );

            if should_record_sync(outcome) {
                cache
                    .last_sync
                    .insert(item.repo.id.clone(), current_timestamp());
            }
        }
    } else {
        let queue = Arc::new(Mutex::new(work_items));
        let (tx, rx) = mpsc::channel::<RepoEvent>();
        for _ in 0..jobs {
            let queue = Arc::clone(&queue);
            let tx = tx.clone();
            let verify = options.verify;
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
                        item.repo.auth.as_ref(),
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
                        &mut cache,
                        cache_path,
                        options.progress,
                        &mut status_state,
                        SyncAction::Syncing,
                        Some(&repo_name),
                        Some(&repo_id),
                        true,
                        summary,
                    )?;
                }
                RepoEvent::Finished { item, outcome } => match outcome {
                    Ok(outcome) => {
                        info!(
                            provider = %item.repo.provider,
                            scope = ?item.repo.scope,
                            repo_id = %item.repo.id,
                            path = %item.path.display(),
                            outcome = ?outcome,
                            "repo sync outcome"
                        );
                        summary.record(outcome);
                        status_state.processed_repos += 1;
                        emit_sync_status(
                            &mut cache,
                            cache_path,
                            options.progress,
                            &mut status_state,
                            action_from_outcome(outcome),
                            Some(&item.repo.name),
                            Some(&item.repo.id),
                            true,
                            summary,
                        )?;
                        cache.record_repo(
                            item.repo.id.clone(),
                            item.repo.name.clone(),
                            item.repo.provider.clone(),
                            item.repo.scope.clone(),
                            item.path.display().to_string(),
                        );
                        if should_record_sync(outcome) {
                            cache
                                .last_sync
                                .insert(item.repo.id.clone(), current_timestamp());
                        }
                    }
                    Err(err) => {
                        summary.failed += 1;
                        status_state.processed_repos += 1;
                        emit_sync_status(
                            &mut cache,
                            cache_path,
                            options.progress,
                            &mut status_state,
                            SyncAction::Failed,
                            Some(&item.repo.name),
                            Some(&item.repo.id),
                            true,
                            summary,
                        )?;
                        warn!(
                            provider = %item.repo.provider,
                            scope = ?item.repo.scope,
                            repo_id = %item.repo.id,
                            path = %item.path.display(),
                            error = %err,
                            "repo sync failed"
                        );
                        cache.record_repo(
                            item.repo.id.clone(),
                            item.repo.name.clone(),
                            item.repo.provider.clone(),
                            item.repo.scope.clone(),
                            item.path.display().to_string(),
                        );
                    }
                },
            }
        }
    }

    emit_sync_status(
        &mut cache,
        cache_path,
        options.progress,
        &mut status_state,
        SyncAction::Done,
        None,
        None,
        false,
        summary,
    )?;
    cache.save(cache_path).context("save cache")?;
    Ok(summary)
}

#[cfg(test)]
mod tests {
    use super::*;
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

        move_repo_path(&from, &to).unwrap();
        assert!(!from.exists());
        assert!(to.join("file.txt").exists());
    }

    #[test]
    fn current_timestamp_secs_is_monotonicish() {
        let first = crate::sync_engine_status::current_timestamp_secs();
        let second = crate::sync_engine_status::current_timestamp_secs();
        assert!(second >= first);
    }
}

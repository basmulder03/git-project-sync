use crate::archive::{archive_repo, remove_repo};
use crate::cache::{RepoCache, RepoInventoryEntry, RepoInventoryRepo, SyncSummarySnapshot};
use crate::config::target_id;
use crate::deleted::{DeletedRepoAction, MissingRemotePolicy, detect_deleted_repos};
use crate::git_sync::{SyncOutcome, sync_repo};
use crate::model::{ProviderTarget, RemoteRepo};
use crate::paths::repo_path;
use crate::provider::RepoProvider;
use anyhow::Context;
use std::collections::HashSet;
use std::fs;
use std::path::{Path, PathBuf};
use std::sync::{Arc, Mutex, mpsc};
use std::time::{Duration, Instant, SystemTime, UNIX_EPOCH};
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
        self.missing_archived += missing.archived;
        self.missing_removed += missing.removed;
        self.missing_skipped += missing.skipped;
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

fn move_repo_path(from: &Path, to: &Path) -> anyhow::Result<()> {
    if let Some(parent) = to.parent() {
        fs::create_dir_all(parent).context("create repo rename parent")?;
    }
    if let Err(err) = fs::rename(from, to) {
        copy_dir_recursive(from, to)
            .with_context(|| format!("copy repo after rename failed: {err}"))?;
        fs::remove_dir_all(from).context("remove old repo path after rename copy")?;
    }
    Ok(())
}

fn copy_dir_recursive(source: &Path, destination: &Path) -> anyhow::Result<()> {
    if !source.exists() {
        return Ok(());
    }
    if let Some(parent) = destination.parent() {
        fs::create_dir_all(parent).context("create repo copy parent")?;
    }
    fs::create_dir_all(destination).context("create repo copy dir")?;
    for entry in fs::read_dir(source).context("read repo source dir")? {
        let entry = entry.context("read repo entry")?;
        let file_type = entry.file_type().context("read repo entry type")?;
        let from = entry.path();
        let to = destination.join(entry.file_name());
        if file_type.is_dir() {
            copy_dir_recursive(&from, &to)?;
        } else if file_type.is_file() {
            fs::copy(&from, &to).with_context(|| format!("copy repo file {}", from.display()))?;
        }
    }
    Ok(())
}

fn load_repos_with_cache(
    provider: &dyn RepoProvider,
    target: &ProviderTarget,
    cache: &mut RepoCache,
    refresh: bool,
) -> anyhow::Result<(Vec<RemoteRepo>, bool)> {
    let target_key = target_id(
        target.provider.clone(),
        target.host.as_deref(),
        &target.scope,
    );
    let now = current_timestamp_secs();
    if !refresh
        && let Some(entry) = cache.repo_inventory.get(&target_key)
        && cache_is_valid(entry, now)
    {
        let auth = provider.auth_for_target(target)?;
        let repos = entry
            .repos
            .iter()
            .cloned()
            .map(|repo| RemoteRepo {
                id: repo.id,
                name: repo.name,
                clone_url: repo.clone_url,
                default_branch: repo.default_branch,
                archived: repo.archived,
                provider: repo.provider,
                scope: repo.scope,
                auth: auth.clone(),
            })
            .collect();
        return Ok((repos, true));
    }

    let repos = provider.list_repos(target).context("list repos")?;
    let inventory = RepoInventoryEntry {
        fetched_at: now,
        repos: repos
            .iter()
            .map(|repo| RepoInventoryRepo {
                id: repo.id.clone(),
                name: repo.name.clone(),
                clone_url: repo.clone_url.clone(),
                default_branch: repo.default_branch.clone(),
                archived: repo.archived,
                provider: repo.provider.clone(),
                scope: repo.scope.clone(),
            })
            .collect(),
    };
    cache.repo_inventory.insert(target_key, inventory);
    Ok((repos, false))
}

fn cache_is_valid(entry: &RepoInventoryEntry, now: u64) -> bool {
    const TTL_SECS: u64 = 15 * 60;
    now.saturating_sub(entry.fetched_at) <= TTL_SECS
}

fn action_from_outcome(outcome: SyncOutcome) -> SyncAction {
    match outcome {
        SyncOutcome::Cloned => SyncAction::Cloned,
        SyncOutcome::FastForwarded => SyncAction::FastForwarded,
        SyncOutcome::UpToDate => SyncAction::UpToDate,
        SyncOutcome::Dirty => SyncAction::Dirty,
        SyncOutcome::Diverged => SyncAction::Diverged,
    }
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

struct StatusEmitterState {
    target_key: String,
    last_status_flush: Instant,
    status_dirty: bool,
    total_repos: usize,
    processed_repos: usize,
}

struct RepoWorkItem {
    repo: RemoteRepo,
    path: PathBuf,
}

enum RepoEvent {
    Started {
        repo_id: String,
        repo_name: String,
    },
    Finished {
        item: RepoWorkItem,
        outcome: anyhow::Result<SyncOutcome>,
    },
}

#[allow(clippy::too_many_arguments)]
fn emit_sync_status(
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

#[derive(Debug, Default, Clone, Copy)]
struct MissingSummary {
    archived: u32,
    removed: u32,
    skipped: u32,
}

#[allow(clippy::type_complexity)]
fn handle_missing_repos(
    cache: &mut RepoCache,
    root: &Path,
    current_repo_ids: &HashSet<String>,
    policy: MissingRemotePolicy,
    decider: Option<&MissingDecider>,
    mut on_action: Option<&mut dyn FnMut(DeletedRepoAction, &crate::deleted::DeletedRepo)>,
) -> anyhow::Result<MissingSummary> {
    let missing = detect_deleted_repos(cache, current_repo_ids);
    if missing.is_empty() {
        return Ok(MissingSummary::default());
    }

    let mut remove_ids: Vec<String> = Vec::new();
    let mut summary = MissingSummary::default();
    for repo in missing {
        let action = resolve_action(repo.entry, policy, decider)?;
        match action {
            DeletedRepoAction::Archive => {
                let destination = archive_repo(
                    root,
                    repo.entry.provider.clone(),
                    &repo.entry.scope,
                    &repo.entry.name,
                )?;
                info!(
                    provider = %repo.entry.provider,
                    scope = ?repo.entry.scope,
                    repo_id = %repo.repo_id,
                    path = %destination.display(),
                    "archived missing repo"
                );
                if let Some(callback) = on_action.as_mut() {
                    callback(DeletedRepoAction::Archive, &repo);
                }
                remove_ids.push(repo.repo_id.to_string());
                summary.archived += 1;
            }
            DeletedRepoAction::Remove => {
                remove_repo(
                    root,
                    repo.entry.provider.clone(),
                    &repo.entry.scope,
                    &repo.entry.name,
                )?;
                info!(
                    provider = %repo.entry.provider,
                    scope = ?repo.entry.scope,
                    repo_id = %repo.repo_id,
                    "removed missing repo"
                );
                if let Some(callback) = on_action.as_mut() {
                    callback(DeletedRepoAction::Remove, &repo);
                }
                remove_ids.push(repo.repo_id.to_string());
                summary.removed += 1;
            }
            DeletedRepoAction::Skip => {
                warn!(
                    provider = %repo.entry.provider,
                    scope = ?repo.entry.scope,
                    repo_id = %repo.repo_id,
                    "skipped missing repo"
                );
                if let Some(callback) = on_action.as_mut() {
                    callback(DeletedRepoAction::Skip, &repo);
                }
                summary.skipped += 1;
            }
        }
    }

    for repo_id in remove_ids {
        cache.repos.remove(&repo_id);
    }

    Ok(summary)
}

fn resolve_action(
    entry: &crate::cache::RepoCacheEntry,
    policy: MissingRemotePolicy,
    decider: Option<&MissingDecider>,
) -> anyhow::Result<DeletedRepoAction> {
    match policy {
        MissingRemotePolicy::Prompt => {
            let decider = decider.context("missing-remote prompt requires a decider")?;
            decider(entry)
        }
        MissingRemotePolicy::Archive => Ok(DeletedRepoAction::Archive),
        MissingRemotePolicy::Remove => Ok(DeletedRepoAction::Remove),
        MissingRemotePolicy::Skip => Ok(DeletedRepoAction::Skip),
    }
}

fn should_record_sync(outcome: SyncOutcome) -> bool {
    matches!(
        outcome,
        SyncOutcome::Cloned | SyncOutcome::FastForwarded | SyncOutcome::UpToDate
    )
}

fn current_timestamp() -> String {
    let since_epoch = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap_or_default();
    since_epoch.as_secs().to_string()
}

fn current_timestamp_secs() -> u64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap_or_default()
        .as_secs()
}

#[cfg(test)]
mod tests {
    use super::*;
    use tempfile::TempDir;

    #[test]
    fn cache_is_valid_within_ttl() {
        let entry = RepoInventoryEntry {
            fetched_at: 100,
            repos: Vec::new(),
        };
        assert!(cache_is_valid(&entry, 100 + 60));
        assert!(!cache_is_valid(&entry, 100 + 16 * 60));
    }

    #[test]
    fn move_repo_path_moves_directory() {
        let tmp = TempDir::new().unwrap();
        let from = tmp.path().join("old");
        let to = tmp.path().join("new");
        fs::create_dir_all(&from).unwrap();
        fs::write(from.join("file.txt"), "data").unwrap();

        move_repo_path(&from, &to).unwrap();
        assert!(!from.exists());
        assert!(to.join("file.txt").exists());
    }
}

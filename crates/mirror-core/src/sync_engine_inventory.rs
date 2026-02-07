use crate::cache::{RepoCache, RepoInventoryEntry, RepoInventoryRepo};
use crate::config::target_id;
use crate::model::{ProviderTarget, RemoteRepo};
use crate::provider::RepoProvider;
use crate::sync_engine_status::current_timestamp_secs;
use anyhow::Context;

pub(crate) async fn load_repos_with_cache(
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
            })
            .collect();
        return Ok((repos, true));
    }

    let repos = provider.list_repos(target).await.context("list repos")?;
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

pub(crate) fn cache_is_valid(entry: &RepoInventoryEntry, now: u64) -> bool {
    const TTL_SECS: u64 = 15 * 60;
    now.saturating_sub(entry.fetched_at) <= TTL_SECS
}

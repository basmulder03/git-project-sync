use super::*;

pub fn prune_cache_for_targets(path: &Path, target_ids: &[String]) -> anyhow::Result<u32> {
    let mut cache = RepoCache::load(path)?;
    let mut removed = 0;
    cache.repo_inventory.retain(|key, _| {
        let keep = target_ids.contains(key);
        if !keep {
            removed += 1;
        }
        keep
    });
    cache
        .target_last_success
        .retain(|key, _| target_ids.contains(key));
    cache
        .target_backoff_until
        .retain(|key, _| target_ids.contains(key));
    cache
        .target_backoff_attempts
        .retain(|key, _| target_ids.contains(key));
    cache
        .target_sync_status
        .retain(|key, _| target_ids.contains(key));
    cache.save(path)?;
    Ok(removed)
}

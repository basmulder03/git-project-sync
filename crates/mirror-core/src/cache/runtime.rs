use super::*;

pub fn update_target_success(path: &Path, target_key: &str, now: u64) -> anyhow::Result<()> {
    let mut cache = RepoCache::load(path)?;
    cache.record_target_success(target_key.to_string(), now);
    cache.save(path)
}

pub fn update_target_failure(path: &Path, target_key: &str, now: u64) -> anyhow::Result<()> {
    let mut cache = RepoCache::load(path)?;
    cache.record_target_failure(target_key.to_string(), now);
    cache.save(path)
}

pub fn backoff_until(cache: &RepoCache, target_key: &str) -> Option<u64> {
    cache.target_backoff_until.get(target_key).copied()
}

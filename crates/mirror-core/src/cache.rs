use crate::model::{ProviderKind, ProviderScope};
use serde::{Deserialize, Serialize};
use std::collections::HashMap;
use std::fs;
use std::path::Path;

#[derive(Debug, Default, Serialize, Deserialize, PartialEq)]
pub struct RepoCache {
    pub last_sync: HashMap<String, String>,
    pub repos: HashMap<String, RepoCacheEntry>,
    #[serde(default)]
    pub repo_inventory: HashMap<String, RepoInventoryEntry>,
    #[serde(default)]
    pub target_last_success: HashMap<String, u64>,
    #[serde(default)]
    pub target_backoff_until: HashMap<String, u64>,
    #[serde(default)]
    pub target_backoff_attempts: HashMap<String, u32>,
}

impl RepoCache {
    pub fn load(path: &Path) -> anyhow::Result<Self> {
        if !path.exists() {
            return Ok(Self::default());
        }
        let data = fs::read_to_string(path)?;
        let cache = serde_json::from_str(&data)?;
        Ok(cache)
    }

    pub fn save(&self, path: &Path) -> anyhow::Result<()> {
        if let Some(parent) = path.parent() {
            fs::create_dir_all(parent)?;
        }
        let data = serde_json::to_string_pretty(self)?;
        fs::write(path, data)?;
        Ok(())
    }

    pub fn record_repo(
        &mut self,
        repo_id: String,
        name: String,
        provider: ProviderKind,
        scope: ProviderScope,
        path: String,
    ) {
        self.repos.insert(
            repo_id,
            RepoCacheEntry {
                name,
                provider,
                scope,
                path,
            },
        );
    }

    pub fn record_target_success(&mut self, target_key: String, now: u64) {
        self.target_last_success.insert(target_key.clone(), now);
        self.target_backoff_until.remove(&target_key);
        self.target_backoff_attempts.remove(&target_key);
    }

    pub fn record_target_failure(&mut self, target_key: String, now: u64) {
        let attempts = self.target_backoff_attempts.entry(target_key.clone()).or_insert(0);
        *attempts = attempts.saturating_add(1);
        let delay = compute_backoff_delay(*attempts);
        self.target_backoff_until
            .insert(target_key, now.saturating_add(delay));
    }
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub struct RepoCacheEntry {
    pub name: String,
    pub provider: ProviderKind,
    pub scope: ProviderScope,
    pub path: String,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub struct RepoInventoryEntry {
    pub fetched_at: u64,
    pub repos: Vec<RepoInventoryRepo>,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub struct RepoInventoryRepo {
    pub id: String,
    pub name: String,
    pub clone_url: String,
    pub default_branch: String,
    #[serde(default)]
    pub archived: bool,
    pub provider: ProviderKind,
    pub scope: ProviderScope,
}

pub fn update_target_success(
    path: &Path,
    target_key: &str,
    now: u64,
) -> anyhow::Result<()> {
    let mut cache = RepoCache::load(path)?;
    cache.record_target_success(target_key.to_string(), now);
    cache.save(path)
}

pub fn update_target_failure(
    path: &Path,
    target_key: &str,
    now: u64,
) -> anyhow::Result<()> {
    let mut cache = RepoCache::load(path)?;
    cache.record_target_failure(target_key.to_string(), now);
    cache.save(path)
}

pub fn backoff_until(cache: &RepoCache, target_key: &str) -> Option<u64> {
    cache.target_backoff_until.get(target_key).copied()
}

fn compute_backoff_delay(attempts: u32) -> u64 {
    const BASE: u64 = 60;
    const MAX: u64 = 3600;
    let exp = attempts.saturating_sub(1).min(10);
    let delay = BASE.saturating_mul(2u64.saturating_pow(exp));
    delay.min(MAX)
}

#[cfg(test)]
mod tests {
    use super::*;
    use tempfile::TempDir;

    #[test]
    fn cache_roundtrip() {
        let tmp = TempDir::new().unwrap();
        let path = tmp.path().join("cache.json");
        let mut cache = RepoCache::default();
        cache
            .last_sync
            .insert("repo-1".into(), "2025-01-01T00:00:00Z".into());
        cache.repo_inventory.insert(
            "target-1".into(),
            RepoInventoryEntry {
                fetched_at: 1,
                repos: vec![RepoInventoryRepo {
                    id: "repo-1".into(),
                    name: "Repo One".into(),
                    clone_url: "https://example.com/repo.git".into(),
                    default_branch: "main".into(),
                    archived: true,
                    provider: ProviderKind::AzureDevOps,
                    scope: ProviderScope::new(vec!["org".into(), "project".into()]).unwrap(),
                }],
            },
        );
        cache.target_last_success.insert("target-1".into(), 1);
        cache.target_backoff_until.insert("target-2".into(), 2);
        cache.target_backoff_attempts.insert("target-2".into(), 3);
        cache.record_repo(
            "repo-1".into(),
            "Repo One".into(),
            ProviderKind::AzureDevOps,
            ProviderScope::new(vec!["org".into(), "project".into()]).unwrap(),
            "D:\\root\\azure-devops\\org\\project\\Repo One".into(),
        );
        cache.save(&path).unwrap();

        let loaded = RepoCache::load(&path).unwrap();
        assert_eq!(cache, loaded);
    }

    #[test]
    fn record_target_failure_sets_backoff() {
        let mut cache = RepoCache::default();
        cache.record_target_failure("target-1".into(), 100);
        let until = cache.target_backoff_until.get("target-1").copied().unwrap();
        assert!(until >= 160);
        let attempts = cache.target_backoff_attempts.get("target-1").copied().unwrap();
        assert_eq!(attempts, 1);
    }

    #[test]
    fn record_target_success_clears_backoff() {
        let mut cache = RepoCache::default();
        cache.target_backoff_until.insert("target-1".into(), 200);
        cache.target_backoff_attempts.insert("target-1".into(), 2);
        cache.record_target_success("target-1".into(), 300);
        assert_eq!(cache.target_last_success.get("target-1").copied(), Some(300));
        assert!(!cache.target_backoff_until.contains_key("target-1"));
        assert!(!cache.target_backoff_attempts.contains_key("target-1"));
    }
}

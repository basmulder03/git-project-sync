use crate::model::{ProviderKind, ProviderScope};
use crate::repo_status::RepoLocalStatus;
use serde::{Deserialize, Serialize};
use std::collections::HashMap;
use std::fs;
use std::path::Path;

#[derive(Debug, Default, Serialize, Deserialize, PartialEq)]
pub struct RepoCache {
    pub version: u32,
    pub last_sync: HashMap<String, String>,
    pub repos: HashMap<String, RepoCacheEntry>,
    #[serde(default)]
    pub repo_inventory: HashMap<String, RepoInventoryEntry>,
    #[serde(default)]
    pub repo_status: HashMap<String, RepoLocalStatus>,
    #[serde(default)]
    pub target_last_success: HashMap<String, u64>,
    #[serde(default)]
    pub target_backoff_until: HashMap<String, u64>,
    #[serde(default)]
    pub target_backoff_attempts: HashMap<String, u32>,
    #[serde(default)]
    pub target_sync_status: HashMap<String, SyncStatus>,
    #[serde(default)]
    pub update_last_check: Option<u64>,
    #[serde(default)]
    pub update_last_result: Option<String>,
    #[serde(default)]
    pub update_last_version: Option<String>,
    #[serde(default)]
    pub update_last_source: Option<String>,
    #[serde(default)]
    pub token_last_check: Option<u64>,
    #[serde(default)]
    pub token_last_source: Option<String>,
    #[serde(default)]
    pub token_status: HashMap<String, TokenStatus>,
}

impl RepoCache {
    pub fn new() -> Self {
        Self {
            version: 4,
            last_sync: HashMap::new(),
            repos: HashMap::new(),
            repo_inventory: HashMap::new(),
            repo_status: HashMap::new(),
            target_last_success: HashMap::new(),
            target_backoff_until: HashMap::new(),
            target_backoff_attempts: HashMap::new(),
            target_sync_status: HashMap::new(),
            update_last_check: None,
            update_last_result: None,
            update_last_version: None,
            update_last_source: None,
            token_last_check: None,
            token_last_source: None,
            token_status: HashMap::new(),
        }
    }

    pub fn load(path: &Path) -> anyhow::Result<Self> {
        if !path.exists() {
            return Ok(Self::new());
        }
        let data = fs::read_to_string(path)?;
        let json: serde_json::Value = serde_json::from_str(&data)?;
        match json.get("version").and_then(|value| value.as_u64()) {
            Some(4) => Ok(serde_json::from_value(json)?),
            Some(3) => Ok(migrate_v3(json)?),
            Some(2) => Ok(migrate_v2(json)?),
            Some(1) | None => Ok(migrate_v1(json)?),
            Some(other) => anyhow::bail!("unsupported cache version {other}"),
        }
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
        let attempts = self
            .target_backoff_attempts
            .entry(target_key.clone())
            .or_insert(0);
        *attempts = attempts.saturating_add(1);
        let delay = compute_backoff_delay(*attempts);
        self.target_backoff_until
            .insert(target_key, now.saturating_add(delay));
    }
}

#[derive(Debug, Serialize, Deserialize, PartialEq)]
struct RepoCacheV1 {
    last_sync: HashMap<String, String>,
    repos: HashMap<String, RepoCacheEntry>,
}

#[derive(Debug, Serialize, Deserialize, PartialEq)]
struct RepoCacheV2 {
    version: u32,
    last_sync: HashMap<String, String>,
    repos: HashMap<String, RepoCacheEntry>,
    #[serde(default)]
    repo_inventory: HashMap<String, RepoInventoryEntry>,
    #[serde(default)]
    repo_status: HashMap<String, RepoLocalStatus>,
    #[serde(default)]
    target_last_success: HashMap<String, u64>,
    #[serde(default)]
    target_backoff_until: HashMap<String, u64>,
    #[serde(default)]
    target_backoff_attempts: HashMap<String, u32>,
    #[serde(default)]
    target_sync_status: HashMap<String, SyncStatus>,
}

#[derive(Debug, Serialize, Deserialize, PartialEq)]
struct RepoCacheV3 {
    version: u32,
    last_sync: HashMap<String, String>,
    repos: HashMap<String, RepoCacheEntry>,
    #[serde(default)]
    repo_inventory: HashMap<String, RepoInventoryEntry>,
    #[serde(default)]
    repo_status: HashMap<String, RepoLocalStatus>,
    #[serde(default)]
    target_last_success: HashMap<String, u64>,
    #[serde(default)]
    target_backoff_until: HashMap<String, u64>,
    #[serde(default)]
    target_backoff_attempts: HashMap<String, u32>,
    #[serde(default)]
    target_sync_status: HashMap<String, SyncStatus>,
    #[serde(default)]
    update_last_check: Option<u64>,
    #[serde(default)]
    update_last_result: Option<String>,
    #[serde(default)]
    update_last_version: Option<String>,
    #[serde(default)]
    update_last_source: Option<String>,
}

fn migrate_v1(json: serde_json::Value) -> anyhow::Result<RepoCache> {
    let v1: RepoCacheV1 = serde_json::from_value(json)?;
    Ok(RepoCache {
        version: 4,
        last_sync: v1.last_sync,
        repos: v1.repos,
        repo_inventory: HashMap::new(),
        repo_status: HashMap::new(),
        target_last_success: HashMap::new(),
        target_backoff_until: HashMap::new(),
        target_backoff_attempts: HashMap::new(),
        target_sync_status: HashMap::new(),
        update_last_check: None,
        update_last_result: None,
        update_last_version: None,
        update_last_source: None,
        token_last_check: None,
        token_last_source: None,
        token_status: HashMap::new(),
    })
}

fn migrate_v2(json: serde_json::Value) -> anyhow::Result<RepoCache> {
    let v2: RepoCacheV2 = serde_json::from_value(json)?;
    Ok(RepoCache {
        version: 4,
        last_sync: v2.last_sync,
        repos: v2.repos,
        repo_inventory: v2.repo_inventory,
        repo_status: v2.repo_status,
        target_last_success: v2.target_last_success,
        target_backoff_until: v2.target_backoff_until,
        target_backoff_attempts: v2.target_backoff_attempts,
        target_sync_status: v2.target_sync_status,
        update_last_check: None,
        update_last_result: None,
        update_last_version: None,
        update_last_source: None,
        token_last_check: None,
        token_last_source: None,
        token_status: HashMap::new(),
    })
}

fn migrate_v3(json: serde_json::Value) -> anyhow::Result<RepoCache> {
    let v3: RepoCacheV3 = serde_json::from_value(json)?;
    Ok(RepoCache {
        version: 4,
        last_sync: v3.last_sync,
        repos: v3.repos,
        repo_inventory: v3.repo_inventory,
        repo_status: v3.repo_status,
        target_last_success: v3.target_last_success,
        target_backoff_until: v3.target_backoff_until,
        target_backoff_attempts: v3.target_backoff_attempts,
        target_sync_status: v3.target_sync_status,
        update_last_check: v3.update_last_check,
        update_last_result: v3.update_last_result,
        update_last_version: v3.update_last_version,
        update_last_source: v3.update_last_source,
        token_last_check: None,
        token_last_source: None,
        token_status: HashMap::new(),
    })
}

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

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Default)]
pub struct SyncSummarySnapshot {
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

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Default)]
pub struct SyncStatus {
    pub in_progress: bool,
    pub last_action: Option<String>,
    pub last_repo: Option<String>,
    pub last_repo_id: Option<String>,
    pub last_updated: u64,
    #[serde(default)]
    pub total_repos: usize,
    #[serde(default)]
    pub processed_repos: usize,
    #[serde(default)]
    pub summary: SyncSummarySnapshot,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub struct TokenStatus {
    pub last_checked: u64,
    pub status: String,
    pub error: Option<String>,
}

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

pub fn token_check_due(cache: &RepoCache, now: u64, interval_secs: u64) -> bool {
    match cache.token_last_check {
        Some(last) => now.saturating_sub(last) >= interval_secs,
        None => true,
    }
}

pub fn update_check_due(cache: &RepoCache, now: u64, interval_secs: u64) -> bool {
    match cache.update_last_check {
        Some(last) => now.saturating_sub(last) >= interval_secs,
        None => true,
    }
}

pub fn record_update_check(
    cache: &mut RepoCache,
    now: u64,
    result: String,
    latest_version: Option<String>,
    source: &str,
) {
    cache.update_last_check = Some(now);
    cache.update_last_result = Some(result);
    cache.update_last_version = latest_version;
    cache.update_last_source = Some(source.to_string());
}

pub fn record_token_check(cache: &mut RepoCache, now: u64, source: &str) {
    cache.token_last_check = Some(now);
    cache.token_last_source = Some(source.to_string());
}

pub fn record_token_status(cache: &mut RepoCache, account: String, status: TokenStatus) {
    cache.token_status.insert(account, status);
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
        let mut cache = RepoCache::new();
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
        cache.target_sync_status.insert(
            "target-1".into(),
            SyncStatus {
                in_progress: true,
                last_action: Some("syncing".to_string()),
                last_repo: Some("Repo One".to_string()),
                last_repo_id: Some("repo-1".to_string()),
                last_updated: 10,
                total_repos: 5,
                processed_repos: 2,
                summary: SyncSummarySnapshot {
                    cloned: 1,
                    ..SyncSummarySnapshot::default()
                },
            },
        );
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
    fn migrates_v1_cache() {
        let tmp = TempDir::new().unwrap();
        let path = tmp.path().join("cache.json");
        let mut v1 = RepoCacheV1 {
            last_sync: HashMap::new(),
            repos: HashMap::new(),
        };
        v1.last_sync.insert("repo-1".into(), "1".into());
        let data = serde_json::to_string_pretty(&v1).unwrap();
        fs::write(&path, data).unwrap();

        let loaded = RepoCache::load(&path).unwrap();
        assert_eq!(loaded.version, 4);
        assert!(loaded.last_sync.contains_key("repo-1"));
        assert!(loaded.target_sync_status.is_empty());
    }

    #[test]
    fn prune_cache_removes_stale_targets() {
        let tmp = TempDir::new().unwrap();
        let path = tmp.path().join("cache.json");
        let mut cache = RepoCache::new();
        cache.repo_inventory.insert(
            "keep".into(),
            RepoInventoryEntry {
                fetched_at: 1,
                repos: Vec::new(),
            },
        );
        cache.repo_inventory.insert(
            "drop".into(),
            RepoInventoryEntry {
                fetched_at: 1,
                repos: Vec::new(),
            },
        );
        cache
            .target_sync_status
            .insert("keep".into(), SyncStatus::default());
        cache
            .target_sync_status
            .insert("drop".into(), SyncStatus::default());
        cache.save(&path).unwrap();
        let removed = prune_cache_for_targets(&path, &["keep".into()]).unwrap();
        assert_eq!(removed, 1);
        let loaded = RepoCache::load(&path).unwrap();
        assert!(loaded.repo_inventory.contains_key("keep"));
        assert!(!loaded.repo_inventory.contains_key("drop"));
        assert!(loaded.target_sync_status.contains_key("keep"));
        assert!(!loaded.target_sync_status.contains_key("drop"));
    }

    #[test]
    fn record_target_failure_sets_backoff() {
        let mut cache = RepoCache::default();
        cache.record_target_failure("target-1".into(), 100);
        let until = cache.target_backoff_until.get("target-1").copied().unwrap();
        assert!(until >= 160);
        let attempts = cache
            .target_backoff_attempts
            .get("target-1")
            .copied()
            .unwrap();
        assert_eq!(attempts, 1);
    }

    #[test]
    fn record_target_success_clears_backoff() {
        let mut cache = RepoCache::default();
        cache.target_backoff_until.insert("target-1".into(), 200);
        cache.target_backoff_attempts.insert("target-1".into(), 2);
        cache.record_target_success("target-1".into(), 300);
        assert_eq!(
            cache.target_last_success.get("target-1").copied(),
            Some(300)
        );
        assert!(!cache.target_backoff_until.contains_key("target-1"));
        assert!(!cache.target_backoff_attempts.contains_key("target-1"));
    }
}

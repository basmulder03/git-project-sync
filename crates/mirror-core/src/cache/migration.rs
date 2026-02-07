use super::*;

#[derive(Debug, Serialize, Deserialize, PartialEq)]
pub(super) struct RepoCacheV0 {
    #[serde(default)]
    pub(super) last_sync: HashMap<String, String>,
    #[serde(default)]
    pub(super) repos: HashMap<String, RepoCacheEntry>,
}

#[derive(Debug, Serialize, Deserialize, PartialEq)]
pub(super) struct RepoCacheV1 {
    pub(super) last_sync: HashMap<String, String>,
    pub(super) repos: HashMap<String, RepoCacheEntry>,
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

pub(super) fn migrate_v1(json: serde_json::Value) -> anyhow::Result<RepoCache> {
    let v1: RepoCacheV1 = serde_json::from_value(json)?;
    Ok(migrate_from_last_sync_repos(v1.last_sync, v1.repos))
}

pub(super) fn migrate_v0(json: serde_json::Value) -> anyhow::Result<RepoCache> {
    let v0: RepoCacheV0 = serde_json::from_value(json)?;
    Ok(migrate_from_last_sync_repos(v0.last_sync, v0.repos))
}

fn migrate_from_last_sync_repos(
    last_sync: HashMap<String, String>,
    repos: HashMap<String, RepoCacheEntry>,
) -> RepoCache {
    RepoCache {
        version: 4,
        last_sync,
        repos,
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

pub(super) fn migrate_v2(json: serde_json::Value) -> anyhow::Result<RepoCache> {
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

pub(super) fn migrate_v3(json: serde_json::Value) -> anyhow::Result<RepoCache> {
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

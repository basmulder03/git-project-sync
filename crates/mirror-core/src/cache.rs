use crate::model::{ProviderKind, ProviderScope};
use serde::{Deserialize, Serialize};
use std::collections::HashMap;
use std::fs;
use std::path::Path;

#[derive(Debug, Default, Serialize, Deserialize, PartialEq)]
pub struct RepoCache {
    pub last_sync: HashMap<String, String>,
    pub repos: HashMap<String, RepoCacheEntry>,
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
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub struct RepoCacheEntry {
    pub name: String,
    pub provider: ProviderKind,
    pub scope: ProviderScope,
    pub path: String,
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
}

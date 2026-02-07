use crate::model::{ProviderKind, ProviderScope, ProviderTarget};
use anyhow::Context;
use directories::ProjectDirs;
use serde::{Deserialize, Serialize};
use sha2::{Digest, Sha256};
use std::fs;
use std::path::{Path, PathBuf};

#[derive(Debug, Default, Serialize, Deserialize)]
pub struct AppConfigV1 {
    pub root: Option<PathBuf>,
    pub targets: Vec<ProviderTarget>,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct AppConfigV2 {
    pub version: u32,
    pub root: Option<PathBuf>,
    pub targets: Vec<TargetConfig>,
    #[serde(default)]
    pub language: Option<String>,
}

impl Default for AppConfigV2 {
    fn default() -> Self {
        Self {
            version: 2,
            root: None,
            targets: Vec::new(),
            language: None,
        }
    }
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct TargetConfig {
    pub id: String,
    pub provider: ProviderKind,
    pub scope: ProviderScope,
    pub host: Option<String>,
    #[serde(default)]
    pub labels: Vec<String>,
}

impl AppConfigV2 {
    pub fn save(&self, path: &Path) -> anyhow::Result<()> {
        if let Some(parent) = path.parent() {
            fs::create_dir_all(parent).context("create config directory")?;
        }
        let data = serde_json::to_string_pretty(self).context("serialize config")?;
        fs::write(path, data).context("write config")?;
        Ok(())
    }
}

pub fn default_config_path() -> anyhow::Result<PathBuf> {
    let project = ProjectDirs::from("com", "git-project-sync", "git-project-sync")
        .context("resolve project dirs")?;
    Ok(project.config_dir().join("config.json"))
}

pub fn default_cache_path() -> anyhow::Result<PathBuf> {
    let project = ProjectDirs::from("com", "git-project-sync", "git-project-sync")
        .context("resolve project dirs")?;
    Ok(project.cache_dir().join("cache.json"))
}

pub fn default_lock_path() -> anyhow::Result<PathBuf> {
    let project = ProjectDirs::from("com", "git-project-sync", "git-project-sync")
        .context("resolve project dirs")?;
    Ok(project.data_local_dir().join("mirror.lock"))
}

pub fn load_or_migrate(path: &Path) -> anyhow::Result<(AppConfigV2, bool)> {
    if !path.exists() {
        return Ok((AppConfigV2::default(), false));
    }
    let data = fs::read_to_string(path).context("read config")?;
    let json: serde_json::Value = serde_json::from_str(&data).context("parse config json")?;
    match json.get("version").and_then(|value| value.as_u64()) {
        Some(2) => {
            let config = serde_json::from_value(json).context("parse config v2")?;
            Ok((config, false))
        }
        _ => {
            let v1: AppConfigV1 = serde_json::from_value(json).context("parse config v1")?;
            Ok((migrate_v1(v1), true))
        }
    }
}

pub fn target_id(provider: ProviderKind, host: Option<&str>, scope: &ProviderScope) -> String {
    let host = host.unwrap_or("<default>");
    let payload = format!(
        "{}:{}:{}",
        provider.as_prefix(),
        host,
        scope.segments().join("/")
    );
    let mut hasher = Sha256::new();
    hasher.update(payload.as_bytes());
    let digest = hasher.finalize();
    hex::encode(digest)
}

fn migrate_v1(v1: AppConfigV1) -> AppConfigV2 {
    let targets = v1
        .targets
        .into_iter()
        .map(|target| TargetConfig {
            id: target_id(
                target.provider.clone(),
                target.host.as_deref(),
                &target.scope,
            ),
            provider: target.provider,
            scope: target.scope,
            host: target.host,
            labels: Vec::new(),
        })
        .collect();
    AppConfigV2 {
        version: 2,
        root: v1.root,
        targets,
        language: None,
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use tempfile::TempDir;

    #[test]
    fn target_id_is_stable() {
        let scope = ProviderScope::new(vec!["org".into(), "project".into()]).unwrap();
        let id_a = target_id(ProviderKind::AzureDevOps, None, &scope);
        let id_b = target_id(ProviderKind::AzureDevOps, None, &scope);
        assert_eq!(id_a, id_b);
    }

    #[test]
    fn migrate_v1_to_v2() {
        let scope = ProviderScope::new(vec!["org".into(), "project".into()]).unwrap();
        let v1 = AppConfigV1 {
            root: Some(PathBuf::from("/tmp")),
            targets: vec![ProviderTarget {
                provider: ProviderKind::AzureDevOps,
                scope: scope.clone(),
                host: None,
            }],
        };

        let v2 = migrate_v1(v1);
        assert_eq!(v2.version, 2);
        assert_eq!(v2.targets.len(), 1);
        assert_eq!(v2.targets[0].provider, ProviderKind::AzureDevOps);
        assert_eq!(v2.targets[0].scope, scope);
    }

    #[test]
    fn load_or_migrate_handles_v1() {
        let tmp = TempDir::new().unwrap();
        let path = tmp.path().join("config.json");
        let scope = ProviderScope::new(vec!["org".into(), "project".into()]).unwrap();
        let v1 = AppConfigV1 {
            root: Some(PathBuf::from("/tmp")),
            targets: vec![ProviderTarget {
                provider: ProviderKind::AzureDevOps,
                scope,
                host: None,
            }],
        };
        let data = serde_json::to_string_pretty(&v1).unwrap();
        fs::write(&path, data).unwrap();

        let (config, migrated) = load_or_migrate(&path).unwrap();
        assert!(migrated);
        assert_eq!(config.version, 2);
        assert_eq!(config.targets.len(), 1);
    }

    #[test]
    fn config_roundtrip_preserves_language() {
        let tmp = TempDir::new().unwrap();
        let path = tmp.path().join("config.json");
        let config = AppConfigV2 {
            version: 2,
            root: Some(PathBuf::from("/tmp")),
            targets: Vec::new(),
            language: Some("nl".to_string()),
        };
        config.save(&path).unwrap();

        let (loaded, migrated) = load_or_migrate(&path).unwrap();
        assert!(!migrated);
        assert_eq!(loaded.language.as_deref(), Some("nl"));
    }
}

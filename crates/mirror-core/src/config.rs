use crate::model::ProviderTarget;
use anyhow::Context;
use directories::ProjectDirs;
use serde::{Deserialize, Serialize};
use std::fs;
use std::path::{Path, PathBuf};

#[derive(Debug, Default, Serialize, Deserialize)]
pub struct AppConfig {
    pub root: Option<PathBuf>,
    pub targets: Vec<ProviderTarget>,
}

impl AppConfig {
    pub fn load(path: &Path) -> anyhow::Result<Self> {
        if !path.exists() {
            return Ok(Self::default());
        }
        let data = fs::read_to_string(path).context("read config")?;
        let config = serde_json::from_str(&data).context("parse config")?;
        Ok(config)
    }

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
    Ok(project.runtime_dir().unwrap_or(project.cache_dir()).join("mirror.lock"))
}

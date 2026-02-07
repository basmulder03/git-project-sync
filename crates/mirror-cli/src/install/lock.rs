use anyhow::Context;
use directories::ProjectDirs;
use std::path::PathBuf;
use std::sync::Arc;

use super::{INSTALL_LOCK, InstallGuard};

pub fn acquire_install_lock() -> anyhow::Result<InstallGuard> {
    let cell = INSTALL_LOCK.get_or_init(|| std::sync::Mutex::new(None));
    let mut guard = cell.lock().expect("install lock mutex");
    if let Some(lock) = guard.as_ref() {
        return Ok(InstallGuard { lock: lock.clone() });
    }
    let path = install_lock_path()?;
    let lock = mirror_core::lockfile::LockFile::try_acquire(&path)?
        .ok_or_else(|| anyhow::anyhow!("installer already running"))?;
    let lock = Arc::new(lock);
    *guard = Some(lock.clone());
    Ok(InstallGuard { lock })
}

fn install_lock_path() -> anyhow::Result<PathBuf> {
    let project = ProjectDirs::from("com", "git-project-sync", "git-project-sync")
        .context("resolve project dirs")?;
    Ok(project.data_local_dir().join("install.lock"))
}

use anyhow::Context;
use directories::ProjectDirs;
#[cfg(unix)]
use directories::BaseDirs;
use mirror_core::lockfile::LockFile;
use std::path::{Path, PathBuf};
use std::process::Command;

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum PathChoice {
    Add,
    Skip,
}

#[derive(Clone, Copy, Debug)]
pub struct InstallOptions {
    pub delayed_start: Option<u64>,
    pub path_choice: PathChoice,
}

#[derive(Clone, Debug)]
pub struct InstallReport {
    pub service: String,
    pub path: String,
}

pub struct InstallGuard {
    _lock: LockFile,
}

pub fn perform_install(exec_path: &Path, options: InstallOptions) -> anyhow::Result<InstallReport> {
    mirror_core::service::install_service_with_delay(exec_path, options.delayed_start)?;
    let service = match options.delayed_start {
        Some(delay) if delay > 0 => format!("Service installed with delayed start ({delay}s)"),
        _ => "Service installed".to_string(),
    };
    let path = match options.path_choice {
        PathChoice::Add => register_path(exec_path)?,
        PathChoice::Skip => "PATH update skipped".to_string(),
    };
    write_marker()?;
    Ok(InstallReport { service, path })
}

pub fn register_path(exec_path: &Path) -> anyhow::Result<String> {
    let dir = exec_path
        .parent()
        .ok_or_else(|| anyhow::anyhow!("executable path has no parent"))?;
    if cfg!(target_os = "windows") {
        return add_path_windows(dir);
    }
    add_path_unix(dir)
}

#[cfg(unix)]
fn add_path_unix(dir: &Path) -> anyhow::Result<String> {
    let base = BaseDirs::new().context("resolve base dirs")?;
    let user_bin = base.home_dir().join(".local").join("bin");
    std::fs::create_dir_all(&user_bin).context("create user bin dir")?;
    let target = user_bin.join("mirror-cli");
    if target.exists() {
        return Ok(format!("PATH entry already exists at {}", target.display()));
    }
    std::os::unix::fs::symlink(dir.join("mirror-cli"), &target)
        .context("create symlink for mirror-cli")?;
    Ok(format!("Symlinked mirror-cli to {}", target.display()))
}

#[cfg(not(unix))]
fn add_path_unix(_dir: &Path) -> anyhow::Result<String> {
    anyhow::bail!("PATH install is only supported on Unix-like systems")
}

fn add_path_windows(dir: &Path) -> anyhow::Result<String> {
    let current = std::env::var("PATH").unwrap_or_default();
    let dir_str = dir.to_string_lossy().to_string();
    if current.split(';').any(|p| p.eq_ignore_ascii_case(&dir_str)) {
        return Ok("PATH already contains mirror-cli directory".to_string());
    }
    let updated = build_path_update(&current, &dir_str);
    Command::new("setx")
        .args(["PATH", &updated])
        .status()
        .context("update PATH with setx")?;
    Ok("Updated user PATH (restart shell to apply)".to_string())
}

fn build_path_update(current: &str, add: &str) -> String {
    if current.trim().is_empty() {
        return add.to_string();
    }
    format!("{current};{add}")
}

pub fn acquire_install_lock() -> anyhow::Result<InstallGuard> {
    let path = install_lock_path()?;
    let lock = LockFile::try_acquire(&path)?
        .ok_or_else(|| anyhow::anyhow!("installer already running"))?;
    Ok(InstallGuard { _lock: lock })
}

pub fn is_installed() -> anyhow::Result<bool> {
    let path = marker_path()?;
    Ok(path.exists())
}

pub fn write_marker() -> anyhow::Result<()> {
    let path = marker_path()?;
    if let Some(parent) = path.parent() {
        std::fs::create_dir_all(parent).context("create install marker dir")?;
    }
    std::fs::write(&path, "installed=true\n").context("write install marker")?;
    Ok(())
}

pub fn remove_marker() -> anyhow::Result<()> {
    let path = marker_path()?;
    if path.exists() {
        std::fs::remove_file(&path).context("remove install marker")?;
    }
    Ok(())
}

fn marker_path() -> anyhow::Result<PathBuf> {
    let project = ProjectDirs::from("com", "git-project-sync", "git-project-sync")
        .context("resolve project dirs")?;
    Ok(project.data_local_dir().join("install.marker"))
}

fn install_lock_path() -> anyhow::Result<PathBuf> {
    let project = ProjectDirs::from("com", "git-project-sync", "git-project-sync")
        .context("resolve project dirs")?;
    Ok(project.data_local_dir().join("install.lock"))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn build_path_update_appends() {
        let updated = build_path_update("C:\\bin", "C:\\new");
        assert_eq!(updated, "C:\\bin;C:\\new");
    }
}

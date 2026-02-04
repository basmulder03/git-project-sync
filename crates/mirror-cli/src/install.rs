use anyhow::Context;
use directories::{ProjectDirs, BaseDirs};
use mirror_core::lockfile::LockFile;
use std::path::{Path, PathBuf};
use std::process::Command;
use serde::{Deserialize, Serialize};
use std::fs;
use std::time::{SystemTime, UNIX_EPOCH};

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
    pub install: String,
    pub service: String,
    pub path: String,
}

pub struct InstallGuard {
    _lock: LockFile,
}

pub fn perform_install(exec_path: &Path, options: InstallOptions) -> anyhow::Result<InstallReport> {
    mirror_core::service::uninstall_service_if_exists().ok();
    let install_path = default_install_path(exec_path)?;
    let install_message = install_binary(exec_path, &install_path)?;
    mirror_core::service::install_service_with_delay(&install_path, options.delayed_start)?;
    let service = match options.delayed_start {
        Some(delay) if delay > 0 => format!("Service installed with delayed start ({delay}s)"),
        _ => "Service installed".to_string(),
    };
    let path = match options.path_choice {
        PathChoice::Add => register_path(&install_path)?,
        PathChoice::Skip => "PATH update skipped".to_string(),
    };
    write_marker()?;
    write_manifest(&install_path)?;
    Ok(InstallReport { install: install_message, service, path })
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

#[derive(Clone, Debug, Serialize, Deserialize)]
struct InstallManifest {
    version: u32,
    installed_path: PathBuf,
    installed_version: String,
    installed_at: u64,
}

const INSTALL_MANIFEST_VERSION: u32 = 1;

fn install_binary(exec_path: &Path, install_path: &Path) -> anyhow::Result<String> {
    if install_path == exec_path {
        return Ok(format!(
            "Install location already active at {}",
            install_path.display()
        ));
    }

    if let Some(parent) = install_path.parent() {
        fs::create_dir_all(parent).context("create install directory")?;
    }

    let tmp_path = install_path.with_extension("tmp");
    fs::copy(exec_path, &tmp_path).context("copy binary to temp location")?;
    #[cfg(unix)]
    {
        let perms = fs::metadata(exec_path)
            .context("read executable permissions")?
            .permissions();
        fs::set_permissions(&tmp_path, perms).context("set executable permissions")?;
    }
    if install_path.exists() {
        fs::remove_file(install_path).context("remove previous installed binary")?;
    }
    fs::rename(&tmp_path, install_path).context("move binary into install location")?;
    Ok(format!("Installed to {}", install_path.display()))
}

fn default_install_path(exec_path: &Path) -> anyhow::Result<PathBuf> {
    let file_name = exec_path
        .file_name()
        .ok_or_else(|| anyhow::anyhow!("executable path has no file name"))?;
    Ok(default_install_dir()?.join(file_name))
}

fn default_install_dir() -> anyhow::Result<PathBuf> {
    if cfg!(target_os = "windows") {
        let base = BaseDirs::new().context("resolve base dirs")?;
        return Ok(build_install_dir_windows(base.data_local_dir()));
    }
    let project = ProjectDirs::from("com", "git-project-sync", "git-project-sync")
        .context("resolve project dirs")?;
    Ok(build_install_dir_unix(project.data_local_dir()))
}

fn build_install_dir_windows(base: &Path) -> PathBuf {
    base.join("Programs").join("git-project-sync")
}

fn build_install_dir_unix(base: &Path) -> PathBuf {
    base.join("bin")
}

pub fn acquire_install_lock() -> anyhow::Result<InstallGuard> {
    let path = install_lock_path()?;
    let lock = LockFile::try_acquire(&path)?
        .ok_or_else(|| anyhow::anyhow!("installer already running"))?;
    Ok(InstallGuard { _lock: lock })
}

pub fn is_installed() -> anyhow::Result<bool> {
    if let Ok(Some(manifest)) = read_manifest() {
        return Ok(manifest.installed_path.exists());
    }
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

pub fn remove_manifest() -> anyhow::Result<()> {
    let path = manifest_path()?;
    if path.exists() {
        fs::remove_file(&path).context("remove install manifest")?;
    }
    Ok(())
}

fn marker_path() -> anyhow::Result<PathBuf> {
    let project = ProjectDirs::from("com", "git-project-sync", "git-project-sync")
        .context("resolve project dirs")?;
    Ok(project.data_local_dir().join("install.marker"))
}

fn manifest_path() -> anyhow::Result<PathBuf> {
    let project = ProjectDirs::from("com", "git-project-sync", "git-project-sync")
        .context("resolve project dirs")?;
    Ok(project.data_local_dir().join("install.json"))
}

fn read_manifest() -> anyhow::Result<Option<InstallManifest>> {
    let path = manifest_path()?;
    if !path.exists() {
        return Ok(None);
    }
    let data = fs::read_to_string(&path).context("read install manifest")?;
    let manifest = serde_json::from_str(&data).context("parse install manifest")?;
    Ok(Some(manifest))
}

fn write_manifest(install_path: &Path) -> anyhow::Result<()> {
    let path = manifest_path()?;
    if let Some(parent) = path.parent() {
        fs::create_dir_all(parent).context("create install manifest dir")?;
    }
    let manifest = InstallManifest {
        version: INSTALL_MANIFEST_VERSION,
        installed_path: install_path.to_path_buf(),
        installed_version: env!("CARGO_PKG_VERSION").to_string(),
        installed_at: current_epoch_seconds(),
    };
    let data = serde_json::to_string_pretty(&manifest).context("serialize install manifest")?;
    fs::write(&path, data).context("write install manifest")?;
    Ok(())
}

fn current_epoch_seconds() -> u64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap_or_default()
        .as_secs()
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

    #[cfg(windows)]
    #[test]
    fn build_install_dir_windows_appends_programs() {
        let base = Path::new("C:\\Users\\me\\AppData\\Local");
        let dir = build_install_dir_windows(base);
        assert_eq!(
            dir,
            PathBuf::from("C:\\Users\\me\\AppData\\Local\\Programs\\git-project-sync")
        );
    }

    #[cfg(unix)]
    #[test]
    fn build_install_dir_unix_appends_bin() {
        let base = Path::new("/home/me/.local/share/git-project-sync");
        let dir = build_install_dir_unix(base);
        assert_eq!(dir, PathBuf::from("/home/me/.local/share/git-project-sync/bin"));
    }
}

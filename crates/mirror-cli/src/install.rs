use anyhow::Context;
use directories::{BaseDirs, ProjectDirs};
use mirror_core::lockfile::LockFile;
use serde::{Deserialize, Serialize};
use std::fs;
use std::path::{Path, PathBuf};
use std::process::{Command, Stdio};
use std::sync::{Arc, Mutex, OnceLock};
use std::time::{SystemTime, UNIX_EPOCH};
#[cfg(windows)]
use tracing::debug;
use tracing::info;

pub struct InstallProgress {
    pub step: usize,
    pub total: usize,
    pub message: String,
}

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

#[derive(Clone, Debug)]
pub struct InstallStatus {
    pub installed: bool,
    pub installed_path: Option<PathBuf>,
    pub manifest_present: bool,
    pub installed_version: Option<String>,
    pub installed_at: Option<u64>,
    pub delayed_start: Option<u64>,
    pub service_installed: bool,
    pub service_running: bool,
    pub service_last_run: Option<String>,
    pub service_last_result: Option<String>,
    pub service_next_run: Option<String>,
    pub service_task_state: Option<String>,
    pub service_schedule_type: Option<String>,
    pub service_start_time: Option<String>,
    pub service_start_date: Option<String>,
    pub service_run_as: Option<String>,
    pub service_task_to_run: Option<String>,
    pub path_in_env: bool,
}

pub struct InstallGuard {
    lock: Arc<LockFile>,
}

static INSTALL_LOCK: OnceLock<Mutex<Option<Arc<LockFile>>>> = OnceLock::new();

impl Drop for InstallGuard {
    fn drop(&mut self) {
        let cell = INSTALL_LOCK.get_or_init(|| Mutex::new(None));
        let mut guard = cell.lock().expect("install lock mutex");
        if Arc::strong_count(&self.lock) == 2 {
            *guard = None;
        }
    }
}

pub fn perform_install_with_progress(
    exec_path: &Path,
    options: InstallOptions,
    progress: Option<&dyn Fn(InstallProgress)>,
    installed_version: Option<&str>,
) -> anyhow::Result<InstallReport> {
    let status = install_status().ok();
    let is_update = status.as_ref().map(|s| s.installed).unwrap_or(false);
    let install_path = resolve_install_path(
        exec_path,
        status
            .as_ref()
            .and_then(|value| value.installed_path.as_deref()),
    )?;
    let path_in_env = install_path
        .parent()
        .map(path_contains_dir)
        .unwrap_or(false);
    let update_path = matches!(options.path_choice, PathChoice::Add) && !path_in_env;
    let delayed_start = options.delayed_start.or_else(|| {
        if is_update {
            status.as_ref().and_then(|value| value.delayed_start)
        } else {
            None
        }
    });
    let total = if update_path { 5 } else { 4 };
    let mut step = 0;
    step += 1;
    report_progress(
        progress,
        step,
        total,
        if is_update {
            "Preparing update"
        } else {
            "Preparing install"
        },
    );
    mirror_core::service::uninstall_service_if_exists().ok();
    step += 1;
    report_progress(
        progress,
        step,
        total,
        &format!(
            "{} binary to {}",
            if is_update { "Updating" } else { "Installing" },
            install_path.display()
        ),
    );
    let install_message = install_binary(exec_path, &install_path, is_update)?;
    step += 1;
    report_progress(
        progress,
        step,
        total,
        if is_update {
            "Updating service"
        } else {
            "Installing service"
        },
    );
    mirror_core::service::install_service_with_delay(&install_path, delayed_start)?;
    let service_action = if is_update { "updated" } else { "installed" };
    let service = match delayed_start {
        Some(delay) if delay > 0 => {
            format!(
                "{} {service_action} with delayed start ({delay}s)",
                service_label()
            )
        }
        _ => format!("{} {service_action}", service_label()),
    };
    let path = if update_path {
        step += 1;
        report_progress(progress, step, total, "Registering PATH entry");
        register_path(&install_path)?
    } else if matches!(options.path_choice, PathChoice::Add) {
        "PATH already contains install directory".to_string()
    } else {
        "PATH update skipped".to_string()
    };
    step += 1;
    report_progress(progress, step, total, "Writing install metadata");
    write_marker()?;
    write_manifest(&install_path, installed_version, delayed_start)?;
    Ok(InstallReport {
        install: install_message,
        service,
        path,
    })
}

fn service_label() -> &'static str {
    if cfg!(target_os = "windows") {
        "Scheduled task"
    } else {
        "Service"
    }
}

fn report_progress(
    progress: Option<&dyn Fn(InstallProgress)>,
    step: usize,
    total: usize,
    message: &str,
) {
    if progress.is_none() {
        info!(step = message, "installer progress");
    }
    if let Some(callback) = progress {
        callback(InstallProgress {
            step,
            total,
            message: message.to_string(),
        });
    }
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

pub fn install_status() -> anyhow::Result<InstallStatus> {
    let manifest = read_manifest()?;
    let installed_path = manifest.as_ref().map(|m| m.installed_path.clone());
    let manifest_present = manifest.is_some();
    let installed_version = manifest.as_ref().map(|m| m.installed_version.clone());
    let installed_at = manifest.as_ref().map(|m| m.installed_at);
    let delayed_start = manifest.as_ref().and_then(|m| m.delayed_start);
    let marker_present = marker_path()?.exists();
    let installed = installed_path
        .as_ref()
        .map(|path| path.exists())
        .unwrap_or(marker_present);
    let service_installed = mirror_core::service::service_exists().unwrap_or(false);
    let service_running = mirror_core::service::service_running().unwrap_or(false);
    let service_status = mirror_core::service::service_status().ok();
    let path_dir = installed_path
        .as_ref()
        .and_then(|path| path.parent().map(|p| p.to_path_buf()));
    let install_dir_for_path_check = if installed_path.is_none() {
        default_install_dir().ok()
    } else {
        path_dir
    };
    let path_in_env = install_dir_for_path_check
        .as_ref()
        .map(|dir| path_contains_dir(dir))
        .unwrap_or(false);
    Ok(InstallStatus {
        installed,
        installed_path,
        manifest_present,
        installed_version,
        installed_at,
        delayed_start,
        service_installed,
        service_running,
        service_last_run: service_status
            .as_ref()
            .and_then(|s| s.last_run_time.clone()),
        service_last_result: service_status.as_ref().and_then(|s| s.last_result.clone()),
        service_next_run: service_status
            .as_ref()
            .and_then(|s| s.next_run_time.clone()),
        service_task_state: service_status.as_ref().and_then(|s| s.task_state.clone()),
        service_schedule_type: service_status
            .as_ref()
            .and_then(|s| s.schedule_type.clone()),
        service_start_time: service_status.as_ref().and_then(|s| s.start_time.clone()),
        service_start_date: service_status.as_ref().and_then(|s| s.start_date.clone()),
        service_run_as: service_status.as_ref().and_then(|s| s.run_as_user.clone()),
        service_task_to_run: service_status.as_ref().and_then(|s| s.task_to_run.clone()),
        path_in_env,
    })
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
        .stdout(Stdio::null())
        .stderr(Stdio::null())
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
    /// Startup delay in seconds (optional for backward-compatible manifests).
    #[serde(default)]
    delayed_start: Option<u64>,
}

const INSTALL_MANIFEST_VERSION: u32 = 1;

fn install_binary(
    exec_path: &Path,
    install_path: &Path,
    is_update: bool,
) -> anyhow::Result<String> {
    if install_path == exec_path {
        return Ok(format!(
            "{} location already active at {}",
            if is_update { "Update" } else { "Install" },
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
    Ok(format!(
        "{} to {}",
        if is_update { "Updated" } else { "Installed" },
        install_path.display()
    ))
}

fn default_install_path(exec_path: &Path) -> anyhow::Result<PathBuf> {
    let file_name = exec_path
        .file_name()
        .ok_or_else(|| anyhow::anyhow!("executable path has no file name"))?;
    Ok(default_install_dir()?.join(file_name))
}

fn resolve_install_path(
    exec_path: &Path,
    installed_path: Option<&Path>,
) -> anyhow::Result<PathBuf> {
    if let Some(path) = installed_path {
        return Ok(path.to_path_buf());
    }
    default_install_path(exec_path)
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
    let cell = INSTALL_LOCK.get_or_init(|| Mutex::new(None));
    let mut guard = cell.lock().expect("install lock mutex");
    if let Some(lock) = guard.as_ref() {
        return Ok(InstallGuard { lock: lock.clone() });
    }
    let path = install_lock_path()?;
    let lock = LockFile::try_acquire(&path)?
        .ok_or_else(|| anyhow::anyhow!("installer already running"))?;
    let lock = Arc::new(lock);
    *guard = Some(lock.clone());
    Ok(InstallGuard { lock })
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

fn write_manifest(
    install_path: &Path,
    installed_version: Option<&str>,
    delayed_start: Option<u64>,
) -> anyhow::Result<()> {
    let path = manifest_path()?;
    if let Some(parent) = path.parent() {
        fs::create_dir_all(parent).context("create install manifest dir")?;
    }
    let manifest = InstallManifest {
        version: INSTALL_MANIFEST_VERSION,
        installed_path: install_path.to_path_buf(),
        installed_version: resolve_installed_version(installed_version),
        installed_at: current_epoch_seconds(),
        delayed_start,
    };
    let data = serde_json::to_string_pretty(&manifest).context("serialize install manifest")?;
    fs::write(&path, data).context("write install manifest")?;
    Ok(())
}

fn resolve_installed_version(override_version: Option<&str>) -> String {
    override_version
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .unwrap_or(env!("CARGO_PKG_VERSION"))
        .to_string()
}

fn path_contains_dir(dir: &Path) -> bool {
    let current = std::env::var_os("PATH").unwrap_or_default();
    #[cfg(windows)]
    let dir = resolve_windows_path(dir);
    std::env::split_paths(&current).any(|path| {
        #[cfg(windows)]
        {
            let path = resolve_windows_path(&path);
            return eq_ignore_ascii_case_wide(&path, &dir);
        }
        #[cfg(not(windows))]
        {
            path == dir
        }
    })
}

#[cfg(windows)]
fn resolve_windows_path(path: &Path) -> PathBuf {
    path.canonicalize().unwrap_or_else(|err| {
        debug!(
            path = %path.display(),
            error = %err,
            "Could not resolve absolute path for PATH entry (path may not exist yet), using original path"
        );
        path.to_path_buf()
    })
}

#[cfg(windows)]
fn eq_ignore_ascii_case_wide(left: &Path, right: &Path) -> bool {
    use std::os::windows::ffi::OsStrExt;

    let left = left.as_os_str().encode_wide();
    let right = right.as_os_str().encode_wide();
    left.map(ascii_lowercase_wide)
        .eq(right.map(ascii_lowercase_wide))
}

/// Convert ASCII uppercase UTF-16 code units (A-Z) to lowercase for case-insensitive path
/// comparisons on Windows, leaving other code units unchanged.
#[cfg(windows)]
fn ascii_lowercase_wide(value: u16) -> u16 {
    const ASCII_UPPERCASE_A: u16 = b'A' as u16;
    const ASCII_UPPERCASE_Z: u16 = b'Z' as u16;
    const ASCII_CASE_OFFSET: u16 = 32;
    if (ASCII_UPPERCASE_A..=ASCII_UPPERCASE_Z).contains(&value) {
        value + ASCII_CASE_OFFSET
    } else {
        value
    }
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
        assert_eq!(
            dir,
            PathBuf::from("/home/me/.local/share/git-project-sync/bin")
        );
    }

    #[test]
    fn resolve_installed_version_prefers_override() {
        let resolved = resolve_installed_version(Some("9.9.9"));
        assert_eq!(resolved, "9.9.9");
    }

    #[test]
    fn resolve_installed_version_falls_back_for_blank_override() {
        let expected = env!("CARGO_PKG_VERSION");
        assert_eq!(resolve_installed_version(None), expected);
        assert_eq!(resolve_installed_version(Some("")), expected);
        assert_eq!(resolve_installed_version(Some("   ")), expected);
    }

    #[test]
    fn resolve_install_path_prefers_installed_path() {
        let exec_path = Path::new("/tmp/mirror-cli-new");
        let installed_path = Path::new("/opt/mirror-cli");
        let resolved = resolve_install_path(exec_path, Some(installed_path)).unwrap();
        assert_eq!(resolved, installed_path);
    }

    #[cfg(windows)]
    #[test]
    fn ascii_lowercase_wide_handles_ascii() {
        assert_eq!(ascii_lowercase_wide(b'A' as u16), b'a' as u16);
        assert_eq!(ascii_lowercase_wide(b'Z' as u16), b'z' as u16);
        assert_eq!(ascii_lowercase_wide(b'a' as u16), b'a' as u16);
        assert_eq!(ascii_lowercase_wide(0x00DF), 0x00DF);
    }

    #[cfg(windows)]
    #[test]
    fn eq_ignore_ascii_case_wide_matches_paths() {
        let left = Path::new("C:\\Test\\Path");
        let right = Path::new("c:\\test\\path");
        assert!(eq_ignore_ascii_case_wide(left, right));
    }

    #[cfg(windows)]
    #[test]
    fn resolve_windows_path_uses_canonical_or_original() {
        let temp = tempfile::TempDir::new().unwrap();
        let existing = temp.path().to_path_buf();
        let canonical = existing.canonicalize().unwrap();
        assert_eq!(resolve_windows_path(&existing), canonical);

        let missing = existing.join("missing");
        assert_eq!(resolve_windows_path(&missing), missing);
    }
}

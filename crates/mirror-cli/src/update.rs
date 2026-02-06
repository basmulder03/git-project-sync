use anyhow::Context;
use directories::ProjectDirs;
use reqwest::blocking::Client;
use semver::Version;
use serde::Deserialize;
use serde_json::json;
use std::fs::{self, File};
use std::io::Write;
use std::path::{Path, PathBuf};
use std::process::Command;

use mirror_core::audit::{AuditLogger, AuditStatus};
use mirror_core::cache::{RepoCache, record_update_check, update_check_due};

use crate::install::{InstallOptions, PathChoice};

pub const DEFAULT_UPDATE_REPO: &str = "basmulder03/git-project-sync";
pub const UPDATE_REPO_ENV: &str = "GIT_PROJECT_SYNC_UPDATE_REPO";

#[derive(Clone, Debug)]
pub struct ReleaseAsset {
    pub name: String,
    pub url: String,
}

#[derive(Clone, Debug)]
pub struct UpdateCheck {
    pub current: Version,
    pub latest: Version,
    pub release_url: Option<String>,
    pub is_newer: bool,
    pub asset: Option<ReleaseAsset>,
}

pub struct AutoUpdateOptions<'a> {
    pub cache_path: &'a Path,
    pub interval_secs: u64,
    pub auto_apply: bool,
    pub audit: &'a AuditLogger,
    pub force: bool,
    pub interactive: bool,
    pub source: &'a str,
    pub override_repo: Option<&'a str>,
}

struct RestartCommand {
    exec: PathBuf,
    args: Vec<String>,
}

#[derive(Deserialize)]
struct ReleaseResponse {
    tag_name: String,
    html_url: Option<String>,
    assets: Vec<ReleaseAssetResponse>,
}

#[derive(Deserialize)]
struct ReleaseAssetResponse {
    name: String,
    browser_download_url: String,
}

pub fn resolve_repo(override_repo: Option<&str>) -> String {
    if let Some(value) = override_repo {
        return value.to_string();
    }
    if let Ok(value) = std::env::var(UPDATE_REPO_ENV) {
        let trimmed = value.trim();
        if !trimmed.is_empty() {
            return trimmed.to_string();
        }
    }
    DEFAULT_UPDATE_REPO.to_string()
}

pub fn expected_asset_name() -> &'static str {
    if cfg!(target_os = "windows") {
        "mirror-cli-windows-x86_64.exe"
    } else if cfg!(target_os = "macos") {
        "mirror-cli-macos-x86_64"
    } else {
        "mirror-cli-linux-x86_64"
    }
}

pub fn check_for_update(override_repo: Option<&str>) -> anyhow::Result<UpdateCheck> {
    let repo = resolve_repo(override_repo);
    let url = format!("https://api.github.com/repos/{repo}/releases/latest");
    let client = Client::new();
    let response = client
        .get(url)
        .header("User-Agent", "git-project-sync")
        .header("Accept", "application/vnd.github+json")
        .send()
        .context("fetch latest release")?
        .error_for_status()
        .context("latest release request failed")?;
    let release: ReleaseResponse = response.json().context("parse release response")?;

    let current = Version::parse(env!("CARGO_PKG_VERSION")).context("parse current version")?;
    let latest = parse_version(&release.tag_name)?;
    let is_newer = latest > current;

    let asset = if is_newer {
        let target = expected_asset_name();
        release
            .assets
            .iter()
            .find(|asset| asset.name == target)
            .map(|asset| ReleaseAsset {
                name: asset.name.clone(),
                url: asset.browser_download_url.clone(),
            })
    } else {
        None
    };

    Ok(UpdateCheck {
        current,
        latest,
        release_url: release.html_url,
        is_newer,
        asset,
    })
}

pub fn apply_update(check: &UpdateCheck) -> anyhow::Result<crate::install::InstallReport> {
    apply_update_with_progress(check, None)
}

pub fn apply_update_with_progress(
    check: &UpdateCheck,
    progress: Option<&dyn Fn(&str)>,
) -> anyhow::Result<crate::install::InstallReport> {
    if !check.is_newer {
        anyhow::bail!("already up to date");
    }
    let asset = check
        .asset
        .as_ref()
        .context("no release asset for this platform")?;
    if !crate::install::is_installed()? {
        anyhow::bail!("no existing install found (run `mirror-cli install` first)");
    }
    let _guard = crate::install::acquire_install_lock()?;
    if let Some(callback) = progress {
        callback("Downloading release asset");
    }
    let download_path = download_asset(asset)?;
    if let Some(callback) = progress {
        callback("Installing update");
    }
    let installed_version = check.latest.to_string();
    crate::install::perform_install_with_progress(
        &download_path,
        InstallOptions {
            delayed_start: None,
            path_choice: PathChoice::Skip,
        },
        None,
        Some(installed_version.as_str()),
    )
}

pub fn check_and_maybe_apply(options: AutoUpdateOptions<'_>) -> anyhow::Result<bool> {
    let now = current_epoch_seconds();
    let mut cache = RepoCache::load(options.cache_path).unwrap_or_default();
    if !options.force && !update_check_due(&cache, now, options.interval_secs) {
        return Ok(false);
    }

    let check = match check_for_update(options.override_repo) {
        Ok(value) => value,
        Err(err) if is_network_error(&err) => {
            record_update_check(
                &mut cache,
                now,
                "skipped:network".to_string(),
                None,
                options.source,
            );
            let _ = cache.save(options.cache_path);
            let _ = options.audit.record(
                "update.check",
                AuditStatus::Skipped,
                Some("update"),
                Some(json!({"reason": "network", "source": options.source})),
                Some(&err.to_string()),
            );
            return Ok(false);
        }
        Err(err) => {
            let _ = options.audit.record(
                "update.check",
                AuditStatus::Failed,
                Some("update"),
                Some(json!({"source": options.source})),
                Some(&err.to_string()),
            );
            return Err(err);
        }
    };

    let latest = check.latest.to_string();
    let result = if check.is_newer {
        "update_available"
    } else {
        "up_to_date"
    };
    record_update_check(
        &mut cache,
        now,
        result.to_string(),
        Some(latest.clone()),
        options.source,
    );
    let _ = cache.save(options.cache_path);

    let _ = options.audit.record(
        "update.check",
        AuditStatus::Ok,
        Some("update"),
        Some(json!({
            "current": check.current.to_string(),
            "latest": latest,
            "is_newer": check.is_newer,
            "source": options.source
        })),
        None,
    );

    let mut applied = false;
    if check.is_newer && options.auto_apply {
        if !crate::install::is_installed()? {
            let _ = options.audit.record(
                "update.apply",
                AuditStatus::Skipped,
                Some("update"),
                Some(json!({"reason": "not_installed", "source": options.source})),
                None,
            );
            return Ok(false);
        }
        match apply_update(&check) {
            Ok(report) => {
                applied = true;
                let _ = options.audit.record(
                    "update.apply",
                    AuditStatus::Ok,
                    Some("update"),
                    Some(json!({
                        "source": options.source,
                        "install": report.install,
                        "service": report.service,
                        "path": report.path
                    })),
                    None,
                );
            }
            Err(err) if is_permission_error(&err) && options.interactive => {
                let _ = options.audit.record(
                    "update.apply",
                    AuditStatus::Failed,
                    Some("update"),
                    Some(json!({"source": options.source, "reason": "permission"})),
                    Some(&err.to_string()),
                );
                return Err(err);
            }
            Err(err) => {
                let _ = options.audit.record(
                    "update.apply",
                    AuditStatus::Failed,
                    Some("update"),
                    Some(json!({"source": options.source})),
                    Some(&err.to_string()),
                );
                return Err(err);
            }
        }
    }

    Ok(applied)
}

fn download_asset(asset: &ReleaseAsset) -> anyhow::Result<PathBuf> {
    let target_dir = update_download_dir()?;
    fs::create_dir_all(&target_dir).context("create update download dir")?;
    let path = target_dir.join(&asset.name);
    let mut file = File::create(&path).context("create download file")?;
    let client = Client::new();
    let mut response = client
        .get(&asset.url)
        .header("User-Agent", "git-project-sync")
        .send()
        .context("download release asset")?
        .error_for_status()
        .context("download request failed")?;
    let size = std::io::copy(&mut response, &mut file).context("write download")?;
    if size == 0 {
        anyhow::bail!("downloaded asset is empty");
    }
    file.flush().ok();
    ensure_executable(&path)?;
    Ok(path)
}

fn update_download_dir() -> anyhow::Result<PathBuf> {
    let project = ProjectDirs::from("com", "git-project-sync", "git-project-sync")
        .context("resolve project dirs")?;
    Ok(project.data_local_dir().join("updates"))
}

fn ensure_executable(path: &Path) -> anyhow::Result<()> {
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        let mut perms = fs::metadata(path)?.permissions();
        perms.set_mode(0o755);
        fs::set_permissions(path, perms).context("set executable permission")?;
    }
    #[cfg(not(unix))]
    {
        let _ = path;
    }
    Ok(())
}

fn parse_version(raw: &str) -> anyhow::Result<Version> {
    let trimmed = raw.trim();
    let normalized = trimmed.strip_prefix('v').unwrap_or(trimmed);
    Version::parse(normalized).context("parse release version")
}

fn choose_restart_path(installed_path: Option<PathBuf>, current_exe: PathBuf) -> PathBuf {
    if let Some(path) = installed_path
        && path.exists()
    {
        return path;
    }
    current_exe
}

fn restart_command() -> anyhow::Result<RestartCommand> {
    let current_exe = std::env::current_exe().context("resolve current executable")?;
    let installed_path = crate::install::install_status()
        .ok()
        .and_then(|status| status.installed_path);
    Ok(RestartCommand {
        exec: choose_restart_path(installed_path, current_exe),
        args: std::env::args().skip(1).collect(),
    })
}

#[cfg(unix)]
pub fn restart_current_process() -> anyhow::Result<()> {
    use std::os::unix::process::CommandExt;

    let command = restart_command()?;
    let err = Command::new(command.exec).args(command.args).exec();
    Err(err).context("re-exec mirror-cli")
}

#[cfg(not(unix))]
pub fn restart_current_process() -> anyhow::Result<()> {
    let command = restart_command()?;
    Command::new(command.exec)
        .args(command.args)
        .spawn()
        .context("restart mirror-cli")?;
    std::process::exit(0);
}

fn current_epoch_seconds() -> u64 {
    use std::time::{SystemTime, UNIX_EPOCH};
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap_or_default()
        .as_secs()
}

pub fn is_network_error(err: &anyhow::Error) -> bool {
    err.chain().any(|cause| {
        if let Some(reqwest_err) = cause.downcast_ref::<reqwest::Error>() {
            return reqwest_err.is_timeout()
                || reqwest_err.is_connect()
                || reqwest_err.is_request();
        }
        false
    })
}

pub fn is_permission_error(err: &anyhow::Error) -> bool {
    err.chain().any(|cause| {
        if let Some(io_err) = cause.downcast_ref::<std::io::Error>() {
            return io_err.kind() == std::io::ErrorKind::PermissionDenied;
        }
        false
    })
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::fs;
    use tempfile::TempDir;

    #[test]
    fn resolve_repo_prefers_override() {
        unsafe {
            std::env::set_var(UPDATE_REPO_ENV, "env/repo");
        }
        let repo = resolve_repo(Some("override/repo"));
        assert_eq!(repo, "override/repo");
        unsafe {
            std::env::remove_var(UPDATE_REPO_ENV);
        }
    }

    #[test]
    fn parse_version_strips_v() {
        let parsed = parse_version("v1.2.3").unwrap();
        assert_eq!(parsed.to_string(), "1.2.3");
    }

    #[test]
    fn choose_restart_path_uses_installed_path() {
        let temp = TempDir::new().unwrap();
        let installed = temp.path().join("mirror-cli");
        fs::write(&installed, b"binary").unwrap();
        let current = temp.path().join("current");
        let chosen = choose_restart_path(Some(installed.clone()), current);
        assert_eq!(chosen, installed);
    }
}

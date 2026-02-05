use anyhow::Context;
use directories::ProjectDirs;
use reqwest::blocking::Client;
use semver::Version;
use serde::Deserialize;
use std::fs::{self, File};
use std::io::Write;
use std::path::{Path, PathBuf};

use crate::install::{InstallOptions, PathChoice};

pub const DEFAULT_UPDATE_REPO: &str = "basmulder03/git-project-sync";
pub const UPDATE_REPO_ENV: &str = "GIT_PROJECT_SYNC_UPDATE_REPO";

#[derive(Clone, Debug)]
pub struct ReleaseAsset {
    pub name: String,
    pub url: String,
    pub size: u64,
}

#[derive(Clone, Debug)]
pub struct UpdateCheck {
    pub repo: String,
    pub current: Version,
    pub latest: Version,
    pub tag: String,
    pub release_url: Option<String>,
    pub is_newer: bool,
    pub asset: Option<ReleaseAsset>,
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
    size: u64,
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
                size: asset.size,
            })
    } else {
        None
    };

    Ok(UpdateCheck {
        repo,
        current,
        latest,
        tag: release.tag_name,
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
    crate::install::perform_install_with_progress(
        &download_path,
        InstallOptions {
            delayed_start: None,
            path_choice: PathChoice::Skip,
        },
        None,
    )
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

#[cfg(test)]
mod tests {
    use super::*;

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
}

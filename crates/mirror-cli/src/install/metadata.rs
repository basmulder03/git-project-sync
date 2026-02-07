use anyhow::Context;
use directories::{BaseDirs, ProjectDirs};
use serde::{Deserialize, Serialize};
use std::fs;
use std::path::{Path, PathBuf};
use std::time::{SystemTime, UNIX_EPOCH};

#[derive(Clone, Debug, Serialize, Deserialize)]
pub(in crate::install) struct InstallManifest {
    pub(in crate::install) version: u32,
    pub(in crate::install) installed_path: PathBuf,
    pub(in crate::install) installed_version: String,
    pub(in crate::install) installed_at: u64,
    #[serde(default)]
    pub(in crate::install) delayed_start: Option<u64>,
}

const INSTALL_MANIFEST_VERSION: u32 = 1;

pub(in crate::install) fn install_binary(
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

pub(in crate::install) fn resolve_install_path(
    exec_path: &Path,
    installed_path: Option<&Path>,
) -> anyhow::Result<PathBuf> {
    if let Some(path) = installed_path {
        return Ok(path.to_path_buf());
    }
    default_install_path(exec_path)
}

pub(in crate::install) fn default_install_dir() -> anyhow::Result<PathBuf> {
    if cfg!(target_os = "windows") {
        let base = BaseDirs::new().context("resolve base dirs")?;
        return Ok(build_install_dir_windows(base.data_local_dir()));
    }
    let project = ProjectDirs::from("com", "git-project-sync", "git-project-sync")
        .context("resolve project dirs")?;
    Ok(build_install_dir_unix(project.data_local_dir()))
}

pub(in crate::install) fn build_install_dir_windows(base: &Path) -> PathBuf {
    base.join("Programs").join("git-project-sync")
}

pub(in crate::install) fn build_install_dir_unix(base: &Path) -> PathBuf {
    base.join("bin")
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

pub(in crate::install) fn marker_exists() -> anyhow::Result<bool> {
    Ok(marker_path()?.exists())
}

fn manifest_path() -> anyhow::Result<PathBuf> {
    let project = ProjectDirs::from("com", "git-project-sync", "git-project-sync")
        .context("resolve project dirs")?;
    Ok(project.data_local_dir().join("install.json"))
}

pub(in crate::install) fn read_manifest() -> anyhow::Result<Option<InstallManifest>> {
    let path = manifest_path()?;
    if !path.exists() {
        return Ok(None);
    }
    let data = fs::read_to_string(&path).context("read install manifest")?;
    let manifest = serde_json::from_str(&data).context("parse install manifest")?;
    Ok(Some(manifest))
}

pub(in crate::install) fn write_manifest(
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

pub(in crate::install) fn resolve_installed_version(override_version: Option<&str>) -> String {
    override_version
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .unwrap_or(env!("CARGO_PKG_VERSION"))
        .to_string()
}

fn current_epoch_seconds() -> u64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap_or_default()
        .as_secs()
}

#[cfg(test)]
mod tests {
    use super::*;

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
}

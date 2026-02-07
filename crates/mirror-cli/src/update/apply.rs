use super::*;

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

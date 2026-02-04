use crate::model::{ProviderKind, ProviderScope};
use crate::paths::repo_path;
use anyhow::Context;
use std::fs;
use std::path::{Path, PathBuf};

pub fn archive_repo(
    root: &Path,
    provider: ProviderKind,
    scope: &ProviderScope,
    repo: &str,
) -> anyhow::Result<PathBuf> {
    let source = repo_path(root, &provider, scope, repo);
    let mut archive_root = root.to_path_buf();
    archive_root.push("_archive");
    let destination = repo_path(&archive_root, &provider, scope, repo);
    if let Some(parent) = destination.parent() {
        fs::create_dir_all(parent).context("create archive parent")?;
    }
    if let Err(err) = fs::rename(&source, &destination) {
        copy_dir_recursive(&source, &destination)
            .with_context(|| format!("copy repo to archive after rename failed: {err}"))?;
        fs::remove_dir_all(&source).context("remove source repo after archive copy")?;
    }
    Ok(destination)
}

pub fn remove_repo(
    root: &Path,
    provider: ProviderKind,
    scope: &ProviderScope,
    repo: &str,
) -> anyhow::Result<()> {
    let source = repo_path(root, &provider, scope, repo);
    if source.exists() {
        fs::remove_dir_all(&source).context("remove repo directory")?;
    }
    Ok(())
}

fn copy_dir_recursive(source: &Path, destination: &Path) -> anyhow::Result<()> {
    if !source.exists() {
        return Ok(());
    }
    if let Some(parent) = destination.parent() {
        fs::create_dir_all(parent).context("create archive copy parent")?;
    }
    fs::create_dir_all(destination).context("create archive copy dir")?;
    for entry in fs::read_dir(source).context("read archive source dir")? {
        let entry = entry.context("read archive entry")?;
        let file_type = entry.file_type().context("read archive entry type")?;
        let from = entry.path();
        let to = destination.join(entry.file_name());
        if file_type.is_dir() {
            copy_dir_recursive(&from, &to)?;
        } else if file_type.is_file() {
            fs::copy(&from, &to)
                .with_context(|| format!("copy archive file {}", from.display()))?;
        }
    }
    Ok(())
}

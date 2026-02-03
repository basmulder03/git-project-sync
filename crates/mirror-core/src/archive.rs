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
    fs::rename(&source, &destination).context("move repo to archive")?;
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

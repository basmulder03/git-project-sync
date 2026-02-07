use anyhow::Context;
use std::fs;
use std::path::Path;

pub(crate) fn move_repo_path(from: &Path, to: &Path) -> anyhow::Result<()> {
    if let Some(parent) = to.parent() {
        fs::create_dir_all(parent).context("create repo rename parent")?;
    }
    if let Err(err) = fs::rename(from, to) {
        copy_dir_recursive(from, to)
            .with_context(|| format!("copy repo after rename failed: {err}"))?;
        fs::remove_dir_all(from).context("remove old repo path after rename copy")?;
    }
    Ok(())
}

fn copy_dir_recursive(source: &Path, destination: &Path) -> anyhow::Result<()> {
    if !source.exists() {
        return Ok(());
    }
    if let Some(parent) = destination.parent() {
        fs::create_dir_all(parent).context("create repo copy parent")?;
    }
    fs::create_dir_all(destination).context("create repo copy dir")?;
    for entry in fs::read_dir(source).context("read repo source dir")? {
        let entry = entry.context("read repo entry")?;
        let file_type = entry.file_type().context("read repo entry type")?;
        let from = entry.path();
        let to = destination.join(entry.file_name());
        if file_type.is_dir() {
            copy_dir_recursive(&from, &to)?;
        } else if file_type.is_file() {
            fs::copy(&from, &to).with_context(|| format!("copy repo file {}", from.display()))?;
        }
    }
    Ok(())
}

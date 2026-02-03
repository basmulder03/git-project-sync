use anyhow::Context;
use fs2::FileExt;
use std::fs::{self, File, OpenOptions};
use std::path::{Path, PathBuf};

#[derive(Debug)]
pub struct LockFile {
    path: PathBuf,
    file: File,
}

impl LockFile {
    pub fn try_acquire(path: &Path) -> anyhow::Result<Option<Self>> {
        if let Some(parent) = path.parent() {
            fs::create_dir_all(parent).context("create lockfile directory")?;
        }
        let file = OpenOptions::new()
            .create(true)
            .read(true)
            .write(true)
            .truncate(false)
            .open(path)
            .with_context(|| format!("open lockfile {}", path.display()))?;

        match file.try_lock_exclusive() {
            Ok(()) => Ok(Some(Self {
                path: path.to_path_buf(),
                file,
            })),
            Err(err) if is_lock_held(&err) => Ok(None),
            Err(err) => Err(err).context("lock file exclusively"),
        }
    }

    pub fn path(&self) -> &Path {
        &self.path
    }
}

impl Drop for LockFile {
    fn drop(&mut self) {
        let _ = self.file.unlock();
    }
}

fn is_lock_held(err: &std::io::Error) -> bool {
    if err.kind() == std::io::ErrorKind::WouldBlock {
        return true;
    }
    matches!(err.raw_os_error(), Some(33))
}

#[cfg(test)]
mod tests {
    use super::*;
    use tempfile::TempDir;

    #[test]
    fn lockfile_prevents_double_lock() {
        let tmp = TempDir::new().unwrap();
        let lock_path = tmp.path().join("mirror.lock");
        let first = LockFile::try_acquire(&lock_path).unwrap();
        assert!(first.is_some());
        let second = LockFile::try_acquire(&lock_path).unwrap();
        assert!(second.is_none());
    }
}

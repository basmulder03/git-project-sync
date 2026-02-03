use crate::lockfile::LockFile;
use std::path::Path;
use std::thread;
use std::time::Duration;
use tracing::{info, warn};

pub fn run_once_with_lock<F>(lock_path: &Path, mut job: F) -> anyhow::Result<bool>
where
    F: FnMut() -> anyhow::Result<()>,
{
    match LockFile::try_acquire(lock_path)? {
        Some(_lock) => {
            job()?;
            Ok(true)
        }
        None => {
            warn!(path = %lock_path.display(), "lock already held; skipping run");
            Ok(false)
        }
    }
}

pub fn run_daemon<F>(lock_path: &Path, interval: Duration, mut job: F) -> anyhow::Result<()>
where
    F: FnMut() -> anyhow::Result<()>,
{
    loop {
        let ran = run_once_with_lock(lock_path, &mut job)?;
        if ran {
            info!("run completed");
        }
        thread::sleep(interval);
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::sync::atomic::{AtomicUsize, Ordering};
    use tempfile::TempDir;

    #[test]
    fn run_once_skips_when_locked() {
        let tmp = TempDir::new().unwrap();
        let lock_path = tmp.path().join("mirror.lock");
        let counter = AtomicUsize::new(0);

        let _guard = LockFile::try_acquire(&lock_path).unwrap().unwrap();
        let ran = run_once_with_lock(&lock_path, || {
            counter.fetch_add(1, Ordering::SeqCst);
            Ok(())
        })
        .unwrap();
        assert!(!ran);
        assert_eq!(counter.load(Ordering::SeqCst), 0);
    }
}

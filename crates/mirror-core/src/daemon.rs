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
    let mut failure_count: u32 = 0;
    loop {
        match run_once_with_lock(lock_path, &mut job) {
            Ok(ran) => {
                if ran {
                    info!("run completed");
                }
                failure_count = 0;
            }
            Err(err) => {
                failure_count = failure_count.saturating_add(1);
                warn!(error = %err, failures = failure_count, "run failed");
            }
        }
        thread::sleep(daemon_backoff_delay(interval, failure_count));
    }
}

pub fn daemon_backoff_delay(interval: Duration, failures: u32) -> Duration {
    if failures == 0 {
        return interval;
    }
    let base = interval.as_secs().max(1);
    let exp = failures.saturating_sub(1).min(5);
    let delay = base.saturating_mul(2u64.saturating_pow(exp));
    Duration::from_secs(delay.min(3600))
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

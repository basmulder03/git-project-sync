use sha2::{Digest, Sha256};
use std::time::{SystemTime, UNIX_EPOCH};

pub fn bucket_for_repo_id(repo_id: &str) -> u8 {
    let mut hasher = Sha256::new();
    hasher.update(repo_id.as_bytes());
    let digest = hasher.finalize();
    let value = u64::from_be_bytes([
        digest[0], digest[1], digest[2], digest[3], digest[4], digest[5], digest[6], digest[7],
    ]);
    (value % 7) as u8
}

pub fn bucket_for_timestamp(seconds_since_epoch: u64) -> u8 {
    ((seconds_since_epoch / 86_400) % 7) as u8
}

pub fn current_day_bucket() -> u8 {
    let now = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap_or_default();
    bucket_for_timestamp(now.as_secs())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn bucket_is_stable() {
        let a = bucket_for_repo_id("repo-123");
        let b = bucket_for_repo_id("repo-123");
        assert_eq!(a, b);
        assert!(a < 7);
    }

    #[test]
    fn bucket_for_timestamp_is_stable() {
        let a = bucket_for_timestamp(0);
        let b = bucket_for_timestamp(0);
        assert_eq!(a, b);
        assert!(a < 7);
    }
}

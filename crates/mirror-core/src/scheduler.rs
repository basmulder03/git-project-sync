use sha2::{Digest, Sha256};

pub fn bucket_for_repo_id(repo_id: &str) -> u8 {
    let mut hasher = Sha256::new();
    hasher.update(repo_id.as_bytes());
    let digest = hasher.finalize();
    let value = u64::from_be_bytes([
        digest[0], digest[1], digest[2], digest[3], digest[4], digest[5], digest[6], digest[7],
    ]);
    (value % 7) as u8
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
}

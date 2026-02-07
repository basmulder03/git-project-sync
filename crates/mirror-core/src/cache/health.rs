use super::*;

pub fn token_check_due(cache: &RepoCache, now: u64, interval_secs: u64) -> bool {
    match cache.token_last_check {
        Some(last) => now.saturating_sub(last) >= interval_secs,
        None => true,
    }
}

pub fn update_check_due(cache: &RepoCache, now: u64, interval_secs: u64) -> bool {
    match cache.update_last_check {
        Some(last) => now.saturating_sub(last) >= interval_secs,
        None => true,
    }
}

pub fn record_update_check(
    cache: &mut RepoCache,
    now: u64,
    result: String,
    latest_version: Option<String>,
    source: &str,
) {
    cache.update_last_check = Some(now);
    cache.update_last_result = Some(result);
    cache.update_last_version = latest_version;
    cache.update_last_source = Some(source.to_string());
}

pub fn record_token_check(cache: &mut RepoCache, now: u64, source: &str) {
    cache.token_last_check = Some(now);
    cache.token_last_source = Some(source.to_string());
}

pub fn record_token_status(cache: &mut RepoCache, account: String, status: TokenStatus) {
    cache.token_status.insert(account, status);
}

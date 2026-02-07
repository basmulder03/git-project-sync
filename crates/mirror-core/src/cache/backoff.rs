pub(super) fn compute_backoff_delay(attempts: u32) -> u64 {
    const BASE: u64 = 60;
    const MAX: u64 = 3600;
    let exp = attempts.saturating_sub(1).min(10);
    let delay = BASE.saturating_mul(2u64.saturating_pow(exp));
    delay.min(MAX)
}

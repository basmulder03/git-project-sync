use anyhow::{Context, bail};
use reqwest::StatusCode;
use reqwest::header::HeaderMap;
use reqwest::{RequestBuilder, Response};
use std::time::{Duration, SystemTime, UNIX_EPOCH};

pub async fn send_with_retry<F>(mut build: F) -> anyhow::Result<Response>
where
    F: FnMut() -> anyhow::Result<RequestBuilder>,
{
    let max_attempts = 3;
    for attempt in 1..=max_attempts {
        let response = build()?.send().await.context("send request")?;
        let status = response.status();
        if status.is_success() {
            return Ok(response);
        }
        if is_retryable(status) && attempt < max_attempts {
            let delay = retry_delay_from_headers(response.headers());
            let _ = response.bytes().await;
            if let Some(delay) = delay {
                tokio::time::sleep(delay).await;
                continue;
            }
            tokio::time::sleep(Duration::from_secs(1)).await;
            continue;
        }
        return Err(response.error_for_status().unwrap_err().into());
    }
    bail!("request failed after retries");
}

pub async fn send_with_retry_allow_statuses<F>(
    mut build: F,
    allowed: &[StatusCode],
) -> anyhow::Result<Response>
where
    F: FnMut() -> anyhow::Result<RequestBuilder>,
{
    let max_attempts = 3;
    for attempt in 1..=max_attempts {
        let response = build()?.send().await.context("send request")?;
        let status = response.status();
        if status.is_success() || allowed.contains(&status) {
            return Ok(response);
        }
        if is_retryable(status) && attempt < max_attempts {
            let delay = retry_delay_from_headers(response.headers());
            let _ = response.bytes().await;
            if let Some(delay) = delay {
                tokio::time::sleep(delay).await;
                continue;
            }
            tokio::time::sleep(Duration::from_secs(1)).await;
            continue;
        }
        return Err(response.error_for_status().unwrap_err().into());
    }
    bail!("request failed after retries");
}

fn is_retryable(status: StatusCode) -> bool {
    matches!(
        status,
        StatusCode::TOO_MANY_REQUESTS | StatusCode::SERVICE_UNAVAILABLE
    )
}

fn retry_delay_from_headers(headers: &HeaderMap) -> Option<Duration> {
    if let Some(delay) = retry_after_seconds(headers) {
        return Some(Duration::from_secs(delay));
    }
    if let Some(delay) = ratelimit_reset_seconds(headers) {
        return Some(Duration::from_secs(delay));
    }
    None
}

fn retry_after_seconds(headers: &HeaderMap) -> Option<u64> {
    headers
        .get("retry-after")
        .and_then(|value| value.to_str().ok())
        .and_then(|value| value.parse::<u64>().ok())
}

fn ratelimit_reset_seconds(headers: &HeaderMap) -> Option<u64> {
    let reset = headers
        .get("x-ratelimit-reset")
        .and_then(|value| value.to_str().ok())
        .and_then(|value| value.parse::<u64>().ok())?;
    let now = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap_or_default()
        .as_secs();
    if reset <= now {
        None
    } else {
        Some(reset - now)
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use reqwest::header::HeaderValue;

    #[test]
    fn retry_after_parses_seconds() {
        let mut headers = HeaderMap::new();
        headers.insert("retry-after", HeaderValue::from_static("5"));
        assert_eq!(retry_after_seconds(&headers), Some(5));
    }

    #[test]
    fn ratelimit_reset_uses_future_time() {
        let now = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap_or_default()
            .as_secs();
        let mut headers = HeaderMap::new();
        headers.insert(
            "x-ratelimit-reset",
            HeaderValue::from_str(&(now + 10).to_string()).unwrap(),
        );
        let delay = ratelimit_reset_seconds(&headers).unwrap();
        assert!(delay > 0);
    }
}

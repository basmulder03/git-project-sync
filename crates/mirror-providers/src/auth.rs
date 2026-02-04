use anyhow::Context;
use keyring::Entry;
use reqwest::blocking::Client;
use serde::{Deserialize, Serialize};
use std::time::{Duration, SystemTime, UNIX_EPOCH};

const SERVICE: &str = "git-project-sync";

const EXPIRY_LEEWAY_SECS: i64 = 60;

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
struct StoredToken {
    kind: String,
    access_token: String,
    #[serde(default)]
    refresh_token: Option<String>,
    #[serde(default)]
    expires_at: Option<i64>,
    #[serde(default)]
    token_endpoint: Option<String>,
    #[serde(default)]
    client_id: Option<String>,
    #[serde(default)]
    scope: Option<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct OAuthToken {
    pub access_token: String,
    pub refresh_token: Option<String>,
    pub expires_at: Option<i64>,
    pub token_endpoint: String,
    pub client_id: String,
    pub scope: Option<String>,
}

#[derive(Debug, Deserialize)]
struct RefreshTokenResponse {
    access_token: String,
    #[serde(default)]
    refresh_token: Option<String>,
    #[serde(default)]
    expires_in: Option<i64>,
    #[serde(default)]
    error: Option<String>,
    #[serde(default)]
    error_description: Option<String>,
}

pub fn get_pat(account: &str) -> anyhow::Result<String> {
    get_token(account)
}

pub fn get_token(account: &str) -> anyhow::Result<String> {
    let entry = Entry::new(SERVICE, account).context("open keyring entry")?;
    let value = entry.get_password().context("read token from keyring")?;
    if let Ok(stored) = serde_json::from_str::<StoredToken>(&value) {
        if stored.kind == "oauth" {
            return ensure_oauth_token(account, &entry, stored);
        }
    }
    Ok(value)
}

pub fn set_pat(account: &str, token: &str) -> anyhow::Result<()> {
    let entry = Entry::new(SERVICE, account).context("open keyring entry")?;
    entry.set_password(token).context("write PAT to keyring")
}

pub fn set_oauth_token(account: &str, token: OAuthToken) -> anyhow::Result<()> {
    let entry = Entry::new(SERVICE, account).context("open keyring entry")?;
    let stored = StoredToken {
        kind: "oauth".to_string(),
        access_token: token.access_token,
        refresh_token: token.refresh_token,
        expires_at: token.expires_at,
        token_endpoint: Some(token.token_endpoint),
        client_id: Some(token.client_id),
        scope: token.scope,
    };
    let payload = serde_json::to_string(&stored).context("serialize oauth token")?;
    entry
        .set_password(&payload)
        .context("write oauth token to keyring")
}

fn ensure_oauth_token(
    account: &str,
    entry: &Entry,
    mut stored: StoredToken,
) -> anyhow::Result<String> {
    if !requires_refresh(&stored, EXPIRY_LEEWAY_SECS) {
        return Ok(stored.access_token);
    }

    let refresh_token = stored
        .refresh_token
        .clone()
        .ok_or_else(|| anyhow::anyhow!("oauth token for {account} expired without refresh token"))?;
    let token_endpoint = stored
        .token_endpoint
        .clone()
        .ok_or_else(|| anyhow::anyhow!("oauth token for {account} missing token endpoint"))?;
    let client_id = stored
        .client_id
        .clone()
        .ok_or_else(|| anyhow::anyhow!("oauth token for {account} missing client id"))?;

    let client = Client::new();
    let mut form = vec![
        ("grant_type", "refresh_token".to_string()),
        ("refresh_token", refresh_token),
        ("client_id", client_id),
    ];
    if let Some(scope) = stored.scope.clone() {
        form.push(("scope", scope));
    }

    let response: RefreshTokenResponse = client
        .post(&token_endpoint)
        .form(&form)
        .send()
        .context("request oauth refresh token")?
        .error_for_status()
        .context("refresh token status")?
        .json()
        .context("decode refresh token response")?;

    if let Some(error) = response.error {
        let message = response.error_description.unwrap_or(error);
        anyhow::bail!("oauth refresh failed: {message}");
    }

    stored.access_token = response.access_token;
    if let Some(refresh) = response.refresh_token {
        stored.refresh_token = Some(refresh);
    }
    if let Some(expires_in) = response.expires_in {
        stored.expires_at = Some(now_epoch_seconds() + expires_in);
    }

    let payload = serde_json::to_string(&stored).context("serialize refreshed token")?;
    entry
        .set_password(&payload)
        .context("write refreshed token to keyring")?;
    Ok(stored.access_token)
}

fn requires_refresh(stored: &StoredToken, leeway_secs: i64) -> bool {
    requires_refresh_at(stored, now_epoch_seconds(), leeway_secs)
}

fn now_epoch_seconds() -> i64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap_or(Duration::from_secs(0))
        .as_secs() as i64
}

fn requires_refresh_at(stored: &StoredToken, now: i64, leeway_secs: i64) -> bool {
    if let Some(expires_at) = stored.expires_at {
        return expires_at <= now + leeway_secs;
    }
    false
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn requires_refresh_with_leeway() {
        let stored = StoredToken {
            kind: "oauth".to_string(),
            access_token: "token".to_string(),
            refresh_token: Some("refresh".to_string()),
            expires_at: Some(100),
            token_endpoint: Some("https://example.com/token".to_string()),
            client_id: Some("client".to_string()),
            scope: None,
        };
        assert!(requires_refresh_at(&stored, 50, 60));
        assert!(!requires_refresh_at(&stored, 0, 0));
    }

    #[test]
    fn oauth_token_round_trip() {
        let token = OAuthToken {
            access_token: "access".to_string(),
            refresh_token: Some("refresh".to_string()),
            expires_at: Some(1234),
            token_endpoint: "https://example.com/token".to_string(),
            client_id: "client".to_string(),
            scope: Some("scope".to_string()),
        };
        let stored = StoredToken {
            kind: "oauth".to_string(),
            access_token: token.access_token.clone(),
            refresh_token: token.refresh_token.clone(),
            expires_at: token.expires_at,
            token_endpoint: Some(token.token_endpoint.clone()),
            client_id: Some(token.client_id.clone()),
            scope: token.scope.clone(),
        };
        let json = serde_json::to_string(&stored).unwrap();
        let decoded: StoredToken = serde_json::from_str(&json).unwrap();
        assert_eq!(decoded.kind, "oauth");
        assert_eq!(decoded.access_token, "access");
        assert_eq!(decoded.refresh_token.as_deref(), Some("refresh"));
    }
}

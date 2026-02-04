use anyhow::Context;
use keyring::Entry;
use mirror_core::audit::{AuditLogger, AuditStatus};
use reqwest::blocking::Client;
use serde::{Deserialize, Serialize};
use serde_json::json;
use std::sync::OnceLock;
use std::time::{Duration, SystemTime, UNIX_EPOCH};

const SERVICE: &str = "git-project-sync";
const OAUTH_ALLOW_ENV: &str = "GIT_PROJECT_SYNC_OAUTH_ALLOW";

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
    revocation_endpoint: Option<String>,
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
    pub revocation_endpoint: Option<String>,
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
        revocation_endpoint: token.revocation_endpoint,
        client_id: Some(token.client_id),
        scope: token.scope,
    };
    let payload = serde_json::to_string(&stored).context("serialize oauth token")?;
    entry
        .set_password(&payload)
        .context("write oauth token to keyring")
}

pub fn revoke_oauth_token(account: &str) -> anyhow::Result<()> {
    let entry = Entry::new(SERVICE, account).context("open keyring entry")?;
    let value = entry.get_password().context("read token from keyring")?;
    let stored: StoredToken = serde_json::from_str(&value).context("decode oauth token")?;
    if stored.kind != "oauth" {
        anyhow::bail!("stored token is not an OAuth token");
    }
    entry
        .delete_password()
        .context("delete oauth token from keyring")?;
    audit_event("oauth.revoke", AuditStatus::Ok, account, None);
    Ok(())
}

fn ensure_oauth_token(
    account: &str,
    entry: &Entry,
    mut stored: StoredToken,
) -> anyhow::Result<String> {
    if !requires_refresh(&stored, EXPIRY_LEEWAY_SECS) {
        return Ok(stored.access_token);
    }

    audit_event("oauth.refresh.start", AuditStatus::Ok, account, None);
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
        audit_event("oauth.refresh", AuditStatus::Failed, account, Some(&message));
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
    audit_event("oauth.refresh", AuditStatus::Ok, account, None);
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

pub fn set_audit_logger(logger: AuditLogger) {
    let _ = AUDIT_LOGGER.set(logger);
}

pub fn oauth_allowed(provider: &str, host: &str) -> bool {
    if let Ok(value) = std::env::var(OAUTH_ALLOW_ENV) {
        if !value.trim().is_empty() {
            return parse_oauth_allowlist(&value)
                .get(provider)
                .map(|hosts| host_matches_any(host, hosts))
                .unwrap_or(false);
        }
    }
    default_oauth_allowed(provider, host)
}

fn parse_oauth_allowlist(value: &str) -> std::collections::HashMap<String, Vec<String>> {
    let mut map = std::collections::HashMap::new();
    for entry in value.split(';').map(|s| s.trim()).filter(|s| !s.is_empty()) {
        let mut iter = entry.splitn(2, '=');
        let provider = iter.next().unwrap_or("").trim().to_string();
        let hosts = iter
            .next()
            .unwrap_or("")
            .split(',')
            .map(|host| host.trim().to_string())
            .filter(|host| !host.is_empty())
            .collect::<Vec<_>>();
        if !provider.is_empty() && !hosts.is_empty() {
            map.insert(provider, hosts);
        }
    }
    map
}

fn host_matches_any(host: &str, patterns: &[String]) -> bool {
    let host = normalize_host(host);
    patterns.iter().any(|pattern| match_host(&host, pattern))
}

fn match_host(host: &str, pattern: &str) -> bool {
    let pattern = normalize_host(pattern);
    if let Some(stripped) = pattern.strip_prefix("*.") {
        return host.ends_with(stripped);
    }
    host == pattern
}

fn normalize_host(value: &str) -> String {
    let lower = value.trim().to_lowercase();
    if let Ok(url) = reqwest::Url::parse(&lower) {
        if let Some(host) = url.host_str() {
            return host.to_string();
        }
    }
    lower
}

fn default_oauth_allowed(provider: &str, host: &str) -> bool {
    let host = normalize_host(host);
    match provider {
        "github" => host == "github.com",
        "azure-devops" => host.ends_with("dev.azure.com") || host.ends_with("visualstudio.com"),
        _ => false,
    }
}

fn audit_event(event: &str, status: AuditStatus, account: &str, error: Option<&str>) {
    if let Some(logger) = AUDIT_LOGGER.get() {
        let details = json!({ "account": account });
        let _ = logger.record(event, status, Some("oauth"), Some(details), error);
    }
}

static AUDIT_LOGGER: OnceLock<AuditLogger> = OnceLock::new();

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
            revocation_endpoint: None,
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
            revocation_endpoint: None,
            client_id: "client".to_string(),
            scope: Some("scope".to_string()),
        };
        let stored = StoredToken {
            kind: "oauth".to_string(),
            access_token: token.access_token.clone(),
            refresh_token: token.refresh_token.clone(),
            expires_at: token.expires_at,
            token_endpoint: Some(token.token_endpoint.clone()),
            revocation_endpoint: token.revocation_endpoint.clone(),
            client_id: Some(token.client_id.clone()),
            scope: token.scope.clone(),
        };
        let json = serde_json::to_string(&stored).unwrap();
        let decoded: StoredToken = serde_json::from_str(&json).unwrap();
        assert_eq!(decoded.kind, "oauth");
        assert_eq!(decoded.access_token, "access");
        assert_eq!(decoded.refresh_token.as_deref(), Some("refresh"));
    }

    #[test]
    fn oauth_allowlist_parses_entries() {
        let map = parse_oauth_allowlist(
            "github=github.com;azure-devops=dev.azure.com,visualstudio.com",
        );
        assert_eq!(map.get("github").unwrap(), &vec!["github.com".to_string()]);
        assert_eq!(
            map.get("azure-devops").unwrap(),
            &vec!["dev.azure.com".to_string(), "visualstudio.com".to_string()]
        );
    }

    #[test]
    fn host_match_supports_wildcards() {
        assert!(match_host("sub.example.com", "*.example.com"));
        assert!(match_host("example.com", "example.com"));
        assert!(match_host("dev.azure.com", "*.azure.com"));
    }

    #[test]
    fn default_oauth_allowed_matches_known_hosts() {
        assert!(default_oauth_allowed("github", "https://github.com"));
        assert!(default_oauth_allowed("azure-devops", "https://dev.azure.com"));
        assert!(!default_oauth_allowed("gitlab", "https://gitlab.com"));
    }
}

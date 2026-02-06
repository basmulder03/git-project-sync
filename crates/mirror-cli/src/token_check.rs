use anyhow::bail;
use mirror_core::model::{ProviderKind, ProviderTarget};
use mirror_providers::ProviderRegistry;

use crate::update;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum TokenValidity {
    Ok,
    Invalid,
    ScopeNotFound,
    Network,
    Error,
}

#[derive(Debug, Clone)]
pub struct TokenCheckResult {
    pub status: TokenValidity,
    pub error: Option<String>,
}

impl TokenCheckResult {
    pub fn ok() -> Self {
        Self {
            status: TokenValidity::Ok,
            error: None,
        }
    }

    pub fn message(&self, target: &ProviderTarget) -> String {
        let scope = target.scope.segments().join("/");
        match self.status {
            TokenValidity::Ok => format!("Token valid for {} {scope}", target.provider.as_prefix()),
            TokenValidity::Invalid => format!(
                "{} authentication failed for scope {scope}. Check your PAT.",
                target.provider.as_prefix()
            ),
            TokenValidity::ScopeNotFound => format!(
                "{} scope not found: {scope}. Check scope configuration.",
                target.provider.as_prefix()
            ),
            TokenValidity::Network => "Network unavailable while validating token".to_string(),
            TokenValidity::Error => "Token validation failed".to_string(),
        }
    }
}

pub fn check_token_validity(target: &ProviderTarget) -> TokenCheckResult {
    let registry = ProviderRegistry::new();
    let adapter = match registry.provider(target.provider.clone()) {
        Ok(adapter) => adapter,
        Err(err) => {
            return TokenCheckResult {
                status: TokenValidity::Error,
                error: Some(err.to_string()),
            };
        }
    };

    match adapter.health_check(target) {
        Ok(_) => TokenCheckResult::ok(),
        Err(err) => classify_error(target.provider.clone(), &err),
    }
}

pub fn ensure_token_valid(target: &ProviderTarget) -> anyhow::Result<TokenCheckResult> {
    let result = check_token_validity(target);
    if result.status != TokenValidity::Ok {
        bail!("Token validation failed: {}", result.message(target));
    }
    Ok(result)
}

fn classify_error(_provider: ProviderKind, err: &anyhow::Error) -> TokenCheckResult {
    if update::is_network_error(err) {
        return TokenCheckResult {
            status: TokenValidity::Network,
            error: Some(err.to_string()),
        };
    }

    if let Some(reqwest_err) = err.downcast_ref::<reqwest::Error>()
        && let Some(status) = reqwest_err.status()
    {
        let status_kind = match status {
            reqwest::StatusCode::UNAUTHORIZED | reqwest::StatusCode::FORBIDDEN => {
                TokenValidity::Invalid
            }
            reqwest::StatusCode::NOT_FOUND => TokenValidity::ScopeNotFound,
            _ => TokenValidity::Error,
        };
        return TokenCheckResult {
            status: status_kind,
            error: Some(err.to_string()),
        };
    }

    TokenCheckResult {
        status: TokenValidity::Error,
        error: Some(err.to_string()),
    }
}

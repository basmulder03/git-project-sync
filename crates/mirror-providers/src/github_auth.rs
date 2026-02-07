use anyhow::anyhow;
use mirror_core::model::{ProviderScope, ProviderTarget};
use mirror_core::provider::ProviderSpec;

use crate::auth;
use crate::github_scope::{is_public_github_api_host, is_public_github_web_host};
use crate::spec::GitHubSpec;

pub(crate) fn account_candidates(
    spec: &GitHubSpec,
    host: &str,
    scope: &ProviderScope,
) -> anyhow::Result<Vec<String>> {
    let mut hosts = vec![host.trim_end_matches('/').to_string()];
    if is_public_github_api_host(host) {
        hosts.push("https://github.com".to_string());
        hosts.push("github.com".to_string());
    } else if is_public_github_web_host(host) {
        hosts.push("https://api.github.com".to_string());
        hosts.push("api.github.com".to_string());
    }
    hosts.dedup();
    hosts
        .into_iter()
        .map(|candidate| spec.account_key(&candidate, scope))
        .collect()
}

pub(crate) fn get_pat_for_target(
    spec: &GitHubSpec,
    host: &str,
    target: &ProviderTarget,
) -> anyhow::Result<String> {
    let accounts = account_candidates(spec, host, &target.scope)?;
    let mut last_no_entry: Option<anyhow::Error> = None;
    let mut first_err: Option<String> = None;
    for account in accounts {
        match auth::get_pat(&account) {
            Ok(token) => return Ok(token),
            Err(err) if is_missing_keyring_entry(&err) => {
                if first_err.is_none() {
                    first_err = Some(format!("{err:#}"));
                }
                last_no_entry = Some(err);
            }
            Err(err) => {
                return Err(err);
            }
        }
    }
    if let Some(err) = last_no_entry {
        return Err(err);
    }
    if let Some(err) = first_err {
        return Err(anyhow!(err));
    }
    Err(anyhow!("read token from keyring"))
}

fn is_missing_keyring_entry(err: &anyhow::Error) -> bool {
    let value = err.to_string().to_ascii_lowercase();
    value.contains("no matching entry found")
        || value.contains("no entry")
        || value.contains("not found in secure storage")
        || value.contains("item not found")
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn account_candidates_include_public_aliases() {
        let spec = GitHubSpec;
        let scope = ProviderScope::new(vec!["me".to_string()]).unwrap();
        let candidates = account_candidates(&spec, "https://api.github.com", &scope).unwrap();
        assert!(
            candidates
                .iter()
                .any(|item| item.contains("github:https://api.github.com:me"))
        );
        assert!(
            candidates
                .iter()
                .any(|item| item.contains("github:https://github.com:me"))
        );
    }
}

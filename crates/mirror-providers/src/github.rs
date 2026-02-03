use crate::auth;
use crate::RepoProvider;
use crate::spec::{GitHubSpec, host_or_default};
use anyhow::Context;
use mirror_core::model::{ProviderKind, ProviderScope, ProviderTarget, RemoteRepo, RepoAuth};
use mirror_core::provider::ProviderSpec;
use reqwest::blocking::Client;
use reqwest::header::HeaderMap;
use serde::Deserialize;
use tracing::info;

pub struct GitHubProvider {
    client: Client,
}

impl GitHubProvider {
    pub fn new() -> anyhow::Result<Self> {
        Ok(Self {
            client: Client::new(),
        })
    }

    fn parse_scope(scope: &ProviderScope) -> anyhow::Result<&str> {
        let segments = scope.segments();
        if segments.len() != 1 {
            anyhow::bail!("github scope requires a single org/user segment");
        }
        Ok(segments[0].as_str())
    }

    fn normalize_branch(value: Option<String>) -> String {
        value
            .unwrap_or_else(|| "main".to_string())
            .trim_start_matches("refs/heads/")
            .to_string()
    }

    fn next_page(headers: &HeaderMap) -> Option<u32> {
        let link = headers.get("link")?.to_str().ok()?;
        for part in link.split(',') {
            let part = part.trim();
            if !part.contains("rel=\"next\"") {
                continue;
            }
            let start = part.find('<')? + 1;
            let end = part.find('>')?;
            let url = &part[start..end];
            for pair in url.split('?').nth(1).unwrap_or("").split('&') {
                let mut iter = pair.splitn(2, '=');
                let key = iter.next().unwrap_or("");
                let value = iter.next().unwrap_or("");
                if key == "page" {
                    if let Ok(page) = value.parse::<u32>() {
                        return Some(page);
                    }
                }
            }
        }
        None
    }
}

impl RepoProvider for GitHubProvider {
    fn kind(&self) -> ProviderKind {
        ProviderKind::GitHub
    }

    fn list_repos(&self, target: &ProviderTarget) -> anyhow::Result<Vec<RemoteRepo>> {
        if target.provider != ProviderKind::GitHub {
            anyhow::bail!("invalid provider target for GitHub");
        }
        let spec = GitHubSpec;
        let host = host_or_default(target.host.as_deref(), &spec);
        let org = Self::parse_scope(&target.scope)?;
        let account = spec.account_key(&host, &target.scope)?;
        let token = auth::get_pat(&account)?;

        let mut page = 1;
        let mut repos = Vec::new();
        let auth = RepoAuth {
            username: "pat".to_string(),
            token: token.clone(),
        };

        loop {
            let url = format!("{host}/orgs/{org}/repos?per_page=100&page={page}");
            info!(org, page, "listing GitHub repos");
            let response = self
                .client
                .get(url)
                .header("User-Agent", "git-project-sync")
                .bearer_auth(token.as_str())
                .send()
                .context("call GitHub list repos")?
                .error_for_status()
                .context("GitHub list repos status")?;
            let next_page = Self::next_page(response.headers());
            let payload: Vec<RepoItem> = response.json().context("decode repos response")?;
            if payload.is_empty() {
                break;
            }
            for repo in payload {
                repos.push(RemoteRepo {
                    id: repo.id.to_string(),
                    name: repo.name.clone(),
                    clone_url: repo.clone_url,
                    default_branch: Self::normalize_branch(repo.default_branch),
                    provider: ProviderKind::GitHub,
                    scope: target.scope.clone(),
                    auth: Some(auth.clone()),
                });
            }
            if let Some(next) = next_page {
                page = next;
            } else {
                break;
            }
        }

        Ok(repos)
    }

    fn validate_auth(&self, target: &ProviderTarget) -> anyhow::Result<()> {
        let spec = GitHubSpec;
        let host = host_or_default(target.host.as_deref(), &spec);
        let _ = Self::parse_scope(&target.scope)?;
        let account = spec.account_key(&host, &target.scope)?;
        let _ = auth::get_pat(&account)?;
        Ok(())
    }

    fn health_check(&self, target: &ProviderTarget) -> anyhow::Result<()> {
        if target.provider != ProviderKind::GitHub {
            anyhow::bail!("invalid provider target for GitHub");
        }
        let spec = GitHubSpec;
        let host = host_or_default(target.host.as_deref(), &spec);
        let org = Self::parse_scope(&target.scope)?;
        let account = spec.account_key(&host, &target.scope)?;
        let token = auth::get_pat(&account)?;

        let url = format!("{host}/orgs/{org}/repos?per_page=1&page=1");
        let response = self
            .client
            .get(url)
            .header("User-Agent", "git-project-sync")
            .bearer_auth(token.as_str())
            .send()
            .context("call GitHub health check")?
            .error_for_status()
            .context("GitHub health check status")?;
        let _payload: Vec<RepoItem> = response.json().context("decode health response")?;
        Ok(())
    }
}

#[derive(Debug, Deserialize)]
struct RepoItem {
    id: u64,
    name: String,
    clone_url: String,
    default_branch: Option<String>,
}

#[cfg(test)]
mod tests {
    use super::*;
    use reqwest::header::HeaderValue;

    #[test]
    fn next_page_parses_link_header() {
        let mut headers = HeaderMap::new();
        headers.insert(
            "link",
            HeaderValue::from_static(
                "<https://api.github.com/orgs/test/repos?per_page=100&page=2>; rel=\"next\"",
            ),
        );
        assert_eq!(GitHubProvider::next_page(&headers), Some(2));
    }

    #[test]
    fn normalize_branch_trims_refs() {
        let value = Some("refs/heads/main".to_string());
        assert_eq!(GitHubProvider::normalize_branch(value), "main");
    }
}

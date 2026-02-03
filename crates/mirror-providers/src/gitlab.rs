use crate::auth;
use crate::RepoProvider;
use crate::spec::{GitLabSpec, host_or_default};
use anyhow::Context;
use mirror_core::model::{ProviderKind, ProviderScope, ProviderTarget, RemoteRepo, RepoAuth};
use mirror_core::provider::ProviderSpec;
use reqwest::blocking::Client;
use reqwest::header::HeaderMap;
use serde::Deserialize;
use tracing::info;

pub struct GitLabProvider {
    client: Client,
}

impl GitLabProvider {
    pub fn new() -> anyhow::Result<Self> {
        Ok(Self {
            client: Client::new(),
        })
    }

    fn parse_scope(scope: &ProviderScope) -> anyhow::Result<String> {
        let segments = scope.segments();
        if segments.is_empty() {
            anyhow::bail!("gitlab scope requires at least one group segment");
        }
        Ok(segments.join("/"))
    }

    fn normalize_branch(value: Option<String>) -> String {
        value
            .unwrap_or_else(|| "main".to_string())
            .trim_start_matches("refs/heads/")
            .to_string()
    }

    fn next_page(headers: &HeaderMap) -> Option<u32> {
        headers
            .get("x-next-page")
            .and_then(|value| value.to_str().ok())
            .and_then(|value| value.parse::<u32>().ok())
    }
}

impl RepoProvider for GitLabProvider {
    fn kind(&self) -> ProviderKind {
        ProviderKind::GitLab
    }

    fn list_repos(&self, target: &ProviderTarget) -> anyhow::Result<Vec<RemoteRepo>> {
        if target.provider != ProviderKind::GitLab {
            anyhow::bail!("invalid provider target for GitLab");
        }
        let spec = GitLabSpec;
        let host = host_or_default(target.host.as_deref(), &spec);
        let group = Self::parse_scope(&target.scope)?;
        let account = spec.account_key(&host, &target.scope)?;
        let token = auth::get_pat(&account)?;

        let mut page = 1;
        let mut repos = Vec::new();
        let auth = RepoAuth {
            username: "pat".to_string(),
            token: token.clone(),
        };

        loop {
            let url =
                format!("{host}/groups/{group}/projects?per_page=100&page={page}");
            info!(group, page, "listing GitLab repos");
            let response = self
                .client
                .get(url)
                .header("PRIVATE-TOKEN", token.as_str())
                .send()
                .context("call GitLab list repos")?
                .error_for_status()
                .context("GitLab list repos status")?;
            let next_page = Self::next_page(response.headers());
            let payload: Vec<ProjectItem> =
                response.json().context("decode repos response")?;
            if payload.is_empty() {
                break;
            }
            for repo in payload {
                repos.push(RemoteRepo {
                    id: repo.id.to_string(),
                    name: repo.name.clone(),
                    clone_url: repo.http_url_to_repo,
                    default_branch: Self::normalize_branch(repo.default_branch),
                    provider: ProviderKind::GitLab,
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
        let spec = GitLabSpec;
        let host = host_or_default(target.host.as_deref(), &spec);
        let _ = Self::parse_scope(&target.scope)?;
        let account = spec.account_key(&host, &target.scope)?;
        let _ = auth::get_pat(&account)?;
        Ok(())
    }

    fn auth_for_target(&self, target: &ProviderTarget) -> anyhow::Result<Option<RepoAuth>> {
        let spec = GitLabSpec;
        let host = host_or_default(target.host.as_deref(), &spec);
        let _ = Self::parse_scope(&target.scope)?;
        let account = spec.account_key(&host, &target.scope)?;
        let token = auth::get_pat(&account)?;
        Ok(Some(RepoAuth {
            username: "pat".to_string(),
            token,
        }))
    }

    fn health_check(&self, target: &ProviderTarget) -> anyhow::Result<()> {
        if target.provider != ProviderKind::GitLab {
            anyhow::bail!("invalid provider target for GitLab");
        }
        let spec = GitLabSpec;
        let host = host_or_default(target.host.as_deref(), &spec);
        let group = Self::parse_scope(&target.scope)?;
        let account = spec.account_key(&host, &target.scope)?;
        let token = auth::get_pat(&account)?;

        let url = format!("{host}/groups/{group}/projects?per_page=1&page=1");
        let response = self
            .client
            .get(url)
            .header("PRIVATE-TOKEN", token.as_str())
            .send()
            .context("call GitLab health check")?
            .error_for_status()
            .context("GitLab health check status")?;
        let _payload: Vec<ProjectItem> = response.json().context("decode health response")?;
        Ok(())
    }
}

#[derive(Debug, Deserialize)]
struct ProjectItem {
    id: u64,
    name: String,
    http_url_to_repo: String,
    default_branch: Option<String>,
}

#[cfg(test)]
mod tests {
    use super::*;
    use reqwest::header::HeaderValue;

    #[test]
    fn next_page_reads_gitlab_header() {
        let mut headers = HeaderMap::new();
        headers.insert("x-next-page", HeaderValue::from_static("3"));
        assert_eq!(GitLabProvider::next_page(&headers), Some(3));
    }

    #[test]
    fn normalize_branch_trims_refs() {
        let value = Some("refs/heads/main".to_string());
        assert_eq!(GitLabProvider::normalize_branch(value), "main");
    }
}

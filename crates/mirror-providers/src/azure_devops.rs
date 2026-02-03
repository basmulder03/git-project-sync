use crate::RepoProvider;
use crate::auth;
use crate::spec::{AzureDevOpsSpec, host_or_default};
use anyhow::Context;
use mirror_core::model::{ProviderKind, ProviderScope, ProviderTarget, RemoteRepo, RepoAuth};
use mirror_core::provider::ProviderSpec;
use reqwest::Url;
use reqwest::blocking::Client;
use reqwest::header::HeaderMap;
use serde::Deserialize;
use tracing::info;

pub struct AzureDevOpsProvider {
    client: Client,
}

impl AzureDevOpsProvider {
    pub fn new() -> anyhow::Result<Self> {
        Ok(Self {
            client: Client::new(),
        })
    }

    fn parse_scope(scope: &ProviderScope) -> anyhow::Result<(&str, Option<&str>)> {
        let segments = scope.segments();
        if segments.len() == 1 {
            return Ok((segments[0].as_str(), None));
        }
        if segments.len() != 2 {
            anyhow::bail!("azure devops scope requires org or org/project segments");
        }
        Ok((segments[0].as_str(), Some(segments[1].as_str())))
    }

    fn build_repos_url(
        host: &str,
        org: &str,
        project: Option<&str>,
        continuation: Option<&str>,
    ) -> anyhow::Result<Url> {
        let base = if let Some(project) = project {
            format!("{host}/{org}/{project}/_apis/git/repositories?api-version=7.1-preview.1")
        } else {
            format!("{host}/{org}/_apis/git/repositories?api-version=7.1-preview.1")
        };
        let mut url = Url::parse(&base).context("parse Azure DevOps repos url")?;
        if let Some(token) = continuation {
            url.query_pairs_mut()
                .append_pair("continuationToken", token);
        }
        Ok(url)
    }

    fn continuation_token(headers: &HeaderMap) -> Option<String> {
        headers
            .get("x-ms-continuationtoken")
            .and_then(|value| value.to_str().ok())
            .map(|value| value.to_string())
    }
}

impl RepoProvider for AzureDevOpsProvider {
    fn kind(&self) -> ProviderKind {
        ProviderKind::AzureDevOps
    }

    fn list_repos(&self, target: &ProviderTarget) -> anyhow::Result<Vec<RemoteRepo>> {
        if target.provider != ProviderKind::AzureDevOps {
            anyhow::bail!("invalid provider target for Azure DevOps");
        }
        let spec = AzureDevOpsSpec;
        let host = host_or_default(target.host.as_deref(), &spec);
        let (org, project) = Self::parse_scope(&target.scope)?;
        let account = spec.account_key(&host, &target.scope)?;
        let pat = auth::get_pat(&account)?;

        let auth = RepoAuth {
            username: "pat".to_string(),
            token: pat,
        };
        let mut repos = Vec::new();
        let mut continuation: Option<String> = None;

        loop {
            let url = Self::build_repos_url(&host, org, project, continuation.as_deref())?;
            info!(org, project = ?project, "listing Azure DevOps repos");
            let response = self
                .client
                .get(url)
                .basic_auth("", Some(auth.token.as_str()))
                .send()
                .context("call Azure DevOps list repos")?
                .error_for_status()
                .context("Azure DevOps list repos status")?;
            let next = Self::continuation_token(response.headers());
            let payload: ReposResponse = response.json().context("decode repos response")?;

            for repo in payload.value {
                let scope = match (project, repo.project.as_ref()) {
                    (Some(_), _) => target.scope.clone(),
                    (None, Some(project)) => ProviderScope::new(vec![
                        org.to_string(),
                        project.name.clone(),
                    ])?,
                    (None, None) => anyhow::bail!(
                        "Azure DevOps repo missing project for org-wide listing"
                    ),
                };
                repos.push(RemoteRepo {
                    id: repo.id,
                    name: repo.name.clone(),
                    clone_url: repo.remote_url,
                    default_branch: repo
                        .default_branch
                        .unwrap_or_else(|| "refs/heads/main".to_string())
                        .trim_start_matches("refs/heads/")
                        .to_string(),
                    provider: ProviderKind::AzureDevOps,
                    scope,
                    auth: Some(auth.clone()),
                });
            }

            if next.is_none() {
                break;
            }
            continuation = next;
        }

        Ok(repos)
    }

    fn validate_auth(&self, target: &ProviderTarget) -> anyhow::Result<()> {
        let spec = AzureDevOpsSpec;
        let host = host_or_default(target.host.as_deref(), &spec);
        let _ = Self::parse_scope(&target.scope)?;
        let account = spec.account_key(&host, &target.scope)?;
        let _ = auth::get_pat(&account)?;
        Ok(())
    }

    fn health_check(&self, target: &ProviderTarget) -> anyhow::Result<()> {
        if target.provider != ProviderKind::AzureDevOps {
            anyhow::bail!("invalid provider target for Azure DevOps");
        }
        let spec = AzureDevOpsSpec;
        let host = host_or_default(target.host.as_deref(), &spec);
        let (org, project) = Self::parse_scope(&target.scope)?;
        let account = spec.account_key(&host, &target.scope)?;
        let pat = auth::get_pat(&account)?;

        let url = Self::build_repos_url(&host, org, project, None)?;
        let response = self
            .client
            .get(url)
            .basic_auth("", Some(pat.as_str()))
            .send()
            .context("call Azure DevOps health check")?
            .error_for_status()
            .context("Azure DevOps health check status")?;
        let _payload: ReposResponse = response.json().context("decode health response")?;
        Ok(())
    }
}

#[derive(Debug, Deserialize)]
struct ReposResponse {
    value: Vec<RepoItem>,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct RepoItem {
    id: String,
    name: String,
    remote_url: String,
    default_branch: Option<String>,
    project: Option<ProjectRef>,
}

#[derive(Debug, Deserialize)]
struct ProjectRef {
    name: String,
}

#[cfg(test)]
mod tests {
    use super::*;
    use reqwest::header::HeaderValue;

    #[test]
    fn parse_scope_allows_org_only() {
        let scope = ProviderScope::new(vec!["org".into()]).unwrap();
        let (org, project) = AzureDevOpsProvider::parse_scope(&scope).unwrap();
        assert_eq!(org, "org");
        assert!(project.is_none());
    }

    #[test]
    fn parse_scope_allows_org_project() {
        let scope = ProviderScope::new(vec!["org".into(), "proj".into()]).unwrap();
        let (org, project) = AzureDevOpsProvider::parse_scope(&scope).unwrap();
        assert_eq!(org, "org");
        assert_eq!(project, Some("proj"));
    }

    #[test]
    fn continuation_token_reads_header() {
        let mut headers = HeaderMap::new();
        headers.insert("x-ms-continuationtoken", HeaderValue::from_static("token-123"));
        let token = AzureDevOpsProvider::continuation_token(&headers);
        assert_eq!(token, Some("token-123".to_string()));
    }
}

use crate::RepoProvider;
use crate::auth;
use anyhow::Context;
use mirror_core::model::{ProviderKind, ProviderScope, ProviderTarget, RemoteRepo, RepoAuth};
use reqwest::blocking::Client;
use serde::Deserialize;
use tracing::info;

const DEFAULT_HOST: &str = "https://dev.azure.com";

pub struct AzureDevOpsProvider {
    client: Client,
}

impl AzureDevOpsProvider {
    pub fn new() -> anyhow::Result<Self> {
        Ok(Self {
            client: Client::new(),
        })
    }

    fn host_for(&self, target: &ProviderTarget) -> String {
        target
            .host
            .clone()
            .unwrap_or_else(|| DEFAULT_HOST.to_string())
    }

    fn account_key(&self, host: &str, org: &str) -> String {
        format!("azdo:{host}:{org}")
    }

    fn parse_scope(scope: &ProviderScope) -> anyhow::Result<(&str, &str)> {
        let segments = scope.segments();
        if segments.len() != 2 {
            anyhow::bail!("azure devops scope requires org and project segments");
        }
        Ok((segments[0].as_str(), segments[1].as_str()))
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
        let host = self.host_for(target);
        let (org, project) = Self::parse_scope(&target.scope)?;
        let account = self.account_key(&host, org);
        let pat = auth::get_pat(&account)?;

        let url =
            format!("{host}/{org}/{project}/_apis/git/repositories?api-version=7.1-preview.1");
        info!(org, project, "listing Azure DevOps repos");
        let response = self
            .client
            .get(url)
            .basic_auth("", Some(pat.as_str()))
            .send()
            .context("call Azure DevOps list repos")?
            .error_for_status()
            .context("Azure DevOps list repos status")?;
        let payload: ReposResponse = response.json().context("decode repos response")?;

        let auth = RepoAuth {
            username: "pat".to_string(),
            token: pat,
        };
        let repos = payload
            .value
            .into_iter()
            .map(|repo| RemoteRepo {
                id: repo.id,
                name: repo.name.clone(),
                clone_url: repo.remote_url,
                default_branch: repo
                    .default_branch
                    .unwrap_or_else(|| "refs/heads/main".to_string())
                    .trim_start_matches("refs/heads/")
                    .to_string(),
                provider: ProviderKind::AzureDevOps,
                scope: target.scope.clone(),
                auth: Some(auth.clone()),
            })
            .collect();
        Ok(repos)
    }

    fn validate_auth(&self, target: &ProviderTarget) -> anyhow::Result<()> {
        let host = self.host_for(target);
        let (org, _project) = Self::parse_scope(&target.scope)?;
        let account = self.account_key(&host, org);
        let _ = auth::get_pat(&account)?;
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
}

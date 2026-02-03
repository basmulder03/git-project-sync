use crate::auth;
use crate::RepoProvider;
use anyhow::Context;
use mirror_core::model::{ProviderKind, ProviderScope, ProviderTarget, RemoteRepo, RepoAuth};
use reqwest::blocking::Client;
use serde::Deserialize;
use tracing::info;

const DEFAULT_HOST: &str = "https://gitlab.com/api/v4";

pub struct GitLabProvider {
    client: Client,
}

impl GitLabProvider {
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
            .trim_end_matches('/')
            .to_string()
    }

    fn account_key(&self, host: &str, group: &str) -> String {
        format!("gitlab:{host}:{group}")
    }

    fn parse_scope(scope: &ProviderScope) -> anyhow::Result<String> {
        let segments = scope.segments();
        if segments.is_empty() {
            anyhow::bail!("gitlab scope requires at least one group segment");
        }
        Ok(segments.join("/"))
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
        let host = self.host_for(target);
        let group = Self::parse_scope(&target.scope)?;
        let account = self.account_key(&host, &group);
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
                    default_branch: repo
                        .default_branch
                        .unwrap_or_else(|| "main".to_string()),
                    provider: ProviderKind::GitLab,
                    scope: target.scope.clone(),
                    auth: Some(auth.clone()),
                });
            }
            page += 1;
        }

        Ok(repos)
    }

    fn validate_auth(&self, target: &ProviderTarget) -> anyhow::Result<()> {
        let host = self.host_for(target);
        let group = Self::parse_scope(&target.scope)?;
        let account = self.account_key(&host, &group);
        let _ = auth::get_pat(&account)?;
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

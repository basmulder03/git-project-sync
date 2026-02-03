use crate::auth;
use crate::RepoProvider;
use anyhow::Context;
use mirror_core::model::{ProviderKind, ProviderScope, ProviderTarget, RemoteRepo, RepoAuth};
use reqwest::blocking::Client;
use serde::Deserialize;
use tracing::info;

const DEFAULT_HOST: &str = "https://api.github.com";

pub struct GitHubProvider {
    client: Client,
}

impl GitHubProvider {
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

    fn account_key(&self, host: &str, org: &str) -> String {
        format!("github:{host}:{org}")
    }

    fn parse_scope(scope: &ProviderScope) -> anyhow::Result<&str> {
        let segments = scope.segments();
        if segments.len() != 1 {
            anyhow::bail!("github scope requires a single org/user segment");
        }
        Ok(segments[0].as_str())
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
        let host = self.host_for(target);
        let org = Self::parse_scope(&target.scope)?;
        let account = self.account_key(&host, org);
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
            let payload: Vec<RepoItem> = response.json().context("decode repos response")?;
            if payload.is_empty() {
                break;
            }
            for repo in payload {
                repos.push(RemoteRepo {
                    id: repo.id.to_string(),
                    name: repo.name.clone(),
                    clone_url: repo.clone_url,
                    default_branch: repo
                        .default_branch
                        .unwrap_or_else(|| "main".to_string()),
                    provider: ProviderKind::GitHub,
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
        let org = Self::parse_scope(&target.scope)?;
        let account = self.account_key(&host, org);
        let _ = auth::get_pat(&account)?;
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

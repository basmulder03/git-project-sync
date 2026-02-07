use crate::RepoProvider;
use crate::auth;
use crate::azure_models::ReposResponse;
use crate::azure_scope::{build_repos_url, continuation_token, parse_scope};
use crate::http::send_with_retry;
use crate::spec::{AzureDevOpsSpec, host_or_default, pat_help};
use anyhow::Context;
use mirror_core::model::{ProviderKind, ProviderScope, ProviderTarget, RemoteRepo, RepoAuth};
use mirror_core::provider::{ProviderFuture, ProviderSpec};
use reqwest::Client;
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
}

impl RepoProvider for AzureDevOpsProvider {
    fn kind(&self) -> ProviderKind {
        ProviderKind::AzureDevOps
    }

    fn list_repos<'a>(&'a self, target: &'a ProviderTarget) -> ProviderFuture<'a, Vec<RemoteRepo>> {
        Box::pin(async move {
            if target.provider != ProviderKind::AzureDevOps {
                anyhow::bail!("invalid provider target for Azure DevOps");
            }
            let spec = AzureDevOpsSpec;
            let host = host_or_default(target.host.as_deref(), &spec);
            let (org, project) = parse_scope(&target.scope)?;
            let account = spec.account_key(&host, &target.scope)?;
            let pat = auth::get_pat(&account)?;

            let mut repos = Vec::new();
            let mut continuation: Option<String> = None;

            loop {
                let url = build_repos_url(&host, org, project, continuation.as_deref())?;
                info!(org, project = ?project, "listing Azure DevOps repos");
                let builder = self.client.get(url).basic_auth("", Some(pat.as_str()));
                let response = send_with_retry(|| builder.try_clone().context("clone request"))
                    .await
                    .context("call Azure DevOps list repos")?
                    .error_for_status()
                    .context("Azure DevOps list repos status")?;
                let next = continuation_token(response.headers());
                let payload: ReposResponse =
                    response.json().await.context("decode repos response")?;

                for repo in payload.value {
                    let scope = match (project, repo.project.as_ref()) {
                        (Some(_), _) => target.scope.clone(),
                        (None, Some(project)) => {
                            ProviderScope::new(vec![org.to_string(), project.name.clone()])?
                        }
                        (None, None) => {
                            anyhow::bail!("Azure DevOps repo missing project for org-wide listing")
                        }
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
                        archived: repo.is_disabled.unwrap_or(false),
                        provider: ProviderKind::AzureDevOps,
                        scope,
                    });
                }

                if next.is_none() {
                    break;
                }
                continuation = next;
            }

            Ok(repos)
        })
    }

    fn validate_auth<'a>(&'a self, target: &'a ProviderTarget) -> ProviderFuture<'a, ()> {
        Box::pin(async move {
            let spec = AzureDevOpsSpec;
            let host = host_or_default(target.host.as_deref(), &spec);
            let _ = parse_scope(&target.scope)?;
            let account = spec.account_key(&host, &target.scope)?;
            let _ = auth::get_pat(&account)?;
            Ok(())
        })
    }

    fn auth_for_target<'a>(
        &'a self,
        target: &'a ProviderTarget,
    ) -> ProviderFuture<'a, Option<RepoAuth>> {
        Box::pin(async move {
            let spec = AzureDevOpsSpec;
            let host = host_or_default(target.host.as_deref(), &spec);
            let _ = parse_scope(&target.scope)?;
            let account = spec.account_key(&host, &target.scope)?;
            let pat = auth::get_pat(&account)?;
            Ok(Some(RepoAuth {
                username: "pat".to_string(),
                token: pat,
            }))
        })
    }

    fn health_check<'a>(&'a self, target: &'a ProviderTarget) -> ProviderFuture<'a, ()> {
        Box::pin(async move {
            if target.provider != ProviderKind::AzureDevOps {
                anyhow::bail!("invalid provider target for Azure DevOps");
            }
            let spec = AzureDevOpsSpec;
            let host = host_or_default(target.host.as_deref(), &spec);
            let (org, project) = parse_scope(&target.scope)?;
            let account = spec.account_key(&host, &target.scope)?;
            let pat = auth::get_pat(&account)?;

            let url = build_repos_url(&host, org, project, None)?;
            let builder = self.client.get(url).basic_auth("", Some(pat.as_str()));
            let response = send_with_retry(|| builder.try_clone().context("clone request"))
                .await
                .context("call Azure DevOps health check")?
                .error_for_status()
                .context("Azure DevOps health check status")?;
            let _payload: ReposResponse =
                response.json().await.context("decode health response")?;
            Ok(())
        })
    }

    fn token_scopes<'a>(
        &'a self,
        target: &'a ProviderTarget,
    ) -> ProviderFuture<'a, Option<Vec<String>>> {
        Box::pin(async move {
            if target.provider != ProviderKind::AzureDevOps {
                anyhow::bail!("invalid provider target for Azure DevOps");
            }
            self.health_check(target).await?;
            let scopes = pat_help(ProviderKind::AzureDevOps)
                .scopes
                .iter()
                .map(|scope| scope.to_string())
                .collect();
            Ok(Some(scopes))
        })
    }

    fn register_webhook<'a>(
        &'a self,
        _target: &'a ProviderTarget,
        _url: &str,
        _secret: Option<&str>,
    ) -> ProviderFuture<'a, ()> {
        Box::pin(async { anyhow::bail!("Azure DevOps webhooks not supported yet") })
    }
}

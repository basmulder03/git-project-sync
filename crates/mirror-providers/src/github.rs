use crate::RepoProvider;
use crate::github_auth::get_pat_for_target;
use crate::github_models::parse_scopes_header;
use crate::github_paging::fetch_repos_page;
use crate::github_scope::{ScopeKind, normalize_branch, owner_matches, parse_scope};
use crate::http::{send_with_retry, send_with_retry_allow_statuses};
use crate::spec::{GitHubSpec, host_or_default};
use anyhow::Context;
use mirror_core::model::{ProviderKind, ProviderTarget, RemoteRepo, RepoAuth};
use reqwest::StatusCode;
use reqwest::blocking::Client;
use serde_json::json;

pub struct GitHubProvider {
    client: Client,
}

impl GitHubProvider {
    pub fn new() -> anyhow::Result<Self> {
        Ok(Self {
            client: Client::new(),
        })
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
        let scope = parse_scope(&target.scope)?;
        let token = get_pat_for_target(&spec, &host, target)?;

        let mut page = 1;
        let mut repos = Vec::new();
        let mut scope_kind = ScopeKind::Org;
        let mut saw_authenticated = false;
        let mut auth_had_results = false;
        let auth = RepoAuth {
            username: "pat".to_string(),
            token: token.clone(),
        };

        loop {
            let (payload, next_page, status) =
                fetch_repos_page(&self.client, &host, scope, token.as_str(), scope_kind, page)?;
            if status == StatusCode::NOT_FOUND && scope_kind == ScopeKind::Org {
                scope_kind = ScopeKind::AuthenticatedUser;
                page = 1;
                continue;
            }
            if status == StatusCode::NOT_FOUND && scope_kind == ScopeKind::AuthenticatedUser {
                scope_kind = ScopeKind::User;
                page = 1;
                continue;
            }
            if status == StatusCode::NOT_FOUND && scope_kind == ScopeKind::User {
                anyhow::bail!("GitHub scope not found: {scope}");
            }
            if payload.is_empty() {
                if scope_kind == ScopeKind::AuthenticatedUser {
                    scope_kind = ScopeKind::User;
                    page = 1;
                    continue;
                }
                break;
            }
            if scope_kind == ScopeKind::AuthenticatedUser {
                saw_authenticated = true;
            }
            for repo in payload {
                if scope_kind == ScopeKind::AuthenticatedUser && !owner_matches(scope, &repo) {
                    continue;
                }
                repos.push(RemoteRepo {
                    id: repo.id.to_string(),
                    name: repo.name.clone(),
                    clone_url: repo.clone_url,
                    default_branch: normalize_branch(repo.default_branch),
                    archived: repo.archived.unwrap_or(false),
                    provider: ProviderKind::GitHub,
                    scope: target.scope.clone(),
                    auth: Some(auth.clone()),
                });
            }
            if scope_kind == ScopeKind::AuthenticatedUser {
                if !repos.is_empty() {
                    auth_had_results = true;
                }
                if !auth_had_results && next_page.is_none() {
                    scope_kind = ScopeKind::User;
                    page = 1;
                    continue;
                }
            }
            if let Some(next) = next_page {
                page = next;
            } else {
                break;
            }
        }

        if saw_authenticated && repos.is_empty() {
            anyhow::bail!("GitHub scope not found: {scope}");
        }
        Ok(repos)
    }

    fn validate_auth(&self, target: &ProviderTarget) -> anyhow::Result<()> {
        let spec = GitHubSpec;
        let host = host_or_default(target.host.as_deref(), &spec);
        let _ = parse_scope(&target.scope)?;
        let _ = get_pat_for_target(&spec, &host, target)?;
        Ok(())
    }

    fn auth_for_target(&self, target: &ProviderTarget) -> anyhow::Result<Option<RepoAuth>> {
        let spec = GitHubSpec;
        let host = host_or_default(target.host.as_deref(), &spec);
        let _ = parse_scope(&target.scope)?;
        let token = get_pat_for_target(&spec, &host, target)?;
        Ok(Some(RepoAuth {
            username: "pat".to_string(),
            token,
        }))
    }

    fn health_check(&self, target: &ProviderTarget) -> anyhow::Result<()> {
        if target.provider != ProviderKind::GitHub {
            anyhow::bail!("invalid provider target for GitHub");
        }
        let spec = GitHubSpec;
        let host = host_or_default(target.host.as_deref(), &spec);
        let scope = parse_scope(&target.scope)?;
        let token = get_pat_for_target(&spec, &host, target)?;

        let (payload, _next, status) = fetch_repos_page(
            &self.client,
            &host,
            scope,
            token.as_str(),
            ScopeKind::Org,
            1,
        )?;
        if status == StatusCode::NOT_FOUND {
            let (payload, _next, status) = fetch_repos_page(
                &self.client,
                &host,
                scope,
                token.as_str(),
                ScopeKind::AuthenticatedUser,
                1,
            )?;
            if status == StatusCode::NOT_FOUND {
                let (_payload, _next, status) = fetch_repos_page(
                    &self.client,
                    &host,
                    scope,
                    token.as_str(),
                    ScopeKind::User,
                    1,
                )?;
                if status == StatusCode::NOT_FOUND {
                    anyhow::bail!("GitHub scope not found: {scope}");
                }
            } else {
                let owned = payload.into_iter().any(|repo| owner_matches(scope, &repo));
                if !owned {
                    let (_payload, _next, status) = fetch_repos_page(
                        &self.client,
                        &host,
                        scope,
                        token.as_str(),
                        ScopeKind::User,
                        1,
                    )?;
                    if status == StatusCode::NOT_FOUND {
                        anyhow::bail!("GitHub scope not found: {scope}");
                    }
                }
            }
        } else if payload.is_empty() {
            return Ok(());
        }
        Ok(())
    }

    fn register_webhook(
        &self,
        target: &ProviderTarget,
        url: &str,
        secret: Option<&str>,
    ) -> anyhow::Result<()> {
        if target.provider != ProviderKind::GitHub {
            anyhow::bail!("invalid provider target for GitHub");
        }
        let spec = GitHubSpec;
        let host = host_or_default(target.host.as_deref(), &spec);
        let org = parse_scope(&target.scope)?;
        let token = get_pat_for_target(&spec, &host, target)?;

        let mut config = json!({
            "url": url,
            "content_type": "json",
        });
        if let Some(secret) = secret {
            config["secret"] = json!(secret);
        }
        let body = json!({
            "name": "web",
            "active": true,
            "events": ["push"],
            "config": config,
        });

        let endpoint = format!("{host}/orgs/{org}/hooks");
        let builder = self
            .client
            .post(endpoint)
            .header("User-Agent", "git-project-sync")
            .bearer_auth(token.as_str())
            .json(&body);
        let response = send_with_retry_allow_statuses(
            || builder.try_clone().context("clone request"),
            &[StatusCode::NOT_FOUND],
        )
        .context("call GitHub webhook register")?;
        if response.status() == StatusCode::NOT_FOUND {
            anyhow::bail!("GitHub org not found or webhooks unsupported for user scopes");
        }
        let response = response
            .error_for_status()
            .context("GitHub webhook register status")?;
        let _ = response.text();
        Ok(())
    }

    fn token_scopes(&self, target: &ProviderTarget) -> anyhow::Result<Option<Vec<String>>> {
        if target.provider != ProviderKind::GitHub {
            anyhow::bail!("invalid provider target for GitHub");
        }
        let spec = GitHubSpec;
        let host = host_or_default(target.host.as_deref(), &spec);
        let _org = parse_scope(&target.scope)?;
        let token = get_pat_for_target(&spec, &host, target)?;

        let url = format!("{host}/user");
        let builder = self
            .client
            .get(url)
            .header("User-Agent", "git-project-sync")
            .bearer_auth(token.as_str());
        let response = send_with_retry(|| builder.try_clone().context("clone request"))
            .context("call GitHub token scopes")?
            .error_for_status()
            .context("GitHub token scopes status")?;
        if let Some(header) = response.headers().get("x-oauth-scopes")
            && let Ok(value) = header.to_str()
        {
            return Ok(Some(parse_scopes_header(value)));
        }
        Ok(None)
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::github_models::{RepoItem, RepoOwner};

    #[test]
    fn owner_matches_filters_user_scope() {
        let repo = RepoItem {
            id: 1,
            name: "repo".to_string(),
            clone_url: "https://example.com/repo.git".to_string(),
            default_branch: Some("main".to_string()),
            archived: Some(false),
            owner: Some(RepoOwner {
                login: "me".to_string(),
            }),
        };
        assert!(owner_matches("me", &repo));
        assert!(!owner_matches("other", &repo));
    }
}

use crate::RepoProvider;
use crate::auth;
use crate::http::{send_with_retry, send_with_retry_allow_statuses};
use crate::spec::{GitHubSpec, host_or_default};
use anyhow::Context;
use mirror_core::model::{ProviderKind, ProviderScope, ProviderTarget, RemoteRepo, RepoAuth};
use mirror_core::provider::ProviderSpec;
use reqwest::StatusCode;
use reqwest::blocking::Client;
use reqwest::header::HeaderMap;
use serde::Deserialize;
use serde_json::json;
use tracing::info;

pub struct GitHubProvider {
    client: Client,
}

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
enum ScopeKind {
    Org,
    User,
    AuthenticatedUser,
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

    fn repos_url(host: &str, scope: &str, kind: ScopeKind, page: u32) -> String {
        match kind {
            ScopeKind::Org => format!("{host}/orgs/{scope}/repos?per_page=100&page={page}"),
            ScopeKind::User => format!("{host}/users/{scope}/repos?per_page=100&page={page}"),
            ScopeKind::AuthenticatedUser => {
                format!("{host}/user/repos?per_page=100&page={page}&affiliation=owner")
            }
        }
    }

    fn owner_matches(scope: &str, repo: &RepoItem) -> bool {
        repo.owner
            .as_ref()
            .map(|owner| owner.login == scope)
            .unwrap_or(false)
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
                if key == "page"
                    && let Ok(page) = value.parse::<u32>()
                {
                    return Some(page);
                }
            }
        }
        None
    }

    fn fetch_repos_page(
        &self,
        host: &str,
        scope: &str,
        token: &str,
        kind: ScopeKind,
        page: u32,
    ) -> anyhow::Result<(Vec<RepoItem>, Option<u32>, StatusCode)> {
        let url = Self::repos_url(host, scope, kind, page);
        info!(scope, page, kind = ?kind, "listing GitHub repos");
        let builder = self
            .client
            .get(url)
            .header("User-Agent", "git-project-sync")
            .bearer_auth(token);
        let response = send_with_retry_allow_statuses(
            || builder.try_clone().expect("clone request"),
            &[StatusCode::NOT_FOUND],
        )
        .context("call GitHub list repos")?;
        let status = response.status();
        if status == StatusCode::NOT_FOUND {
            return Ok((Vec::new(), None, status));
        }
        let response = response
            .error_for_status()
            .context("GitHub list repos status")?;
        let next_page = Self::next_page(response.headers());
        let payload: Vec<RepoItem> = response.json().context("decode repos response")?;
        Ok((payload, next_page, status))
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
        let scope = Self::parse_scope(&target.scope)?;
        let account = spec.account_key(&host, &target.scope)?;
        let token = auth::get_pat(&account)?;

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
                self.fetch_repos_page(&host, scope, token.as_str(), scope_kind, page)?;
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
                if scope_kind == ScopeKind::AuthenticatedUser && !Self::owner_matches(scope, &repo)
                {
                    continue;
                }
                repos.push(RemoteRepo {
                    id: repo.id.to_string(),
                    name: repo.name.clone(),
                    clone_url: repo.clone_url,
                    default_branch: Self::normalize_branch(repo.default_branch),
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
        let _ = Self::parse_scope(&target.scope)?;
        let account = spec.account_key(&host, &target.scope)?;
        let _ = auth::get_pat(&account)?;
        Ok(())
    }

    fn auth_for_target(&self, target: &ProviderTarget) -> anyhow::Result<Option<RepoAuth>> {
        let spec = GitHubSpec;
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
        if target.provider != ProviderKind::GitHub {
            anyhow::bail!("invalid provider target for GitHub");
        }
        let spec = GitHubSpec;
        let host = host_or_default(target.host.as_deref(), &spec);
        let scope = Self::parse_scope(&target.scope)?;
        let account = spec.account_key(&host, &target.scope)?;
        let token = auth::get_pat(&account)?;

        let (payload, _next, status) =
            self.fetch_repos_page(&host, scope, token.as_str(), ScopeKind::Org, 1)?;
        if status == StatusCode::NOT_FOUND {
            let (payload, _next, status) = self.fetch_repos_page(
                &host,
                scope,
                token.as_str(),
                ScopeKind::AuthenticatedUser,
                1,
            )?;
            if status == StatusCode::NOT_FOUND {
                let (_payload, _next, status) =
                    self.fetch_repos_page(&host, scope, token.as_str(), ScopeKind::User, 1)?;
                if status == StatusCode::NOT_FOUND {
                    anyhow::bail!("GitHub scope not found: {scope}");
                }
            } else {
                let owned = payload
                    .into_iter()
                    .any(|repo| Self::owner_matches(scope, &repo));
                if !owned {
                    let (_payload, _next, status) =
                        self.fetch_repos_page(&host, scope, token.as_str(), ScopeKind::User, 1)?;
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
        let org = Self::parse_scope(&target.scope)?;
        let account = spec.account_key(&host, &target.scope)?;
        let token = auth::get_pat(&account)?;

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
            || builder.try_clone().expect("clone request"),
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
        let _org = Self::parse_scope(&target.scope)?;
        let account = spec.account_key(&host, &target.scope)?;
        let token = auth::get_pat(&account)?;

        let url = format!("{host}/user");
        let builder = self
            .client
            .get(url)
            .header("User-Agent", "git-project-sync")
            .bearer_auth(token.as_str());
        let response = send_with_retry(|| builder.try_clone().expect("clone request"))
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

#[derive(Debug, Deserialize)]
struct RepoItem {
    id: u64,
    name: String,
    clone_url: String,
    default_branch: Option<String>,
    archived: Option<bool>,
    owner: Option<RepoOwner>,
}

#[derive(Debug, Deserialize)]
struct RepoOwner {
    login: String,
}

fn parse_scopes_header(value: &str) -> Vec<String> {
    value
        .split(',')
        .map(|scope| scope.trim())
        .filter(|scope| !scope.is_empty())
        .map(|scope| scope.to_string())
        .collect()
}

#[cfg(test)]
mod tests {
    use super::*;
    use reqwest::header::HeaderValue;
    use serde_json::json;

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

    #[test]
    fn repo_item_deserializes_archived_flag() {
        let value = json!({
            "id": 1,
            "name": "repo",
            "clone_url": "https://example.com/repo.git",
            "default_branch": "main",
            "archived": true,
            "owner": { "login": "me" }
        });
        let repo: RepoItem = serde_json::from_value(value).unwrap();
        assert_eq!(repo.archived, Some(true));
    }

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
        assert!(GitHubProvider::owner_matches("me", &repo));
        assert!(!GitHubProvider::owner_matches("other", &repo));
    }

    #[test]
    fn parse_scopes_header_splits() {
        let scopes = parse_scopes_header("repo, read:org, ");
        assert_eq!(scopes, vec!["repo".to_string(), "read:org".to_string()]);
    }
}

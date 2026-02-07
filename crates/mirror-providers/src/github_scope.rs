use mirror_core::model::ProviderScope;

use crate::github_models::RepoItem;

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub(crate) enum ScopeKind {
    Org,
    User,
    AuthenticatedUser,
}

pub(crate) fn parse_scope(scope: &ProviderScope) -> anyhow::Result<&str> {
    let segments = scope.segments();
    if segments.len() != 1 {
        anyhow::bail!("github scope requires a single org/user segment");
    }
    Ok(segments[0].as_str())
}

pub(crate) fn normalize_branch(value: Option<String>) -> String {
    value
        .unwrap_or_else(|| "main".to_string())
        .trim_start_matches("refs/heads/")
        .to_string()
}

pub(crate) fn repos_url(host: &str, scope: &str, kind: ScopeKind, page: u32) -> String {
    match kind {
        ScopeKind::Org => format!("{host}/orgs/{scope}/repos?per_page=100&page={page}"),
        ScopeKind::User => format!("{host}/users/{scope}/repos?per_page=100&page={page}"),
        ScopeKind::AuthenticatedUser => {
            format!("{host}/user/repos?per_page=100&page={page}&affiliation=owner")
        }
    }
}

pub(crate) fn owner_matches(scope: &str, repo: &RepoItem) -> bool {
    repo.owner
        .as_ref()
        .map(|owner| owner.login == scope)
        .unwrap_or(false)
}

pub(crate) fn is_public_github_api_host(host: &str) -> bool {
    let value = host.trim_end_matches('/').to_ascii_lowercase();
    value == "https://api.github.com" || value == "api.github.com"
}

pub(crate) fn is_public_github_web_host(host: &str) -> bool {
    let value = host.trim_end_matches('/').to_ascii_lowercase();
    value == "https://github.com" || value == "http://github.com" || value == "github.com"
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn normalize_branch_trims_refs() {
        let value = Some("refs/heads/main".to_string());
        assert_eq!(normalize_branch(value), "main");
    }

    #[test]
    fn parse_scope_requires_single_segment() {
        let scope = ProviderScope::new(vec!["org".to_string(), "project".to_string()]).unwrap();
        let err = parse_scope(&scope).unwrap_err();
        assert!(err.to_string().contains("single org/user segment"));
    }
}

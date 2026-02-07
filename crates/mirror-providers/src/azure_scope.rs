use anyhow::Context;
use mirror_core::model::ProviderScope;
use reqwest::Url;

use crate::provider_paging::string_header;
use reqwest::header::HeaderMap;

pub(crate) fn parse_scope(scope: &ProviderScope) -> anyhow::Result<(&str, Option<&str>)> {
    let segments = scope.segments();
    if segments.len() == 1 {
        return Ok((segments[0].as_str(), None));
    }
    if segments.len() != 2 {
        anyhow::bail!("azure devops scope requires org or org/project segments");
    }
    Ok((segments[0].as_str(), Some(segments[1].as_str())))
}

pub(crate) fn build_repos_url(
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

pub(crate) fn continuation_token(headers: &HeaderMap) -> Option<String> {
    string_header(headers, "x-ms-continuationtoken")
}

#[cfg(test)]
mod tests {
    use super::*;
    use reqwest::header::HeaderValue;

    #[test]
    fn parse_scope_allows_org_only() {
        let scope = ProviderScope::new(vec!["org".into()]).unwrap();
        let (org, project) = parse_scope(&scope).unwrap();
        assert_eq!(org, "org");
        assert!(project.is_none());
    }

    #[test]
    fn parse_scope_allows_org_project() {
        let scope = ProviderScope::new(vec!["org".into(), "proj".into()]).unwrap();
        let (org, project) = parse_scope(&scope).unwrap();
        assert_eq!(org, "org");
        assert_eq!(project, Some("proj"));
    }

    #[test]
    fn continuation_token_reads_header() {
        let mut headers = HeaderMap::new();
        headers.insert(
            "x-ms-continuationtoken",
            HeaderValue::from_static("token-123"),
        );
        let token = continuation_token(&headers);
        assert_eq!(token, Some("token-123".to_string()));
    }
}

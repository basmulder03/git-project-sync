use anyhow::Context;
use reqwest::Client;
use reqwest::StatusCode;

use crate::github_models::RepoItem;
use crate::github_scope::{ScopeKind, repos_url};
use crate::http::send_with_retry_allow_statuses;
use crate::provider_paging::next_page_from_link_header;

pub(crate) async fn fetch_repos_page(
    client: &Client,
    host: &str,
    scope: &str,
    token: &str,
    kind: ScopeKind,
    page: u32,
) -> anyhow::Result<(Vec<RepoItem>, Option<u32>, StatusCode)> {
    let url = repos_url(host, scope, kind, page);
    let builder = client
        .get(url)
        .header("User-Agent", "git-project-sync")
        .bearer_auth(token);
    let response = send_with_retry_allow_statuses(
        || builder.try_clone().context("clone request"),
        &[StatusCode::NOT_FOUND],
    )
    .await
    .context("call GitHub list repos")?;
    let status = response.status();
    if status == StatusCode::NOT_FOUND {
        return Ok((Vec::new(), None, status));
    }
    let response = response
        .error_for_status()
        .context("GitHub list repos status")?;
    let next_page = next_page_from_link_header(response.headers());
    let payload: Vec<RepoItem> = response.json().await.context("decode repos response")?;
    Ok((payload, next_page, status))
}

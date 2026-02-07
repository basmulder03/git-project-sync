use mirror_core::model::ProviderScope;
use reqwest::header::HeaderMap;

use crate::provider_paging::next_page_from_header;

pub(crate) fn parse_scope(scope: &ProviderScope) -> anyhow::Result<String> {
    let segments = scope.segments();
    if segments.is_empty() {
        anyhow::bail!("gitlab scope requires at least one group segment");
    }
    Ok(segments.join("/"))
}

pub(crate) fn normalize_branch(value: Option<String>) -> String {
    value
        .unwrap_or_else(|| "main".to_string())
        .trim_start_matches("refs/heads/")
        .to_string()
}

pub(crate) fn next_page(headers: &HeaderMap) -> Option<u32> {
    next_page_from_header(headers, "x-next-page")
}

#[cfg(test)]
mod tests {
    use super::*;
    use reqwest::header::HeaderValue;

    #[test]
    fn next_page_reads_gitlab_header() {
        let mut headers = HeaderMap::new();
        headers.insert("x-next-page", HeaderValue::from_static("3"));
        assert_eq!(next_page(&headers), Some(3));
    }

    #[test]
    fn normalize_branch_trims_refs() {
        let value = Some("refs/heads/main".to_string());
        assert_eq!(normalize_branch(value), "main");
    }
}

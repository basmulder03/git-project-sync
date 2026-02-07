use serde::Deserialize;

#[derive(Debug, Deserialize)]
pub(crate) struct RepoItem {
    pub(crate) id: u64,
    pub(crate) name: String,
    pub(crate) clone_url: String,
    pub(crate) default_branch: Option<String>,
    pub(crate) archived: Option<bool>,
    pub(crate) owner: Option<RepoOwner>,
}

#[derive(Debug, Deserialize)]
pub(crate) struct RepoOwner {
    pub(crate) login: String,
}

pub(crate) fn parse_scopes_header(value: &str) -> Vec<String> {
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
    use serde_json::json;

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
    fn parse_scopes_header_splits() {
        let scopes = parse_scopes_header("repo, read:org, ");
        assert_eq!(scopes, vec!["repo".to_string(), "read:org".to_string()]);
    }
}

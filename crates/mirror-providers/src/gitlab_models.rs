use serde::Deserialize;

#[derive(Debug, Deserialize)]
pub(crate) struct TokenScopes {
    pub(crate) scopes: Vec<String>,
}

#[derive(Debug, Deserialize)]
pub(crate) struct ProjectItem {
    pub(crate) id: u64,
    pub(crate) name: String,
    pub(crate) http_url_to_repo: String,
    pub(crate) default_branch: Option<String>,
    pub(crate) archived: Option<bool>,
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    #[test]
    fn project_item_deserializes_archived_flag() {
        let value = json!({
            "id": 1,
            "name": "repo",
            "http_url_to_repo": "https://example.com/repo.git",
            "default_branch": "main",
            "archived": true
        });
        let repo: ProjectItem = serde_json::from_value(value).unwrap();
        assert_eq!(repo.archived, Some(true));
    }

    #[test]
    fn token_scopes_deserialize_scopes() {
        let value = json!({
            "id": 1,
            "name": "token",
            "scopes": ["read_api", "read_repository"],
        });
        let token: TokenScopes = serde_json::from_value(value).unwrap();
        assert_eq!(token.scopes, vec!["read_api", "read_repository"]);
    }
}

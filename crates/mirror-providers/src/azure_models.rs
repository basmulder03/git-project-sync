use serde::Deserialize;

#[derive(Debug, Deserialize)]
pub(crate) struct ReposResponse {
    pub(crate) value: Vec<RepoItem>,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
pub(crate) struct RepoItem {
    pub(crate) id: String,
    pub(crate) name: String,
    pub(crate) remote_url: String,
    pub(crate) default_branch: Option<String>,
    pub(crate) is_disabled: Option<bool>,
    pub(crate) project: Option<ProjectRef>,
}

#[derive(Debug, Deserialize)]
pub(crate) struct ProjectRef {
    pub(crate) name: String,
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    #[test]
    fn repo_item_deserializes_archived_flag() {
        let value = json!({
            "id": "1",
            "name": "repo",
            "remoteUrl": "https://example.com/repo.git",
            "defaultBranch": "refs/heads/main",
            "isDisabled": true
        });
        let repo: RepoItem = serde_json::from_value(value).unwrap();
        assert_eq!(repo.is_disabled, Some(true));
    }
}

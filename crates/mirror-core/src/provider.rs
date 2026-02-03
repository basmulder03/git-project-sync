use crate::model::{ProviderKind, ProviderScope, ProviderTarget, RemoteRepo};

pub trait RepoProvider {
    fn kind(&self) -> ProviderKind;
    fn list_repos(&self, target: &ProviderTarget) -> anyhow::Result<Vec<RemoteRepo>>;
    fn validate_auth(&self, target: &ProviderTarget) -> anyhow::Result<()>;

    fn get_repo(
        &self,
        _target: &ProviderTarget,
        _repo_id: &str,
    ) -> anyhow::Result<Option<RemoteRepo>> {
        Ok(None)
    }
}

pub trait ProviderSpec {
    fn kind(&self) -> ProviderKind;
    fn default_host(&self) -> &'static str;
    fn parse_scope(&self, segments: Vec<String>) -> anyhow::Result<ProviderScope>;
    fn validate_scope(&self, scope: &ProviderScope) -> anyhow::Result<()>;
    fn account_key(&self, host: &str, scope: &ProviderScope) -> anyhow::Result<String>;
}

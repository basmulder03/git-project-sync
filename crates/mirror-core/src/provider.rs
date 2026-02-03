use crate::model::{ProviderKind, ProviderTarget, RemoteRepo};

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

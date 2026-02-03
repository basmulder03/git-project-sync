pub mod auth;
pub mod azure_devops;

use anyhow::Result;
use mirror_core::model::{ProviderKind, ProviderTarget, RemoteRepo};

pub trait RepoProvider {
    fn kind(&self) -> ProviderKind;
    fn list_repos(&self, target: &ProviderTarget) -> Result<Vec<RemoteRepo>>;
    fn validate_auth(&self, target: &ProviderTarget) -> Result<()>;

    fn get_repo(&self, _target: &ProviderTarget, _repo_id: &str) -> Result<Option<RemoteRepo>> {
        Ok(None)
    }
}

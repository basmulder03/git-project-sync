use mirror_core::model::ProviderKind;
use mirror_core::provider::ProviderSpec;

use crate::azure_devops::AzureDevOpsProvider;
use crate::github::GitHubProvider;
use crate::gitlab::GitLabProvider;
use crate::spec::{AzureDevOpsSpec, GitHubSpec, GitLabSpec};
use crate::RepoProvider;

pub struct ProviderRegistry;

impl Default for ProviderRegistry {
    fn default() -> Self {
        Self::new()
    }
}

impl ProviderRegistry {
    pub fn new() -> Self {
        Self
    }

    pub fn provider(&self, kind: ProviderKind) -> anyhow::Result<Box<dyn RepoProvider>> {
        match kind {
            ProviderKind::AzureDevOps => Ok(Box::new(AzureDevOpsProvider::new()?)),
            ProviderKind::GitHub => Ok(Box::new(GitHubProvider::new()?)),
            ProviderKind::GitLab => Ok(Box::new(GitLabProvider::new()?)),
        }
    }

    pub fn spec(&self, kind: ProviderKind) -> Box<dyn ProviderSpec> {
        match kind {
            ProviderKind::AzureDevOps => Box::new(AzureDevOpsSpec),
            ProviderKind::GitHub => Box::new(GitHubSpec),
            ProviderKind::GitLab => Box::new(GitLabSpec),
        }
    }
}

use serde::{Deserialize, Serialize};
use std::fmt;

#[derive(Clone, Debug, Eq, PartialEq, Hash, Serialize, Deserialize)]
pub struct ProviderScope {
    segments: Vec<String>,
}

impl ProviderScope {
    pub fn new(segments: Vec<String>) -> anyhow::Result<Self> {
        if segments.is_empty() {
            anyhow::bail!("provider scope must have at least one segment");
        }
        Ok(Self { segments })
    }

    pub fn segments(&self) -> &[String] {
        &self.segments
    }
}

#[derive(Clone, Debug, Eq, PartialEq, Hash, Serialize, Deserialize)]
pub enum ProviderKind {
    AzureDevOps,
    GitHub,
    GitLab,
}

impl ProviderKind {
    pub fn as_prefix(&self) -> &'static str {
        match self {
            ProviderKind::AzureDevOps => "azure-devops",
            ProviderKind::GitHub => "github",
            ProviderKind::GitLab => "gitlab",
        }
    }
}

impl fmt::Display for ProviderKind {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        f.write_str(self.as_prefix())
    }
}

#[derive(Clone, Debug, Eq, PartialEq, Hash, Serialize, Deserialize)]
pub struct ProviderTarget {
    pub provider: ProviderKind,
    pub scope: ProviderScope,
    pub host: Option<String>,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct RemoteRepo {
    pub id: String,
    pub name: String,
    pub clone_url: String,
    pub default_branch: String,
    #[serde(default)]
    pub archived: bool,
    pub provider: ProviderKind,
    pub scope: ProviderScope,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct RepoAuth {
    pub username: String,
    pub token: String,
}

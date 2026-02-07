use mirror_core::model::{ProviderKind, ProviderScope};
use mirror_core::provider::ProviderSpec;

pub struct AzureDevOpsSpec;
pub struct GitHubSpec;
pub struct GitLabSpec;

pub struct PatHelp {
    pub url: &'static str,
    pub scopes: &'static [&'static str],
}

pub fn pat_help(kind: ProviderKind) -> PatHelp {
    match kind {
        ProviderKind::AzureDevOps => PatHelp {
            url: "https://dev.azure.com/<org>/_usersSettings/tokens",
            scopes: &["Code (Read)"],
        },
        ProviderKind::GitHub => PatHelp {
            url: "https://github.com/settings/personal-access-tokens/new",
            scopes: &[
                "Fine-grained PAT",
                "Repository permissions: Contents (Read-only), Metadata (Read-only)",
                "Organization permissions: Members (Read-only) when syncing org repos",
            ],
        },
        ProviderKind::GitLab => PatHelp {
            url: "https://gitlab.com/-/profile/personal_access_tokens",
            scopes: &["read_api", "read_repository"],
        },
    }
}

pub fn spec_for(kind: ProviderKind) -> Box<dyn ProviderSpec> {
    match kind {
        ProviderKind::AzureDevOps => Box::new(AzureDevOpsSpec),
        ProviderKind::GitHub => Box::new(GitHubSpec),
        ProviderKind::GitLab => Box::new(GitLabSpec),
    }
}

impl ProviderSpec for AzureDevOpsSpec {
    fn kind(&self) -> ProviderKind {
        ProviderKind::AzureDevOps
    }

    fn default_host(&self) -> &'static str {
        "https://dev.azure.com"
    }

    fn parse_scope(&self, segments: Vec<String>) -> anyhow::Result<ProviderScope> {
        if segments.len() != 1 && segments.len() != 2 {
            anyhow::bail!("azure devops scope requires org or org/project segments");
        }
        ProviderScope::new(segments)
    }

    fn validate_scope(&self, scope: &ProviderScope) -> anyhow::Result<()> {
        let len = scope.segments().len();
        if len != 1 && len != 2 {
            anyhow::bail!("azure devops scope requires org or org/project segments");
        }
        Ok(())
    }

    fn account_key(&self, host: &str, scope: &ProviderScope) -> anyhow::Result<String> {
        self.validate_scope(scope)?;
        let org = scope.segments()[0].as_str();
        Ok(format!("azdo:{host}:{org}"))
    }
}

impl ProviderSpec for GitHubSpec {
    fn kind(&self) -> ProviderKind {
        ProviderKind::GitHub
    }

    fn default_host(&self) -> &'static str {
        "https://api.github.com"
    }

    fn parse_scope(&self, segments: Vec<String>) -> anyhow::Result<ProviderScope> {
        if segments.len() != 1 {
            anyhow::bail!("github scope requires a single org/user segment");
        }
        ProviderScope::new(segments)
    }

    fn validate_scope(&self, scope: &ProviderScope) -> anyhow::Result<()> {
        if scope.segments().len() != 1 {
            anyhow::bail!("github scope requires a single org/user segment");
        }
        Ok(())
    }

    fn account_key(&self, host: &str, scope: &ProviderScope) -> anyhow::Result<String> {
        self.validate_scope(scope)?;
        let org = scope.segments()[0].as_str();
        Ok(format!("github:{host}:{org}"))
    }
}

impl ProviderSpec for GitLabSpec {
    fn kind(&self) -> ProviderKind {
        ProviderKind::GitLab
    }

    fn default_host(&self) -> &'static str {
        "https://gitlab.com/api/v4"
    }

    fn parse_scope(&self, segments: Vec<String>) -> anyhow::Result<ProviderScope> {
        if segments.is_empty() {
            anyhow::bail!("gitlab scope requires at least one group segment");
        }
        ProviderScope::new(segments)
    }

    fn validate_scope(&self, scope: &ProviderScope) -> anyhow::Result<()> {
        if scope.segments().is_empty() {
            anyhow::bail!("gitlab scope requires at least one group segment");
        }
        Ok(())
    }

    fn account_key(&self, host: &str, scope: &ProviderScope) -> anyhow::Result<String> {
        self.validate_scope(scope)?;
        let group = scope.segments().join("/");
        Ok(format!("gitlab:{host}:{group}"))
    }
}

pub fn host_or_default(host: Option<&str>, spec: &dyn ProviderSpec) -> String {
    host.unwrap_or(spec.default_host())
        .trim_end_matches('/')
        .to_string()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn azure_devops_scope_allows_org_or_project() {
        let spec = AzureDevOpsSpec;
        assert!(spec.parse_scope(vec!["org".into()]).is_ok());
        assert!(spec.parse_scope(vec!["org".into(), "proj".into()]).is_ok());
        assert!(spec.parse_scope(vec![]).is_err());
    }

    #[test]
    fn github_scope_requires_one_segment() {
        let spec = GitHubSpec;
        assert!(spec.parse_scope(vec![]).is_err());
        assert!(spec.parse_scope(vec!["org".into()]).is_ok());
    }

    #[test]
    fn gitlab_scope_requires_at_least_one_segment() {
        let spec = GitLabSpec;
        assert!(spec.parse_scope(vec![]).is_err());
        assert!(spec.parse_scope(vec!["group".into(), "sub".into()]).is_ok());
    }

    #[test]
    fn pat_help_contains_scopes() {
        let github = pat_help(ProviderKind::GitHub);
        assert!(github.url.contains("personal-access-tokens/new"));
        assert!(
            github
                .scopes
                .iter()
                .any(|item| item.contains("Fine-grained"))
        );
        let azdo = pat_help(ProviderKind::AzureDevOps);
        assert!(azdo.url.contains("dev.azure.com"));
    }
}

use mirror_core::model::{ProviderKind, ProviderScope};
use mirror_core::provider::ProviderSpec;

pub struct AzureDevOpsSpec;
pub struct GitHubSpec;
pub struct GitLabSpec;

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
}

use crate::model::{ProviderKind, ProviderScope};
use std::path::{Path, PathBuf};

pub fn repo_path(
    root: &Path,
    provider: &ProviderKind,
    scope: &ProviderScope,
    repo: &str,
) -> PathBuf {
    let mut path = root.to_path_buf();
    path.push(provider.as_prefix());
    for segment in scope.segments() {
        path.push(segment);
    }
    path.push(repo);
    path
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::model::ProviderScope;

    #[test]
    fn maps_azure_devops_path() {
        let scope = ProviderScope::new(vec!["org".into(), "project".into()]).unwrap();
        let path = repo_path(
            Path::new("D:\\root"),
            &ProviderKind::AzureDevOps,
            &scope,
            "repo",
        );
        assert_eq!(
            path,
            PathBuf::from("D:\\root")
                .join("azure-devops")
                .join("org")
                .join("project")
                .join("repo")
        );
    }
}

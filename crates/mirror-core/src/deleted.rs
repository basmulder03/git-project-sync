use crate::cache::{RepoCache, RepoCacheEntry};
use std::collections::HashSet;

#[derive(Debug, Clone, Copy, Eq, PartialEq)]
pub enum MissingRemotePolicy {
    Prompt,
    Archive,
    Remove,
    Skip,
}

#[derive(Debug, Clone, Copy, Eq, PartialEq)]
pub enum DeletedRepoAction {
    Archive,
    Remove,
    Skip,
}

pub struct DeletedRepo<'a> {
    pub repo_id: &'a str,
    pub entry: &'a RepoCacheEntry,
}

pub fn detect_deleted_repos<'a>(
    cache: &'a RepoCache,
    current_repo_ids: &HashSet<String>,
) -> Vec<DeletedRepo<'a>> {
    cache
        .repos
        .iter()
        .filter_map(|(repo_id, entry)| {
            if current_repo_ids.contains(repo_id) {
                None
            } else {
                Some(DeletedRepo {
                    repo_id,
                    entry,
                })
            }
        })
        .collect()
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::cache::{RepoCache, RepoCacheEntry};
    use crate::model::{ProviderKind, ProviderScope};
    use std::collections::HashSet;

    #[test]
    fn detects_missing_repo_ids() {
        let mut cache = RepoCache::default();
        cache.repos.insert(
            "repo-1".into(),
            RepoCacheEntry {
                name: "Repo One".into(),
                provider: ProviderKind::AzureDevOps,
                scope: ProviderScope::new(vec!["org".into(), "proj".into()]).unwrap(),
                path: "D:\\root\\azure-devops\\org\\proj\\Repo One".into(),
            },
        );
        cache.repos.insert(
            "repo-2".into(),
            RepoCacheEntry {
                name: "Repo Two".into(),
                provider: ProviderKind::AzureDevOps,
                scope: ProviderScope::new(vec!["org".into(), "proj".into()]).unwrap(),
                path: "D:\\root\\azure-devops\\org\\proj\\Repo Two".into(),
            },
        );

        let current: HashSet<String> = ["repo-2".to_string()].into_iter().collect();
        let missing = detect_deleted_repos(&cache, &current);
        assert_eq!(missing.len(), 1);
        assert_eq!(missing[0].entry.name, "Repo One");
    }
}

use crate::cache::RepoCache;
use crate::model::RemoteRepo;
use crate::paths::repo_path;
use crate::sync_engine_fs::move_repo_path;
use crate::sync_engine_types::RepoWorkItem;
use std::path::{Path, PathBuf};
use tracing::{info, warn};

pub(crate) fn build_work_items(
    cache: &RepoCache,
    root: &Path,
    repos: Vec<RemoteRepo>,
) -> Vec<RepoWorkItem> {
    let mut work_items: Vec<RepoWorkItem> = Vec::new();
    for repo in repos {
        let path = repo_path(root, &repo.provider, &repo.scope, &repo.name);
        if let Some(entry) = cache.repos.get(&repo.id) {
            let cached_path = PathBuf::from(&entry.path);
            if cached_path != path {
                if cached_path.exists() && !path.exists() {
                    if let Err(err) = move_repo_path(&cached_path, &path) {
                        warn!(
                            repo_id = %repo.id,
                            from = %cached_path.display(),
                            to = %path.display(),
                            error = %err,
                            "failed to move repo after rename"
                        );
                    } else {
                        info!(
                            repo_id = %repo.id,
                            from = %cached_path.display(),
                            to = %path.display(),
                            "moved repo to match rename"
                        );
                    }
                } else if !cached_path.exists() {
                    info!(
                        repo_id = %repo.id,
                        from = %cached_path.display(),
                        to = %path.display(),
                        "cached repo path missing; updating to new path"
                    );
                }
            }
        }
        work_items.push(RepoWorkItem { repo, path });
    }
    work_items
}

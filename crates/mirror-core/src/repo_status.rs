use git2::{BranchType, Repository};
use serde::{Deserialize, Serialize};
use std::path::Path;
use std::time::{SystemTime, UNIX_EPOCH};

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Default)]
pub struct RepoLocalStatus {
    pub checked_at: u64,
    pub head_branch: Option<String>,
    pub head_commit_time: Option<u64>,
    pub upstream: Option<String>,
    pub ahead: Option<u32>,
    pub behind: Option<u32>,
}

pub fn compute_repo_status(path: &Path) -> anyhow::Result<RepoLocalStatus> {
    let now = current_timestamp_secs();
    let repo = match Repository::open(path) {
        Ok(repo) => repo,
        Err(_) => {
            return Ok(RepoLocalStatus {
                checked_at: now,
                ..RepoLocalStatus::default()
            });
        }
    };

    let mut status = RepoLocalStatus {
        checked_at: now,
        ..RepoLocalStatus::default()
    };

    let head = match repo.head() {
        Ok(head) => head,
        Err(_) => return Ok(status),
    };

    if head.is_branch() {
        status.head_branch = head.shorthand().map(|name| name.to_string());
    }

    if let Some(oid) = head.target()
        && let Ok(commit) = repo.find_commit(oid)
    {
        let seconds = commit.time().seconds();
        if seconds >= 0 {
            status.head_commit_time = Some(seconds as u64);
        }
    }

    if let Some(branch_name) = status.head_branch.as_deref()
        && let Ok(branch) = repo.find_branch(branch_name, BranchType::Local)
        && let Ok(upstream) = branch.upstream()
    {
        if let Some(name) = upstream.get().name() {
            status.upstream = Some(name.to_string());
        }
        if let (Some(local_oid), Some(upstream_oid)) = (head.target(), upstream.get().target()) {
            let (ahead, behind) = repo.graph_ahead_behind(local_oid, upstream_oid)?;
            status.ahead = Some(ahead as u32);
            status.behind = Some(behind as u32);
        }
    }

    Ok(status)
}

fn current_timestamp_secs() -> u64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap_or_default()
        .as_secs()
}

#[cfg(test)]
mod tests {
    use super::*;
    use git2::{Commit, Oid, Signature};
    use tempfile::TempDir;

    fn commit_file(repo: &Repository, name: &str, contents: &str, parents: &[&Commit<'_>]) -> Oid {
        let repo_dir = repo.workdir().unwrap();
        std::fs::write(repo_dir.join(name), contents).unwrap();
        let mut index = repo.index().unwrap();
        index.add_path(Path::new(name)).unwrap();
        let tree_id = index.write_tree().unwrap();
        let tree = repo.find_tree(tree_id).unwrap();
        let sig = Signature::now("tester", "tester@example.com").unwrap();
        repo.commit(Some("HEAD"), &sig, &sig, "commit", &tree, parents)
            .unwrap()
    }

    #[test]
    fn compute_repo_status_reports_branch_and_ahead() {
        let temp = TempDir::new().unwrap();
        let repo = Repository::init(temp.path()).unwrap();

        let first = commit_file(&repo, "a.txt", "a", &[]);
        let first_commit = repo.find_commit(first).unwrap();
        repo.branch("main", &first_commit, true).unwrap();
        repo.set_head("refs/heads/main").unwrap();

        repo.remote("origin", "https://example.com/repo.git")
            .unwrap();
        repo.reference("refs/remotes/origin/main", first, true, "origin main")
            .unwrap();

        let mut branch = repo.find_branch("main", git2::BranchType::Local).unwrap();
        branch.set_upstream(Some("origin/main")).unwrap();

        let status = compute_repo_status(temp.path()).unwrap();
        assert_eq!(status.head_branch.as_deref(), Some("main"));
        assert_eq!(status.ahead, Some(0));
        assert_eq!(status.behind, Some(0));
        assert!(status.head_commit_time.is_some());

        let _ = commit_file(&repo, "b.txt", "b", &[&first_commit]);
        let status = compute_repo_status(temp.path()).unwrap();
        assert_eq!(status.ahead, Some(1));
        assert_eq!(status.behind, Some(0));
    }
}

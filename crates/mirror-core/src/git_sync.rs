use crate::model::RepoAuth;
use anyhow::Context;
use git2::{
    Cred, FetchOptions, Oid, RemoteCallbacks, Repository, StatusOptions,
    build::{CheckoutBuilder, RepoBuilder},
};
use std::path::Path;
use tracing::{info, warn};

#[derive(Debug, Clone, Copy, Eq, PartialEq)]
pub enum SyncOutcome {
    Cloned,
    FastForwarded,
    UpToDate,
    Dirty,
    Diverged,
}

pub fn sync_repo(
    repo_path: &Path,
    remote_url: &str,
    default_branch: &str,
    auth: Option<&RepoAuth>,
    verify: bool,
) -> anyhow::Result<SyncOutcome> {
    if !repo_path.exists() {
        clone_repo(repo_path, remote_url, auth)?;
        return Ok(SyncOutcome::Cloned);
    }

    let repo = Repository::open(repo_path).context("open repo")?;
    if !is_working_tree_clean(&repo)? {
        warn!(path = %repo_path.display(), "working tree dirty; skipping sync");
        return Ok(SyncOutcome::Dirty);
    }

    ensure_origin_remote(&repo, remote_url)?;
    fetch_origin(&repo, auth)?;
    if verify {
        let summary = verify_refs(&repo, default_branch)?;
        if summary.default_mismatch {
            warn!(
                default_branch = %default_branch,
                "default branch does not match remote HEAD"
            );
        }
        if !summary.mismatched_branches.is_empty() {
            warn!(
                count = summary.mismatched_branches.len(),
                branches = ?summary.mismatched_branches,
                "branch refs do not match their upstreams"
            );
        }
    }
    detect_orphaned_branches(&repo)?;
    fast_forward_default_branch(&repo, default_branch)
}

fn clone_repo(repo_path: &Path, remote_url: &str, auth: Option<&RepoAuth>) -> anyhow::Result<()> {
    let mut fo = FetchOptions::new();
    fo.remote_callbacks(remote_callbacks(auth));
    info!(path = %repo_path.display(), "cloning repo");
    let mut builder = RepoBuilder::new();
    builder.fetch_options(fo);
    builder.clone(remote_url, repo_path).context("clone repo")?;
    Ok(())
}

fn is_working_tree_clean(repo: &Repository) -> anyhow::Result<bool> {
    let mut options = StatusOptions::new();
    options
        .include_untracked(true)
        .recurse_untracked_dirs(true)
        .include_ignored(false);
    let statuses = repo.statuses(Some(&mut options)).context("status repo")?;
    Ok(statuses.is_empty())
}

fn ensure_origin_remote(repo: &Repository, remote_url: &str) -> anyhow::Result<()> {
    match repo.find_remote("origin") {
        Ok(remote) => {
            let current = remote.url().unwrap_or_default();
            if current != remote_url {
                repo.remote_set_url("origin", remote_url)
                    .context("update origin remote url")?;
            }
        }
        Err(_) => {
            repo.remote("origin", remote_url)
                .context("create origin remote")?;
        }
    }
    Ok(())
}

fn fetch_origin(repo: &Repository, auth: Option<&RepoAuth>) -> anyhow::Result<()> {
    let mut remote = repo.find_remote("origin").context("find origin remote")?;
    let mut fo = FetchOptions::new();
    fo.remote_callbacks(remote_callbacks(auth));
    info!("fetching origin");
    remote
        .fetch(&[] as &[&str], Some(&mut fo), None)
        .context("fetch origin")?;
    Ok(())
}

fn fast_forward_default_branch(
    repo: &Repository,
    default_branch: &str,
) -> anyhow::Result<SyncOutcome> {
    let local_ref = format!("refs/heads/{default_branch}");
    let remote_ref = format!("refs/remotes/origin/{default_branch}");

    let remote_oid = match repo.refname_to_id(&remote_ref) {
        Ok(oid) => oid,
        Err(_) => {
            warn!(remote_ref = %remote_ref, "default branch missing on remote; skipping");
            return Ok(SyncOutcome::Diverged);
        }
    };

    let local_oid = match repo.refname_to_id(&local_ref) {
        Ok(oid) => oid,
        Err(_) => {
            warn!(
                default_branch = %default_branch,
                "default branch missing locally; creating local branch"
            );
            create_local_branch(repo, default_branch, remote_oid)?;
            return Ok(SyncOutcome::FastForwarded);
        }
    };

    let (ahead, behind) = repo
        .graph_ahead_behind(local_oid, remote_oid)
        .context("compare local and remote")?;

    if ahead > 0 && behind > 0 {
        warn!("default branch diverged; skipping");
        return Ok(SyncOutcome::Diverged);
    }
    if ahead > 0 && behind == 0 {
        warn!("default branch ahead of origin; skipping");
        return Ok(SyncOutcome::Diverged);
    }
    if behind == 0 {
        return Ok(SyncOutcome::UpToDate);
    }

    update_branch_ref(repo, &local_ref, remote_oid)?;
    if is_head_on_default(repo, default_branch)? {
        checkout_head(repo)?;
    } else if let Ok(head) = repo.head()
        && head.is_branch()
            && let Some(name) = head.shorthand()
                && name != default_branch {
                    warn!(
                        current = %name,
                        new_default = %default_branch,
                        "default branch differs from current HEAD"
                    );
                }
    Ok(SyncOutcome::FastForwarded)
}

fn detect_orphaned_branches(repo: &Repository) -> anyhow::Result<Vec<String>> {
    let mut orphaned = Vec::new();
    let branches = repo.branches(Some(git2::BranchType::Local))?;
    for branch in branches {
        let (branch, _) = branch?;
        let name = branch.name()?.unwrap_or("").to_string();
        if name.is_empty() {
            continue;
        }
        if let Some(upstream_name) = upstream_ref_from_config(repo, &name)?
            && repo.find_reference(&upstream_name).is_err() {
                warn!(
                    branch = %name,
                    upstream = %upstream_name,
                    "local branch upstream missing on remote"
                );
                orphaned.push(name);
            }
    }
    Ok(orphaned)
}

#[derive(Debug, Default)]
struct VerifySummary {
    default_mismatch: bool,
    mismatched_branches: Vec<String>,
}

fn verify_refs(repo: &Repository, default_branch: &str) -> anyhow::Result<VerifySummary> {
    let mut summary = VerifySummary::default();
    let local_ref = format!("refs/heads/{default_branch}");
    let remote_ref = format!("refs/remotes/origin/{default_branch}");
    if let (Ok(local_oid), Ok(remote_oid)) =
        (repo.refname_to_id(&local_ref), repo.refname_to_id(&remote_ref))
        && local_oid != remote_oid {
            summary.default_mismatch = true;
        }

    let branches = repo.branches(Some(git2::BranchType::Local))?;
    for branch in branches {
        let (branch, _) = branch?;
        let name = branch.name()?.unwrap_or("").to_string();
        if name.is_empty() || name == default_branch {
            continue;
        }
        if let Some(upstream_name) = upstream_ref_from_config(repo, &name)?
            && let (Ok(local_oid), Ok(upstream_oid)) =
                (repo.refname_to_id(&format!("refs/heads/{name}")), repo.refname_to_id(&upstream_name))
                && local_oid != upstream_oid {
                    summary.mismatched_branches.push(name);
                }
    }
    Ok(summary)
}

fn upstream_ref_from_config(repo: &Repository, branch_name: &str) -> anyhow::Result<Option<String>> {
    let config = repo.config().context("open repo config")?;
    let remote_key = format!("branch.{branch_name}.remote");
    let merge_key = format!("branch.{branch_name}.merge");
    let remote = match config.get_string(&remote_key) {
        Ok(value) => value,
        Err(_) => return Ok(None),
    };
    let merge = match config.get_string(&merge_key) {
        Ok(value) => value,
        Err(_) => return Ok(None),
    };
    if remote == "." {
        return Ok(Some(merge));
    }
    let merged_branch = merge
        .strip_prefix("refs/heads/")
        .unwrap_or(merge.as_str());
    Ok(Some(format!("refs/remotes/{remote}/{merged_branch}")))
}

fn create_local_branch(repo: &Repository, default_branch: &str, target: Oid) -> anyhow::Result<()> {
    let commit = repo.find_commit(target).context("find remote commit")?;
    repo.branch(default_branch, &commit, false)
        .context("create local branch")?;
    Ok(())
}

fn update_branch_ref(repo: &Repository, local_ref: &str, target: Oid) -> anyhow::Result<()> {
    let mut reference = repo
        .find_reference(local_ref)
        .with_context(|| format!("find local ref {local_ref}"))?;
    reference
        .set_target(target, "fast-forward")
        .context("set local ref target")?;
    Ok(())
}

fn is_head_on_default(repo: &Repository, default_branch: &str) -> anyhow::Result<bool> {
    let head = match repo.head() {
        Ok(head) => head,
        Err(_) => return Ok(false),
    };
    Ok(head.is_branch() && head.shorthand() == Some(default_branch))
}

fn checkout_head(repo: &Repository) -> anyhow::Result<()> {
    let mut checkout = CheckoutBuilder::new();
    checkout.safe();
    repo.checkout_head(Some(&mut checkout))
        .context("checkout head")?;
    Ok(())
}

fn remote_callbacks(auth: Option<&RepoAuth>) -> RemoteCallbacks<'static> {
    let auth = auth.cloned();
    let mut callbacks = RemoteCallbacks::new();
    callbacks.credentials(move |_url, username_from_url, _allowed| {
        if let Some(auth) = auth.as_ref() {
            let username = if auth.username.is_empty() {
                username_from_url.unwrap_or("pat")
            } else {
                auth.username.as_str()
            };
            Cred::userpass_plaintext(username, &auth.token)
        } else {
            Cred::default()
        }
    });
    callbacks
}

#[cfg(test)]
mod tests {
    use super::*;
    use git2::{Commit, Repository, Signature};
    use std::path::Path;
    use tempfile::TempDir;

    #[test]
    fn clean_repo_detects_dirty() {
        let tmp = TempDir::new().unwrap();
        let repo = Repository::init(tmp.path()).unwrap();
        assert!(is_working_tree_clean(&repo).unwrap());

        std::fs::write(tmp.path().join("file.txt"), "data").unwrap();
        assert!(!is_working_tree_clean(&repo).unwrap());
    }

    #[test]
    fn missing_remote_ref_is_skipped() {
        let tmp = TempDir::new().unwrap();
        let repo = Repository::init(tmp.path()).unwrap();
        let outcome = fast_forward_default_branch(&repo, "main").unwrap();
        assert_eq!(outcome, SyncOutcome::Diverged);
    }

    #[test]
    fn diverged_default_branch_returns_diverged() {
        let tmp = TempDir::new().unwrap();
        let repo = Repository::init(tmp.path()).unwrap();

        let base = commit_file(&repo, "base.txt", "base", &[], Some("HEAD"));
        let base_commit = repo.find_commit(base).unwrap();

        let local = commit_file(&repo, "local.txt", "local", &[&base_commit], Some("HEAD"));
        let remote = commit_file(
            &repo,
            "remote.txt",
            "remote",
            &[&base_commit],
            Some("refs/remotes/origin/main"),
        );

        repo.reference("refs/heads/main", local, true, "local main")
            .unwrap();
        repo.reference(
            "refs/remotes/origin/main",
            remote,
            true,
            "remote main",
        )
        .unwrap();

        let outcome = fast_forward_default_branch(&repo, "main").unwrap();
        assert_eq!(outcome, SyncOutcome::Diverged);
    }

    #[test]
    fn ensure_origin_updates_url() {
        let tmp = TempDir::new().unwrap();
        let repo = Repository::init(tmp.path()).unwrap();
        repo.remote("origin", "https://example.com/old.git").unwrap();
        ensure_origin_remote(&repo, "https://example.com/new.git").unwrap();
        let remote = repo.find_remote("origin").unwrap();
        assert_eq!(remote.url(), Some("https://example.com/new.git"));
    }

    #[test]
    fn default_branch_change_creates_new_branch() {
        let tmp = TempDir::new().unwrap();
        let repo = Repository::init(tmp.path()).unwrap();

        let base = commit_file(&repo, "base.txt", "base", &[], Some("refs/heads/main"));
        repo.set_head("refs/heads/main").unwrap();
        let base_commit = repo.find_commit(base).unwrap();

        let remote = commit_file(
            &repo,
            "remote.txt",
            "remote",
            &[&base_commit],
            Some("refs/remotes/origin/develop"),
        );
        repo.reference(
            "refs/remotes/origin/develop",
            remote,
            true,
            "remote develop",
        )
        .unwrap();

        let outcome = fast_forward_default_branch(&repo, "develop").unwrap();
        assert_eq!(outcome, SyncOutcome::FastForwarded);
        assert!(repo.find_reference("refs/heads/develop").is_ok());
        let head = repo.head().unwrap();
        assert_eq!(head.shorthand(), Some("main"));
    }

    #[test]
    fn detects_orphaned_local_branch() {
        let tmp = TempDir::new().unwrap();
        let repo = Repository::init(tmp.path()).unwrap();
        repo.remote("origin", "https://example.com/repo.git").unwrap();

        let base = commit_file(&repo, "base.txt", "base", &[], Some("refs/heads/main"));
        repo.set_head("refs/heads/main").unwrap();
        let base_commit = repo.find_commit(base).unwrap();
        repo.reference(
            "refs/remotes/origin/main",
            base_commit.id(),
            true,
            "remote main",
        )
        .unwrap();

        let mut main = repo.find_branch("main", git2::BranchType::Local).unwrap();
        main.set_upstream(Some("origin/main")).unwrap();

        repo.find_reference("refs/remotes/origin/main").unwrap().delete().unwrap();
        let orphaned = detect_orphaned_branches(&repo).unwrap();
        assert_eq!(orphaned, vec!["main".to_string()]);
    }

    #[test]
    fn verify_detects_default_mismatch() {
        let tmp = TempDir::new().unwrap();
        let repo = Repository::init(tmp.path()).unwrap();
        repo.remote("origin", "https://example.com/repo.git").unwrap();

        let base = commit_file(&repo, "base.txt", "base", &[], Some("refs/heads/main"));
        repo.reference(
            "refs/remotes/origin/main",
            base,
            true,
            "remote main",
        )
        .unwrap();
        let base_commit = repo.find_commit(base).unwrap();
        let local = commit_file(
            &repo,
            "local.txt",
            "local",
            &[&base_commit],
            Some("refs/heads/main"),
        );
        repo.reference("refs/heads/main", local, true, "local main").unwrap();

        let summary = verify_refs(&repo, "main").unwrap();
        assert!(summary.default_mismatch);
    }

    #[test]
    fn verify_detects_branch_mismatch() {
        let tmp = TempDir::new().unwrap();
        let repo = Repository::init(tmp.path()).unwrap();
        repo.remote("origin", "https://example.com/repo.git").unwrap();

        let base = commit_file(&repo, "base.txt", "base", &[], Some("refs/heads/main"));
        repo.reference(
            "refs/remotes/origin/main",
            base,
            true,
            "remote main",
        )
        .unwrap();
        repo.reference("refs/heads/main", base, true, "local main").unwrap();

        let feature_local = commit_file(&repo, "feat.txt", "feat", &[], Some("refs/heads/feature"));
        let feature_remote = commit_file(
            &repo,
            "feat-remote.txt",
            "feat-remote",
            &[],
            Some("refs/remotes/origin/feature"),
        );
        repo.reference("refs/heads/feature", feature_local, true, "local feature")
            .unwrap();
        repo.reference(
            "refs/remotes/origin/feature",
            feature_remote,
            true,
            "remote feature",
        )
        .unwrap();
        let mut feature = repo.find_branch("feature", git2::BranchType::Local).unwrap();
        feature.set_upstream(Some("origin/feature")).unwrap();

        let summary = verify_refs(&repo, "main").unwrap();
        assert_eq!(summary.mismatched_branches, vec!["feature".to_string()]);
    }

    fn commit_file(
        repo: &Repository,
        name: &str,
        contents: &str,
        parents: &[&Commit<'_>],
        update_ref: Option<&str>,
    ) -> Oid {
        let workdir = repo.workdir().unwrap();
        std::fs::write(workdir.join(name), contents).unwrap();
        let mut index = repo.index().unwrap();
        index.add_path(Path::new(name)).unwrap();
        let tree_id = index.write_tree().unwrap();
        let tree = repo.find_tree(tree_id).unwrap();
        let sig = Signature::now("tester", "tester@example.com").unwrap();
        repo.commit(update_ref, &sig, &sig, "commit", &tree, parents)
            .unwrap()
    }
}

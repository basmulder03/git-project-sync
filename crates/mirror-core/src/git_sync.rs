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

    fetch_origin(&repo, remote_url, auth)?;
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

fn fetch_origin(
    repo: &Repository,
    remote_url: &str,
    auth: Option<&RepoAuth>,
) -> anyhow::Result<()> {
    let mut remote = match repo.find_remote("origin") {
        Ok(remote) => remote,
        Err(_) => repo
            .remote("origin", remote_url)
            .context("create origin remote")?,
    };
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

    let remote_oid = repo
        .refname_to_id(&remote_ref)
        .with_context(|| format!("missing remote ref {remote_ref}"))?;

    let local_oid = match repo.refname_to_id(&local_ref) {
        Ok(oid) => oid,
        Err(_) => {
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
    }
    Ok(SyncOutcome::FastForwarded)
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
    fn missing_remote_ref_is_error() {
        let tmp = TempDir::new().unwrap();
        let repo = Repository::init(tmp.path()).unwrap();
        let err = fast_forward_default_branch(&repo, "main").unwrap_err();
        assert!(err.to_string().contains("missing remote ref"));
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

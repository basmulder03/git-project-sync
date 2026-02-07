use super::*;
use std::fs;
use tempfile::TempDir;

#[test]
fn resolve_repo_prefers_override() {
    unsafe {
        std::env::set_var(UPDATE_REPO_ENV, "env/repo");
    }
    let repo = repo_check::resolve_repo(Some("override/repo"));
    assert_eq!(repo, "override/repo");
    unsafe {
        std::env::remove_var(UPDATE_REPO_ENV);
    }
}

#[test]
fn parse_version_strips_v() {
    let parsed = repo_check::parse_version("v1.2.3").unwrap();
    assert_eq!(parsed.to_string(), "1.2.3");
}

#[test]
fn choose_restart_path_uses_installed_path() {
    let temp = TempDir::new().unwrap();
    let installed = temp.path().join("mirror-cli");
    fs::write(&installed, b"binary").unwrap();
    let current = temp.path().join("current");
    let chosen = restart::choose_restart_path(Some(installed.clone()), current);
    assert_eq!(chosen, installed);
}

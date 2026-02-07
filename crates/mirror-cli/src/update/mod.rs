use anyhow::Context;
use directories::ProjectDirs;
use reqwest::blocking::Client;
use semver::Version;
use serde::Deserialize;
use serde_json::json;
use std::fs::{self, File};
use std::io::Write;
use std::path::{Path, PathBuf};
use std::process::Command;

use mirror_core::audit::{AuditLogger, AuditStatus};
use mirror_core::cache::{RepoCache, record_update_check, update_check_due};

use crate::install::{InstallOptions, PathChoice};

mod apply;
mod auto;
mod errors;
mod repo_check;
mod restart;
#[cfg(test)]
mod tests;

pub use apply::{apply_update, apply_update_with_progress};
pub use auto::check_and_maybe_apply;
pub use errors::{is_network_error, is_permission_error};
pub use repo_check::check_for_update;
pub use restart::restart_current_process;

pub const DEFAULT_UPDATE_REPO: &str = "basmulder03/git-project-sync";
pub const UPDATE_REPO_ENV: &str = "GIT_PROJECT_SYNC_UPDATE_REPO";

#[derive(Clone, Debug)]
pub struct ReleaseAsset {
    pub name: String,
    pub url: String,
}

#[derive(Clone, Debug)]
pub struct UpdateCheck {
    pub current: Version,
    pub latest: Version,
    pub release_url: Option<String>,
    pub is_newer: bool,
    pub asset: Option<ReleaseAsset>,
}

pub struct AutoUpdateOptions<'a> {
    pub cache_path: &'a Path,
    pub interval_secs: u64,
    pub auto_apply: bool,
    pub audit: &'a AuditLogger,
    pub force: bool,
    pub interactive: bool,
    pub source: &'a str,
    pub override_repo: Option<&'a str>,
}

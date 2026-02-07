use crate::install::{InstallOptions, PathChoice};
use crate::{install, logging, repo_overview, token_check, tui, update};
use anyhow::Context;
use clap::{CommandFactory, Parser, ValueEnum};
use mirror_core::audit::{AuditContext, AuditLogger, AuditStatus};
use mirror_core::cache::{
    RepoCache, RepoCacheEntry, backoff_until, update_check_due, update_target_failure,
    update_target_success,
};
use mirror_core::config::{
    AppConfigV2, TargetConfig, default_cache_path, default_config_path, default_lock_path,
    load_or_migrate, target_id,
};
use mirror_core::deleted::{DeletedRepoAction, MissingRemotePolicy};
#[cfg(test)]
use mirror_core::model::ProviderScope;
use mirror_core::model::{ProviderKind, ProviderTarget};
use mirror_core::scheduler::{bucket_for_repo_id, current_day_bucket};
use mirror_core::sync_engine::{
    RunSyncOptions, SyncAction, SyncProgress, SyncSummary, run_sync_filtered,
};
use mirror_providers::ProviderRegistry;
use mirror_providers::auth;
use mirror_providers::spec::{host_or_default, spec_for};
use reqwest::StatusCode;
use std::cell::Cell;
use std::io::IsTerminal;
use std::io::{self, Write};
use std::path::{Path, PathBuf};
use std::process::Command;
use tracing::{info, warn};
use tracing_subscriber::EnvFilter;
use tracing_subscriber::prelude::*;

mod app;
mod args;
mod config_cmd;
mod daemon_cmd;
mod misc_cmd;
mod shared;
mod sync_cmd;
mod target_cmd;
#[cfg(test)]
mod tests;
mod token_cmd;

use args::*;

use config_cmd::handle_config;
use daemon_cmd::handle_daemon;
use misc_cmd::{
    handle_cache, handle_health, handle_install, handle_service, handle_task, handle_update,
    handle_webhook, run_update_check,
};
use shared::{
    current_epoch_seconds, maybe_escalate_and_reexec, run_token_validity_checks,
    should_run_cli_token_check, should_run_cli_update_check, stdin_is_tty, stdout_is_tty,
};
use sync_cmd::handle_sync;
use target_cmd::handle_target;
use token_cmd::handle_token;

pub async fn run() -> anyhow::Result<()> {
    app::run().await
}

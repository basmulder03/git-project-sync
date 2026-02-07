use crate::logging::LogBuffer;
use crate::repo_overview;
use crate::update;
use anyhow::Context;
use crossterm::{
    event::{self, Event, KeyCode, KeyEvent, KeyEventKind, KeyModifiers},
    execute,
    terminal::{EnterAlternateScreen, LeaveAlternateScreen, disable_raw_mode, enable_raw_mode},
};
use mirror_core::audit::{AuditContext, AuditLogger, AuditStatus};
use mirror_core::cache::{RepoCache, SyncSummarySnapshot};
use mirror_core::config::{
    AppConfigV2, TargetConfig, default_cache_path, default_config_path, default_lock_path,
    load_or_migrate, target_id,
};
use mirror_core::deleted::MissingRemotePolicy;
use mirror_core::lockfile::LockFile;
use mirror_core::model::ProviderKind;
use mirror_core::model::ProviderTarget;
use mirror_core::model::RemoteRepo;
use mirror_core::repo_status::RepoLocalStatus;
use mirror_core::sync_engine::{RunSyncOptions, SyncSummary, run_sync_filtered};
use mirror_providers::ProviderRegistry;
use mirror_providers::auth;
use mirror_providers::spec::{host_or_default, pat_help, spec_for};
use ratatui::{
    Terminal,
    backend::CrosstermBackend,
    layout::{Constraint, Direction, Layout},
    style::{Modifier, Style},
    text::{Line, Span},
    widgets::{Block, Borders, List, ListItem, Paragraph, Wrap},
};
use semver::Version;
use std::collections::{HashMap, HashSet};
use std::future::Future;
use std::io::{self, Stdout};
use std::sync::mpsc;
use std::thread;
use std::time::{Duration, Instant};
use tracing::{debug, error, info, warn};

#[derive(Clone, Copy, Debug)]
pub enum StartView {
    Main,
    Dashboard,
    Install,
}

enum RunOutcome {
    Exit,
    Restart,
}

const REPO_STATUS_TTL_SECS: u64 = 600;
const LOG_PANEL_HEIGHT: u16 = 7;
const LOG_PANEL_BORDER_HEIGHT: u16 = 2;
const LOG_HEADER_LINES: usize = 1;

fn tui_block_on<F: Future>(future: F) -> F::Output {
    thread_local! {
        static RUNTIME: std::cell::RefCell<Option<tokio::runtime::Runtime>> = const { std::cell::RefCell::new(None) };
    }

    RUNTIME.with(|cell| {
        let mut runtime = cell.borrow_mut();
        if runtime.is_none() {
            let created = tokio::runtime::Builder::new_current_thread()
                .enable_time()
                .build()
                .expect("create tui runtime");
            *runtime = Some(created);
        }
        runtime
            .as_mut()
            .expect("tui runtime initialized")
            .block_on(future)
    })
}

pub fn run_tui(
    audit: &AuditLogger,
    log_buffer: LogBuffer,
    start_view: StartView,
) -> anyhow::Result<()> {
    enable_raw_mode().context("enable raw mode")?;
    let mut stdout = io::stdout();
    execute!(stdout, EnterAlternateScreen).context("enter alternate screen")?;
    let backend = CrosstermBackend::new(stdout);
    let mut terminal = Terminal::new(backend).context("create terminal")?;

    info!(start_view = ?start_view, "Starting TUI");
    let _ = audit.record("tui.start", AuditStatus::Ok, Some("tui"), None, None)?;
    let result = run_app(&mut terminal, audit, log_buffer, start_view);

    disable_raw_mode().ok();
    execute!(terminal.backend_mut(), LeaveAlternateScreen).ok();
    terminal.show_cursor().ok();

    let outcome = match result {
        Ok(outcome) => {
            let _ = audit.record("tui.exit", AuditStatus::Ok, Some("tui"), None, None);
            Some(outcome)
        }
        Err(err) => {
            error!(error = %err, "TUI exited with error");
            return Err(err);
        }
    };

    if matches!(outcome, Some(RunOutcome::Restart)) {
        update::restart_current_process().context("restart after update apply")?;
    }

    Ok(())
}

fn run_app(
    terminal: &mut Terminal<CrosstermBackend<Stdout>>,
    audit: &AuditLogger,
    log_buffer: LogBuffer,
    start_view: StartView,
) -> anyhow::Result<RunOutcome> {
    let mut app = TuiApp::load(audit.clone(), log_buffer, start_view)?;
    let mut last_tick = Instant::now();
    let tick_rate = Duration::from_millis(200);
    debug!(
        tick_rate_ms = tick_rate.as_millis(),
        "TUI event loop started"
    );

    loop {
        terminal.draw(|frame| app.draw(frame))?;

        let timeout = tick_rate
            .checked_sub(last_tick.elapsed())
            .unwrap_or_else(|| Duration::from_secs(0));

        if event::poll(timeout)?
            && let Event::Key(key) = event::read()?
            && app.handle_key(key)?
        {
            return Ok(RunOutcome::Exit);
        }

        if last_tick.elapsed() >= tick_rate {
            last_tick = Instant::now();
        }

        app.poll_install_events()?;
        app.poll_repo_status_events()?;
        app.poll_sync_events()?;
        app.poll_update_events()?;
        if app.restart_requested {
            return Ok(RunOutcome::Restart);
        }
    }
}

#[derive(Clone, Copy, Debug, PartialEq, Eq, Hash)]
enum View {
    Main,
    Dashboard,
    Install,
    UpdatePrompt,
    UpdateProgress,
    SyncStatus,
    InstallStatus,
    ConfigRoot,
    RepoOverview,
    Targets,
    TargetAdd,
    TargetRemove,
    TokenMenu,
    TokenList,
    TokenSet,
    TokenValidate,
    Service,
    AuditLog,
    Message,
    InstallProgress,
}

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
enum InstallAction {
    Install,
    Update,
    Reinstall,
}

impl InstallAction {
    fn label(self) -> &'static str {
        match self {
            InstallAction::Install => "Install new",
            InstallAction::Update => "Update installed version",
            InstallAction::Reinstall => "Reinstall current version",
        }
    }

    fn verb(self) -> &'static str {
        match self {
            InstallAction::Install => "install",
            InstallAction::Update => "update install",
            InstallAction::Reinstall => "reinstall",
        }
    }
}

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
enum InstallState {
    NotInstalled,
    UpdateReady,
    Installed,
    Unknown,
}

impl InstallState {
    fn menu_label(self) -> &'static str {
        match self {
            InstallState::NotInstalled => "Setup (not installed)",
            InstallState::UpdateReady => "Setup (update ready)",
            InstallState::Installed => "Setup (installed)",
            InstallState::Unknown => "Setup",
        }
    }
}

#[derive(Clone, Debug)]
struct InputField {
    label: &'static str,
    value: String,
    mask: bool,
}

impl InputField {
    fn new(label: &'static str) -> Self {
        Self {
            label,
            value: String::new(),
            mask: false,
        }
    }

    fn with_mask(label: &'static str) -> Self {
        Self {
            label,
            value: String::new(),
            mask: true,
        }
    }

    fn display_value(&self) -> String {
        if self.mask {
            "*".repeat(self.value.len())
        } else {
            self.value.clone()
        }
    }

    fn push(&mut self, ch: char) {
        self.value.push(ch);
    }

    fn pop(&mut self) {
        self.value.pop();
    }
}

mod app_core;
mod draw;
mod handle;
mod helpers;
mod jobs;
#[cfg(test)]
mod tests;

use helpers::*;

struct TuiApp {
    config_path: std::path::PathBuf,
    config: AppConfigV2,
    view: View,
    menu_index: usize,
    message: String,
    input_index: usize,
    input_fields: Vec<InputField>,
    provider_index: usize,
    token_menu_index: usize,
    token_validation: HashMap<String, TokenValidation>,
    audit: AuditLogger,
    log_buffer: LogBuffer,
    audit_filter: AuditFilter,
    validation_message: Option<String>,
    show_target_stats: bool,
    repo_status: HashMap<String, RepoLocalStatus>,
    repo_status_last_refresh: Option<u64>,
    repo_status_refreshing: bool,
    repo_status_rx: Option<mpsc::Receiver<Result<HashMap<String, RepoLocalStatus>, String>>>,
    repo_overview_message: Option<String>,
    repo_overview_selected: usize,
    repo_overview_scroll: usize,
    repo_overview_collapsed: HashSet<String>,
    repo_overview_compact: bool,
    sync_running: bool,
    sync_rx: Option<mpsc::Receiver<Result<SyncSummary, String>>>,
    install_guard: Option<crate::install::InstallGuard>,
    install_rx: Option<mpsc::Receiver<InstallEvent>>,
    install_progress: Option<InstallProgressState>,
    install_status: Option<crate::install::InstallStatus>,
    update_rx: Option<mpsc::Receiver<UpdateEvent>>,
    update_progress: Option<UpdateProgressState>,
    update_prompt: Option<update::UpdateCheck>,
    update_return_view: View,
    restart_requested: bool,
    message_return_view: View,
    audit_search: String,
    audit_search_active: bool,
    view_stack: Vec<View>,
    scroll_offsets: HashMap<View, usize>,
}

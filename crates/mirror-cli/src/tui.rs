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

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
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
    install_scroll: usize,
    update_rx: Option<mpsc::Receiver<UpdateEvent>>,
    update_progress: Option<UpdateProgressState>,
    update_prompt: Option<update::UpdateCheck>,
    update_return_view: View,
    restart_requested: bool,
    message_return_view: View,
    audit_scroll: usize,
    audit_search: String,
    audit_search_active: bool,
}

impl TuiApp {
    fn load(
        audit: AuditLogger,
        log_buffer: LogBuffer,
        start_view: StartView,
    ) -> anyhow::Result<Self> {
        let config_path = default_config_path()?;
        let (config, migrated) = load_or_migrate(&config_path)?;
        if migrated {
            config.save(&config_path)?;
        }
        let view = match start_view {
            StartView::Dashboard => View::Dashboard,
            StartView::Install => View::Install,
            StartView::Main => View::Main,
        };
        let mut app = Self {
            config_path,
            config,
            view,
            menu_index: 0,
            message: String::new(),
            input_index: 0,
            input_fields: Vec::new(),
            provider_index: 0,
            token_menu_index: 0,
            token_validation: HashMap::new(),
            audit,
            log_buffer,
            audit_filter: AuditFilter::All,
            validation_message: None,
            show_target_stats: false,
            repo_status: HashMap::new(),
            repo_status_last_refresh: None,
            repo_status_refreshing: false,
            repo_status_rx: None,
            repo_overview_message: None,
            repo_overview_selected: 0,
            repo_overview_scroll: 0,
            repo_overview_collapsed: HashSet::new(),
            repo_overview_compact: false,
            sync_running: false,
            sync_rx: None,
            install_guard: None,
            install_rx: None,
            install_progress: None,
            install_status: None,
            install_scroll: 0,
            update_rx: None,
            update_progress: None,
            update_prompt: None,
            update_return_view: View::Main,
            restart_requested: false,
            message_return_view: View::Main,
            audit_scroll: 0,
            audit_search: String::new(),
            audit_search_active: false,
        };
        if app.view == View::Install {
            app.enter_install_view()?;
        }
        Ok(app)
    }

    fn prepare_install_form(&mut self) {
        let status = crate::install::install_status().ok();
        let mut delayed_start = InputField::new("Delayed start seconds (optional)");
        if let Some(value) = status
            .as_ref()
            .and_then(|state| state.delayed_start)
            .filter(|value| *value > 0)
        {
            delayed_start.value = value.to_string();
        }
        let mut path = InputField::new("Add CLI to PATH? (y/n)");
        if status
            .as_ref()
            .map(|state| state.path_in_env)
            .unwrap_or(false)
        {
            path.value = "y".to_string();
        } else {
            path.value = "n".to_string();
        }
        self.input_fields = vec![delayed_start, path];
        self.input_index = 0;
    }

    fn ensure_install_guard(&mut self) -> anyhow::Result<()> {
        if self.install_guard.is_none() {
            self.install_guard = Some(crate::install::acquire_install_lock()?);
        }
        Ok(())
    }

    fn release_install_guard(&mut self) {
        self.install_guard = None;
    }

    fn enter_install_view(&mut self) -> anyhow::Result<()> {
        self.ensure_install_guard()?;
        info!("Entered install view");
        self.view = View::Install;
        self.install_scroll = 0;
        self.prepare_install_form();
        self.drain_input_events()?;
        Ok(())
    }

    fn drain_input_events(&self) -> anyhow::Result<()> {
        while event::poll(Duration::from_millis(0))? {
            let _ = event::read()?;
        }
        Ok(())
    }

    fn draw(&mut self, frame: &mut ratatui::Frame) {
        let layout = Layout::default()
            .direction(Direction::Vertical)
            .margin(1)
            .constraints([
                Constraint::Length(3),
                Constraint::Min(0),
                Constraint::Length(LOG_PANEL_HEIGHT),
                Constraint::Length(3),
            ])
            .split(frame.area());

        let header = Paragraph::new("Git Project Sync — Terminal UI")
            .block(Block::default().borders(Borders::ALL).title("Header"));
        frame.render_widget(header, layout[0]);

        match self.view {
            View::Main => self.draw_main(frame, layout[1]),
            View::Dashboard => self.draw_dashboard(frame, layout[1]),
            View::Install => self.draw_install(frame, layout[1]),
            View::UpdatePrompt => self.draw_update_prompt(frame, layout[1]),
            View::UpdateProgress => self.draw_update_progress(frame, layout[1]),
            View::SyncStatus => self.draw_sync_status(frame, layout[1]),
            View::InstallStatus => self.draw_install_status(frame, layout[1]),
            View::ConfigRoot => self.draw_config_root(frame, layout[1]),
            View::RepoOverview => self.draw_repo_overview(frame, layout[1]),
            View::Targets => self.draw_targets(frame, layout[1]),
            View::TargetAdd => self.draw_form(frame, layout[1], "Add Target"),
            View::TargetRemove => self.draw_form(frame, layout[1], "Remove Target"),
            View::TokenMenu => self.draw_token_menu(frame, layout[1]),
            View::TokenList => self.draw_token_list(frame, layout[1]),
            View::TokenSet => self.draw_token_set(frame, layout[1]),
            View::TokenValidate => self.draw_token_validate(frame, layout[1]),
            View::Service => self.draw_service(frame, layout[1]),
            View::AuditLog => self.draw_audit_log(frame, layout[1]),
            View::Message => self.draw_message(frame, layout[1]),
            View::InstallProgress => self.draw_install_progress(frame, layout[1]),
        }

        self.draw_log_panel(frame, layout[2]);

        let footer = Paragraph::new(self.footer_text())
            .block(Block::default().borders(Borders::ALL).title("Help"));
        frame.render_widget(footer, layout[3]);
    }

    fn footer_text(&self) -> String {
        match self.view {
            View::Main => "Up/Down: navigate | Enter: select | q: quit".to_string(),
            View::Dashboard => dashboard_footer_text().to_string(),
            View::Install => {
                let status = crate::install::install_status().ok();
                let action = install_action_from_status(status.as_ref());
                format!(
                    "Tab: next | Enter: {} | s: status | u: check updates | Esc: back",
                    action.verb()
                )
            }
            View::UpdatePrompt => "y: apply update | n: cancel | Esc: back".to_string(),
            View::UpdateProgress => "Updating... please wait".to_string(),
            View::SyncStatus => "Enter/Esc: back".to_string(),
            View::InstallStatus => "Enter/Esc: back".to_string(),
            View::ConfigRoot => "Enter: save | Esc: back".to_string(),
            View::RepoOverview => {
                "Up/Down: scroll | PgUp/PgDn | Enter: collapse | c: compact | r: refresh | Esc: back"
                    .to_string()
            }
            View::Targets => "a: add | d: remove | Esc: back".to_string(),
            View::TargetAdd | View::TargetRemove | View::TokenSet | View::TokenValidate => {
                "Tab: next field | Enter: submit | Esc: back".to_string()
            }
            View::TokenMenu => "Up/Down: navigate | Enter: select | Esc: back".to_string(),
            View::TokenList => "Esc: back".to_string(),
            View::Service => "i: install | u: uninstall | Esc: back".to_string(),
            View::AuditLog => {
                "Up/Down: scroll | PgUp/PgDn | /: search | f: failures | a: all | Esc: back"
                    .to_string()
            }
            View::Message => "Enter: back".to_string(),
            View::InstallProgress => "Applying setup... please wait".to_string(),
        }
    }

    fn draw_main(&self, frame: &mut ratatui::Frame, area: ratatui::layout::Rect) {
        let status = crate::install::install_status().ok();
        let action = install_action_from_status(status.as_ref());
        let state = install_state_from_status(status.as_ref(), action);
        let items = vec![
            "Dashboard".to_string(),
            state.menu_label().to_string(),
            "Config".to_string(),
            "Targets".to_string(),
            "Tokens".to_string(),
            "Service".to_string(),
            "Audit Log".to_string(),
            "Repo Overview".to_string(),
            "Update".to_string(),
            "Quit".to_string(),
        ];
        let list_items: Vec<ListItem> = items
            .iter()
            .enumerate()
            .map(|(idx, item)| {
                let mut line = Line::from(Span::raw(item.as_str()));
                if idx == self.menu_index {
                    line = line.style(Style::default().add_modifier(Modifier::BOLD));
                }
                ListItem::new(line)
            })
            .collect();
        let list =
            List::new(list_items).block(Block::default().borders(Borders::ALL).title("Main Menu"));
        frame.render_widget(list, area);
    }

    fn draw_dashboard(&self, frame: &mut ratatui::Frame, area: ratatui::layout::Rect) {
        let stats = self.dashboard_stats();
        let mut lines = vec![
            Line::from(Span::raw("Dashboard: System status")),
            Line::from(Span::raw("")),
            Line::from(Span::raw(format!("Targets: {} total", stats.total_targets))),
            Line::from(Span::raw(format!("Healthy: {}", stats.healthy_targets))),
            Line::from(Span::raw(format!("Backoff: {}", stats.backoff_targets))),
            Line::from(Span::raw(format!(
                "No recent success: {}",
                stats.no_success_targets
            ))),
            Line::from(Span::raw(format!(
                "Last sync: {}",
                stats.last_sync.unwrap_or_else(|| "unknown".to_string())
            ))),
            Line::from(Span::raw(format!(
                "Audit entries today: {}",
                stats.audit_entries
            ))),
        ];
        if self.show_target_stats {
            lines.push(Line::from(Span::raw("")));
            lines.push(Line::from(Span::raw("Per-target status:")));
            for target in stats.targets {
                lines.push(Line::from(Span::raw(format!(
                    "{} | {} | {}",
                    target.id, target.status, target.last_success
                ))));
            }
        } else {
            lines.push(Line::from(Span::raw("")));
            lines.push(Line::from(Span::raw("Press t to show per-target status")));
        }
        let widget = Paragraph::new(lines)
            .wrap(Wrap { trim: false })
            .block(Block::default().borders(Borders::ALL).title("Dashboard"));
        frame.render_widget(widget, area);
    }

    fn draw_install(&self, frame: &mut ratatui::Frame, area: ratatui::layout::Rect) {
        let status = crate::install::install_status().ok();
        let action = install_action_from_status(status.as_ref());
        let mut lines = vec![
            Line::from(Span::raw(
                "Context: Setup status with install/update actions",
            )),
            Line::from(Span::raw("")),
            Line::from(Span::raw(format!(
                "Current version: {}",
                env!("CARGO_PKG_VERSION")
            ))),
        ];
        if let Some(status) = status.as_ref() {
            let service_label = if cfg!(target_os = "windows") {
                "Scheduled task"
            } else {
                "Service"
            };
            lines.push(Line::from(Span::raw(format!(
                "Installed: {}",
                if status.installed { "yes" } else { "no" }
            ))));
            lines.push(Line::from(Span::raw(format!(
                "Action: {} (press Enter)",
                action.label()
            ))));
            lines.push(Line::from(Span::raw(format!(
                "Path: {}",
                status
                    .installed_path
                    .as_ref()
                    .map(|p| p.display().to_string())
                    .unwrap_or_else(|| "(unknown)".to_string())
            ))));
            if let Some(value) = status.installed_version.as_deref() {
                lines.push(Line::from(Span::raw(format!("Installed version: {value}"))));
            }
            if let Some(value) = status.installed_at {
                lines.push(Line::from(Span::raw(format!(
                    "Installed at: {}",
                    epoch_to_label(value)
                ))));
            }
            lines.push(Line::from(Span::raw(format!(
                "Startup delay: {}",
                format_delayed_start(status.delayed_start)
            ))));
            lines.push(Line::from(Span::raw(format!(
                "{} installed: {}",
                service_label,
                if status.service_installed {
                    "yes"
                } else {
                    "no"
                }
            ))));
            lines.push(Line::from(Span::raw(format!(
                "{} running: {}",
                service_label,
                if status.service_running { "yes" } else { "no" }
            ))));
            if let Some(value) = status.service_last_run.as_deref() {
                lines.push(Line::from(Span::raw(format!("Last run: {value}"))));
            }
            if let Some(value) = status.service_next_run.as_deref() {
                lines.push(Line::from(Span::raw(format!("Next run: {value}"))));
            }
            if let Some(value) = status.service_last_result.as_deref() {
                lines.push(Line::from(Span::raw(format!("Last result: {value}"))));
            }
            if cfg!(target_os = "windows") {
                if let Some(value) = status.service_task_state.as_deref() {
                    lines.push(Line::from(Span::raw(format!("Task state: {value}"))));
                }
                if let Some(value) = status.service_schedule_type.as_deref() {
                    lines.push(Line::from(Span::raw(format!("Schedule type: {value}"))));
                }
                if let Some(value) = status.service_start_date.as_deref() {
                    lines.push(Line::from(Span::raw(format!("Start date: {value}"))));
                }
                if let Some(value) = status.service_start_time.as_deref() {
                    lines.push(Line::from(Span::raw(format!("Start time: {value}"))));
                }
                if let Some(value) = status.service_run_as.as_deref() {
                    lines.push(Line::from(Span::raw(format!("Run as: {value}"))));
                }
                if let Some(value) = status.service_task_to_run.as_deref() {
                    lines.push(Line::from(Span::raw(format!("Task command: {value}"))));
                }
                lines.push(Line::from(Span::raw("Task name: git-project-sync")));
            }
            lines.push(Line::from(Span::raw(format!(
                "PATH contains install dir (current shell): {}",
                if status.path_in_env { "yes" } else { "no" }
            ))));
            lines.push(Line::from(Span::raw("")));
        } else {
            lines.push(Line::from(Span::raw(format!(
                "Action: {} (press Enter)",
                action.label()
            ))));
            lines.push(Line::from(Span::raw("Status unavailable.")));
            lines.push(Line::from(Span::raw("")));
        }
        lines.push(Line::from(Span::raw("Tip: Press u to check for updates.")));
        lines.push(Line::from(Span::raw("")));
        for (idx, field) in self.input_fields.iter().enumerate() {
            let label = if idx == self.input_index {
                format!("> {}: {}", field.label, field.display_value())
            } else {
                format!("  {}: {}", field.label, field.display_value())
            };
            lines.push(Line::from(Span::raw(label)));
        }
        if let Some(message) = self.validation_message.as_deref() {
            lines.push(Line::from(Span::raw("")));
            lines.push(Line::from(Span::raw(format!("Validation: {message}"))));
        }
        let body_height = area.height.saturating_sub(2) as usize;
        let max_scroll = lines.len().saturating_sub(body_height);
        let scroll = self.install_scroll.min(max_scroll);
        let widget = Paragraph::new(lines)
            .wrap(Wrap { trim: false })
            .scroll((scroll as u16, 0))
            .block(Block::default().borders(Borders::ALL).title("Setup"));
        frame.render_widget(widget, area);
    }

    fn draw_install_progress(&self, frame: &mut ratatui::Frame, area: ratatui::layout::Rect) {
        let mut lines = vec![
            Line::from(Span::raw("Context: Applying setup... please wait")),
            Line::from(Span::raw("")),
        ];
        if let Some(progress) = &self.install_progress {
            let bar = progress_bar(progress.current, progress.total, 20);
            lines.push(Line::from(Span::raw(format!(
                "Step {}/{} {}",
                progress.current, progress.total, bar
            ))));
            lines.push(Line::from(Span::raw("")));
            for line in &progress.messages {
                lines.push(Line::from(Span::raw(line)));
            }
        } else {
            lines.push(Line::from(Span::raw("Starting installer...")));
        }
        let widget = Paragraph::new(lines)
            .wrap(Wrap { trim: false })
            .block(Block::default().borders(Borders::ALL).title("Setup"));
        frame.render_widget(widget, area);
    }

    fn draw_update_prompt(&self, frame: &mut ratatui::Frame, area: ratatui::layout::Rect) {
        let widget = Paragraph::new(self.message.clone())
            .wrap(Wrap { trim: false })
            .block(Block::default().borders(Borders::ALL).title("Update"));
        frame.render_widget(widget, area);
    }

    fn draw_update_progress(&self, frame: &mut ratatui::Frame, area: ratatui::layout::Rect) {
        let mut lines = vec![
            Line::from(Span::raw("Context: Updating... please wait")),
            Line::from(Span::raw("")),
        ];
        if let Some(progress) = &self.update_progress {
            for line in &progress.messages {
                lines.push(Line::from(Span::raw(line)));
            }
        } else {
            lines.push(Line::from(Span::raw("Starting update...")));
        }
        let widget = Paragraph::new(lines)
            .wrap(Wrap { trim: false })
            .block(Block::default().borders(Borders::ALL).title("Update"));
        frame.render_widget(widget, area);
    }

    fn draw_install_status(&self, frame: &mut ratatui::Frame, area: ratatui::layout::Rect) {
        let mut lines = vec![
            Line::from(Span::raw("Context: Setup status")),
            Line::from(Span::raw("")),
        ];
        if let Some(status) = &self.install_status {
            let service_label = if cfg!(target_os = "windows") {
                "Scheduled task"
            } else {
                "Service"
            };
            lines.push(Line::from(Span::raw(format!(
                "Installed: {}",
                if status.installed { "yes" } else { "no" }
            ))));
            lines.push(Line::from(Span::raw(format!(
                "Installed path: {}",
                status
                    .installed_path
                    .as_ref()
                    .map(|p| p.display().to_string())
                    .unwrap_or_else(|| "(unknown)".to_string())
            ))));
            lines.push(Line::from(Span::raw(format!(
                "Manifest present: {}",
                if status.manifest_present { "yes" } else { "no" }
            ))));
            if let Some(value) = status.installed_version.as_deref() {
                lines.push(Line::from(Span::raw(format!("Installed version: {value}"))));
            }
            if let Some(value) = status.installed_at {
                lines.push(Line::from(Span::raw(format!(
                    "Installed at: {}",
                    epoch_to_label(value)
                ))));
            }
            lines.push(Line::from(Span::raw(format!(
                "Startup delay: {}",
                format_delayed_start(status.delayed_start)
            ))));
            lines.push(Line::from(Span::raw(format!(
                "{} installed: {}",
                service_label,
                if status.service_installed {
                    "yes"
                } else {
                    "no"
                }
            ))));
            lines.push(Line::from(Span::raw(format!(
                "{} running: {}",
                service_label,
                if status.service_running { "yes" } else { "no" }
            ))));
            if let Some(value) = status.service_last_run.as_deref() {
                lines.push(Line::from(Span::raw(format!("Last run: {value}"))));
            }
            if let Some(value) = status.service_next_run.as_deref() {
                lines.push(Line::from(Span::raw(format!("Next run: {value}"))));
            }
            if let Some(value) = status.service_last_result.as_deref() {
                lines.push(Line::from(Span::raw(format!("Last result: {value}"))));
            }
            if cfg!(target_os = "windows") {
                if let Some(value) = status.service_task_state.as_deref() {
                    lines.push(Line::from(Span::raw(format!("Task state: {value}"))));
                }
                if let Some(value) = status.service_schedule_type.as_deref() {
                    lines.push(Line::from(Span::raw(format!("Schedule type: {value}"))));
                }
                if let Some(value) = status.service_start_date.as_deref() {
                    lines.push(Line::from(Span::raw(format!("Start date: {value}"))));
                }
                if let Some(value) = status.service_start_time.as_deref() {
                    lines.push(Line::from(Span::raw(format!("Start time: {value}"))));
                }
                if let Some(value) = status.service_run_as.as_deref() {
                    lines.push(Line::from(Span::raw(format!("Run as: {value}"))));
                }
                if let Some(value) = status.service_task_to_run.as_deref() {
                    lines.push(Line::from(Span::raw(format!("Task command: {value}"))));
                }
                lines.push(Line::from(Span::raw("Task name: git-project-sync")));
            }
            lines.push(Line::from(Span::raw(format!(
                "PATH contains install dir (current shell): {}",
                if status.path_in_env { "yes" } else { "no" }
            ))));
        } else {
            lines.push(Line::from(Span::raw("Status unavailable.")));
        }
        let body_height = area.height.saturating_sub(2) as usize;
        let max_scroll = lines.len().saturating_sub(body_height);
        let scroll = self.install_scroll.min(max_scroll);
        let widget = Paragraph::new(lines)
            .wrap(Wrap { trim: false })
            .scroll((scroll as u16, 0))
            .block(Block::default().borders(Borders::ALL).title("Setup Status"));
        frame.render_widget(widget, area);
    }

    fn draw_sync_status(&self, frame: &mut ratatui::Frame, area: ratatui::layout::Rect) {
        let mut lines = vec![
            Line::from(Span::raw("Context: Sync status by target")),
            Line::from(Span::raw("")),
        ];
        match self.sync_status_lines() {
            Ok(mut status_lines) => {
                if status_lines.is_empty() {
                    lines.push(Line::from(Span::raw("No targets configured.")));
                } else {
                    lines.append(&mut status_lines);
                }
            }
            Err(err) => {
                lines.push(Line::from(Span::raw(format!(
                    "Failed to load sync status: {err}"
                ))));
            }
        }
        let widget = Paragraph::new(lines)
            .wrap(Wrap { trim: false })
            .block(Block::default().borders(Borders::ALL).title("Sync Status"));
        frame.render_widget(widget, area);
    }

    fn draw_audit_log(&mut self, frame: &mut ratatui::Frame, area: ratatui::layout::Rect) {
        let log_path = self.audit_log_path();
        let mut lines = match read_audit_lines(&log_path, self.audit_filter) {
            Ok(lines) => lines,
            Err(err) => vec![format!("Failed to read audit log: {err}")],
        };
        let query = self.audit_search.trim();
        if !query.is_empty() {
            let needle = query.to_lowercase();
            lines.retain(|line| line.to_lowercase().contains(&needle));
        }

        let header_lines = vec![
            Line::from(Span::raw("Context: Audit log entries (newest first)")),
            Line::from(Span::raw(format!(
                "Filter: {} | Search: {}{}",
                match self.audit_filter {
                    AuditFilter::All => "all",
                    AuditFilter::Failures => "failures",
                },
                if self.audit_search.is_empty() {
                    "<none>"
                } else {
                    &self.audit_search
                },
                if self.audit_search_active {
                    " (editing)"
                } else {
                    ""
                }
            ))),
            Line::from(Span::raw("")),
        ];

        let layout = Layout::default()
            .direction(Direction::Vertical)
            .constraints([
                Constraint::Length(header_lines.len() as u16 + 2),
                Constraint::Min(0),
            ])
            .split(area);

        let header = Paragraph::new(header_lines).block(
            Block::default()
                .borders(Borders::ALL)
                .title("Audit Log Viewer"),
        );
        frame.render_widget(header, layout[0]);

        let body_height = layout[1].height as usize;
        let max_scroll = lines.len().saturating_sub(body_height);
        let scroll = self.audit_scroll.min(max_scroll);
        self.audit_scroll = scroll;
        let visible_lines = slice_with_scroll(&lines, self.audit_scroll, body_height);
        let list_items: Vec<ListItem> = visible_lines
            .into_iter()
            .map(|line| ListItem::new(Line::from(Span::raw(line))))
            .collect();
        let list = List::new(list_items).block(Block::default().borders(Borders::ALL));
        frame.render_widget(list, layout[1]);
    }

    fn draw_token_menu(&self, frame: &mut ratatui::Frame, area: ratatui::layout::Rect) {
        let items = ["List", "Set/Update", "Validate", "Back"];
        let list_items: Vec<ListItem> = items
            .iter()
            .enumerate()
            .map(|(idx, item)| {
                let mut line = Line::from(Span::raw(*item));
                if idx == self.token_menu_index {
                    line = line.style(Style::default().add_modifier(Modifier::BOLD));
                }
                ListItem::new(line)
            })
            .collect();
        let list = List::new(list_items).block(
            Block::default()
                .borders(Borders::ALL)
                .title("Token Management"),
        );
        frame.render_widget(list, area);
    }

    fn draw_token_list(&self, frame: &mut ratatui::Frame, area: ratatui::layout::Rect) {
        let entries = self.token_entries();
        let mut items: Vec<ListItem> = Vec::new();
        items.push(ListItem::new(Line::from(Span::raw(
            "Context: Tokens per configured target",
        ))));
        items.push(ListItem::new(Line::from(Span::raw(""))));
        if entries.is_empty() {
            items.push(ListItem::new(Line::from(Span::raw(
                "No targets configured yet.",
            ))));
        } else {
            for entry in entries {
                let status = if entry.present { "stored" } else { "missing" };
                let validation = entry
                    .validation
                    .as_ref()
                    .map(|v| format!(" | {}", v.display()))
                    .unwrap_or_else(|| " | not verified".to_string());
                items.push(ListItem::new(Line::from(Span::raw(format!(
                    "{} | {} | {} | {} | {}{}",
                    entry.account,
                    entry.provider.as_prefix(),
                    entry.scope,
                    entry.host,
                    status,
                    validation
                )))));
            }
        }
        let list =
            List::new(items).block(Block::default().borders(Borders::ALL).title("Token List"));
        frame.render_widget(list, area);
    }

    fn draw_token_set(&self, frame: &mut ratatui::Frame, area: ratatui::layout::Rect) {
        let provider = provider_kind(self.provider_index);
        let help = pat_help(provider.clone());
        let mut lines = Vec::new();
        lines.push(Line::from(Span::raw("Context: Set or update a token")));
        lines.push(Line::from(Span::raw("")));
        lines.push(Line::from(Span::raw(format!(
            "Provider: {}",
            provider_label(self.provider_index)
        ))));
        lines.push(Line::from(Span::raw(format!("Create PAT: {}", help.url))));
        lines.push(Line::from(Span::raw(format!(
            "Required scopes: {}",
            help.scopes.join(", ")
        ))));
        lines.push(Line::from(Span::raw(
            "Tip: Scope uses space-separated segments.",
        )));
        if let Some(message) = self.validation_message.as_deref() {
            lines.push(Line::from(Span::raw("")));
            lines.push(Line::from(Span::raw(format!("Validation: {message}"))));
        }
        lines.push(Line::from(Span::raw("")));
        for (idx, field) in self.input_fields.iter().enumerate() {
            let label = if idx == self.input_index {
                format!("> {}: {}", field.label, field.display_value())
            } else {
                format!("  {}: {}", field.label, field.display_value())
            };
            lines.push(Line::from(Span::raw(label)));
        }
        let widget = Paragraph::new(lines).wrap(Wrap { trim: false }).block(
            Block::default()
                .borders(Borders::ALL)
                .title("Set/Update Token"),
        );
        frame.render_widget(widget, area);
    }

    fn draw_token_validate(&self, frame: &mut ratatui::Frame, area: ratatui::layout::Rect) {
        let provider = provider_kind(self.provider_index);
        let help = pat_help(provider.clone());
        let mut lines = Vec::new();
        lines.push(Line::from(Span::raw("Context: Validate required scopes")));
        lines.push(Line::from(Span::raw("")));
        lines.push(Line::from(Span::raw(format!(
            "Provider: {}",
            provider_label(self.provider_index)
        ))));
        lines.push(Line::from(Span::raw(format!(
            "Required scopes: {}",
            help.scopes.join(", ")
        ))));
        lines.push(Line::from(Span::raw(
            "Tip: Host optional; defaults to provider host.",
        )));
        if let Some(message) = self.validation_message.as_deref() {
            lines.push(Line::from(Span::raw("")));
            lines.push(Line::from(Span::raw(format!("Validation: {message}"))));
        }
        lines.push(Line::from(Span::raw("")));
        for (idx, field) in self.input_fields.iter().enumerate() {
            let label = if idx == self.input_index {
                format!("> {}: {}", field.label, field.display_value())
            } else {
                format!("  {}: {}", field.label, field.display_value())
            };
            lines.push(Line::from(Span::raw(label)));
        }
        let widget = Paragraph::new(lines).wrap(Wrap { trim: false }).block(
            Block::default()
                .borders(Borders::ALL)
                .title("Validate Token"),
        );
        frame.render_widget(widget, area);
    }

    fn draw_service(&self, frame: &mut ratatui::Frame, area: ratatui::layout::Rect) {
        let os = std::env::consts::OS;
        let os_hint = match os {
            "linux" => "systemd user service",
            "macos" => "LaunchAgent",
            "windows" => "Windows service",
            _ => "service helper",
        };
        let lines = vec![
            Line::from(Span::raw("Install or uninstall the background service.")),
            Line::from(Span::raw(format!("Detected OS: {os} ({os_hint})"))),
            Line::from(Span::raw("")),
            Line::from(Span::raw("Press i to install")),
            Line::from(Span::raw("Press u to uninstall")),
        ];
        let widget = Paragraph::new(lines).wrap(Wrap { trim: false }).block(
            Block::default()
                .borders(Borders::ALL)
                .title("Service Installer"),
        );
        frame.render_widget(widget, area);
    }

    fn draw_config_root(&self, frame: &mut ratatui::Frame, area: ratatui::layout::Rect) {
        let current = self
            .config
            .root
            .as_ref()
            .map(|p| p.display().to_string())
            .unwrap_or_else(|| "<unset>".to_string());
        let mut lines = vec![
            Line::from(Span::raw("Context: Select the mirror root folder")),
            Line::from(Span::raw(format!("Current root: {current}"))),
            Line::from(Span::raw("")),
            Line::from(Span::raw(
                "Tip: Use an absolute path (e.g. /path/to/mirrors)",
            )),
        ];
        if let Some(message) = self.validation_message.as_deref() {
            lines.push(Line::from(Span::raw("")));
            lines.push(Line::from(Span::raw(format!("Validation: {message}"))));
        }
        lines.push(Line::from(Span::raw("")));
        lines.push(Line::from(Span::raw("New root:")));
        lines.push(Line::from(Span::raw(
            self.input_fields
                .first()
                .map(|f| f.display_value())
                .unwrap_or_default(),
        )));
        let widget = Paragraph::new(lines)
            .wrap(Wrap { trim: false })
            .block(Block::default().borders(Borders::ALL).title("Config Root"));
        frame.render_widget(widget, area);
    }

    fn draw_repo_overview(&mut self, frame: &mut ratatui::Frame, area: ratatui::layout::Rect) {
        let root = self.config.root.as_deref();
        let root_label = root
            .map(|p| p.display().to_string())
            .unwrap_or_else(|| "<unset>".to_string());
        let last_refresh = self
            .repo_status_last_refresh
            .map(epoch_to_label)
            .unwrap_or_else(|| "never".to_string());

        let mut header_lines = vec![
            Line::from(Span::raw("Context: Repo overview (cache)")),
            Line::from(Span::raw(format!("Root: {root_label}"))),
            Line::from(Span::raw(format!("Status refresh: {last_refresh}"))),
        ];
        if let Some(message) = self.repo_overview_message.as_deref() {
            header_lines.push(Line::from(Span::raw(message)));
        } else if self.repo_status_refreshing {
            header_lines.push(Line::from(Span::raw("Refreshing repo status...")));
        }

        let layout = Layout::default()
            .direction(Direction::Vertical)
            .constraints([
                Constraint::Length(header_lines.len() as u16 + 2),
                Constraint::Min(0),
            ])
            .split(area);

        let header = Paragraph::new(header_lines)
            .wrap(Wrap { trim: false })
            .block(
                Block::default()
                    .borders(Borders::ALL)
                    .title("Repo Overview"),
            );
        frame.render_widget(header, layout[0]);

        let rows = self.current_overview_rows();
        let visible_rows = self.visible_overview_rows(&rows);
        let body_height = layout[1].height as usize;
        let max_scroll = visible_rows.len().saturating_sub(body_height);
        let scroll = self.repo_overview_scroll.min(max_scroll);
        let selected = clamp_index(self.repo_overview_selected, visible_rows.len());
        let scroll = adjust_scroll(selected, scroll, body_height, visible_rows.len());
        self.repo_overview_selected = selected;
        self.repo_overview_scroll = scroll;
        let start = scroll;
        let end = (start + body_height).min(visible_rows.len());
        let mut items = Vec::new();
        let area_width = layout[1].width as usize;
        let name_width = name_column_width(area_width, self.repo_overview_compact);
        let header = format_overview_header(name_width, self.repo_overview_compact);
        items.push(ListItem::new(Line::from(Span::raw(header))));
        for (idx, row) in visible_rows[start..end].iter().enumerate() {
            let mut line = Line::from(Span::raw(format_overview_row(
                row,
                name_width,
                self.repo_overview_compact,
            )));
            let selected_idx = start + idx;
            if selected_idx == selected {
                line = line.style(Style::default().add_modifier(Modifier::BOLD));
            }
            items.push(ListItem::new(line));
        }

        let list = List::new(items).block(Block::default().borders(Borders::ALL));
        frame.render_widget(list, layout[1]);
    }

    fn draw_targets(&self, frame: &mut ratatui::Frame, area: ratatui::layout::Rect) {
        let mut items: Vec<ListItem> = Vec::new();
        items.push(ListItem::new(Line::from(Span::raw(
            "Context: Targets map to provider + scope",
        ))));
        items.push(ListItem::new(Line::from(Span::raw(
            "Tip: Press a to add or d to remove",
        ))));
        items.push(ListItem::new(Line::from(Span::raw(""))));
        if self.config.targets.is_empty() {
            items.push(ListItem::new(Line::from(Span::raw(
                "No targets configured.",
            ))));
        } else {
            for target in &self.config.targets {
                let host = target
                    .host
                    .clone()
                    .unwrap_or_else(|| "(default)".to_string());
                items.push(ListItem::new(Line::from(Span::raw(format!(
                    "{} | {} | {} | {}",
                    target.id,
                    target.provider.as_prefix(),
                    target.scope.segments().join("/"),
                    host
                )))));
            }
        }
        let list = List::new(items).block(Block::default().borders(Borders::ALL).title("Targets"));
        frame.render_widget(list, area);
    }

    fn draw_form(&self, frame: &mut ratatui::Frame, area: ratatui::layout::Rect, title: &str) {
        let mut lines = Vec::new();
        if let Some(context) = self.form_context() {
            lines.push(Line::from(Span::raw(context)));
            lines.push(Line::from(Span::raw("")));
        }
        if let Some(hint) = self.form_hint() {
            lines.push(Line::from(Span::raw(format!("Tip: {hint}"))));
            lines.push(Line::from(Span::raw("")));
        }
        if matches!(
            self.view,
            View::TargetAdd | View::TokenSet | View::TokenValidate
        ) {
            lines.push(Line::from(Span::raw(provider_selector_line(
                self.provider_index,
            ))));
            lines.push(Line::from(Span::raw(
                "Tip: Use Left/Right to change provider",
            )));
            lines.push(Line::from(Span::raw("")));
        }
        if let Some(message) = self.validation_message.as_deref() {
            lines.push(Line::from(Span::raw(format!("Validation: {message}"))));
            lines.push(Line::from(Span::raw("")));
        }
        for (idx, field) in self.input_fields.iter().enumerate() {
            let label = if idx == self.input_index {
                format!("> {}: {}", field.label, field.display_value())
            } else {
                format!("  {}: {}", field.label, field.display_value())
            };
            lines.push(Line::from(Span::raw(label)));
        }
        let widget = Paragraph::new(lines)
            .wrap(Wrap { trim: false })
            .block(Block::default().borders(Borders::ALL).title(title));
        frame.render_widget(widget, area);
    }

    fn draw_message(&self, frame: &mut ratatui::Frame, area: ratatui::layout::Rect) {
        let widget = Paragraph::new(self.message.clone())
            .wrap(Wrap { trim: false })
            .block(Block::default().borders(Borders::ALL).title("Message"));
        frame.render_widget(widget, area);
    }

    fn draw_log_panel(&self, frame: &mut ratatui::Frame, area: ratatui::layout::Rect) {
        let max_lines = area.height.saturating_sub(LOG_PANEL_BORDER_HEIGHT) as usize;
        if max_lines == 0 {
            return;
        }
        let header = Line::from(Span::styled(
            "time     level target message",
            Style::default().add_modifier(Modifier::BOLD),
        ));
        let mut lines = Vec::new();
        lines.push(header);
        let entries = self.log_buffer.entries();
        if entries.is_empty() {
            lines.push(Line::from(Span::raw("No log messages yet.")));
        } else if max_lines > LOG_HEADER_LINES {
            let max_entries = max_lines.saturating_sub(LOG_HEADER_LINES);
            let start = entries.len().saturating_sub(max_entries);
            for entry in entries[start..].iter() {
                lines.push(Line::from(Span::raw(entry.format_compact())));
            }
        }
        let widget = Paragraph::new(lines)
            .wrap(Wrap { trim: false })
            .block(Block::default().borders(Borders::ALL).title("Logs"));
        frame.render_widget(widget, area);
    }

    fn handle_key(&mut self, key: KeyEvent) -> anyhow::Result<bool> {
        if key.kind != KeyEventKind::Press {
            return Ok(false);
        }
        match self.view {
            View::Main => self.handle_main(key),
            View::Dashboard => self.handle_dashboard(key),
            View::Install => self.handle_install(key),
            View::UpdatePrompt => self.handle_update_prompt(key),
            View::UpdateProgress => self.handle_update_progress(key),
            View::ConfigRoot => self.handle_config_root(key),
            View::RepoOverview => self.handle_repo_overview(key),
            View::Targets => self.handle_targets(key),
            View::TargetAdd => self.handle_target_add(key),
            View::TargetRemove => self.handle_target_remove(key),
            View::TokenMenu => self.handle_token_menu(key),
            View::TokenList => self.handle_token_list(key),
            View::TokenSet => self.handle_token_set(key),
            View::TokenValidate => self.handle_token_validate(key),
            View::Service => self.handle_service(key),
            View::AuditLog => self.handle_audit_log(key),
            View::Message => self.handle_message(key),
            View::InstallProgress => self.handle_install_progress(key),
            View::InstallStatus => self.handle_install_status(key),
            View::SyncStatus => self.handle_sync_status(key),
        }
    }

    fn handle_main(&mut self, key: KeyEvent) -> anyhow::Result<bool> {
        match key.code {
            KeyCode::Char('q') => return Ok(true),
            KeyCode::Down => self.menu_index = (self.menu_index + 1) % 10,
            KeyCode::Up => {
                if self.menu_index == 0 {
                    self.menu_index = 9;
                } else {
                    self.menu_index -= 1;
                }
            }
            KeyCode::Enter => {
                info!(selection = self.menu_index, "Main menu selected");
                match self.menu_index {
                    0 => {
                        info!("Switching to dashboard view");
                        self.view = View::Dashboard;
                    }
                    1 => {
                        if let Err(err) = self.enter_install_view() {
                            error!(error = %err, "Setup unavailable");
                            self.message = format!("Setup unavailable: {err}");
                            self.view = View::Message;
                        }
                    }
                    2 => {
                        info!("Switching to config root view");
                        self.view = View::ConfigRoot;
                        self.input_fields = vec![InputField::new("Root path")];
                        self.input_index = 0;
                    }
                    3 => {
                        info!("Switching to targets view");
                        self.view = View::Targets;
                    }
                    4 => {
                        info!("Switching to token menu view");
                        self.view = View::TokenMenu;
                        self.token_menu_index = 0;
                    }
                    5 => {
                        info!("Switching to service view");
                        self.view = View::Service;
                    }
                    6 => {
                        info!("Switching to audit log view");
                        self.view = View::AuditLog;
                        self.audit_scroll = 0;
                        self.audit_search_active = false;
                    }
                    7 => {
                        if let Err(err) = self.enter_repo_overview() {
                            error!(error = %err, "Repo overview unavailable");
                            self.message = format!("Repo overview unavailable: {err}");
                            self.view = View::Message;
                        }
                    }
                    8 => {
                        info!("Starting update check from main menu");
                        self.start_update_check(View::Main)?;
                    }
                    _ => return Ok(true),
                }
            }
            _ => {}
        }
        Ok(false)
    }

    fn handle_install(&mut self, key: KeyEvent) -> anyhow::Result<bool> {
        match key.code {
            KeyCode::Esc => {
                self.release_install_guard();
                self.view = View::Main;
            }
            KeyCode::Tab => self.input_index = (self.input_index + 1) % self.input_fields.len(),
            KeyCode::Down => self.install_scroll = self.install_scroll.saturating_add(1),
            KeyCode::Up => self.install_scroll = self.install_scroll.saturating_sub(1),
            KeyCode::PageDown => self.install_scroll = self.install_scroll.saturating_add(10),
            KeyCode::PageUp => self.install_scroll = self.install_scroll.saturating_sub(10),
            KeyCode::Home => self.install_scroll = 0,
            KeyCode::Enter => {
                if let Err(err) = self.ensure_install_guard() {
                    error!(error = %err, "Setup lock unavailable");
                    self.message = format!("Setup unavailable: {err}");
                    self.view = View::Message;
                    return Ok(false);
                }
                let delay_raw = self.input_fields[0].value.trim();
                let delayed_start = if delay_raw.is_empty() {
                    None
                } else {
                    match delay_raw.parse::<u64>() {
                        Ok(value) => Some(value),
                        Err(_) => {
                            warn!(value = delay_raw, "Invalid delayed start input");
                            self.validation_message =
                                Some("Delayed start must be a number.".to_string());
                            return Ok(false);
                        }
                    }
                };
                let path_raw = self.input_fields[1].value.trim().to_lowercase();
                let path_choice = if path_raw == "y" || path_raw == "yes" {
                    crate::install::PathChoice::Add
                } else {
                    crate::install::PathChoice::Skip
                };
                let exec = std::env::current_exe().context("resolve current executable")?;
                info!(
                    delayed_start = delayed_start,
                    path_choice = ?path_choice,
                    "Starting install"
                );
                let (tx, rx) = mpsc::channel::<InstallEvent>();
                thread::spawn(move || {
                    let result = crate::install::perform_install_with_progress(
                        &exec,
                        crate::install::InstallOptions {
                            delayed_start,
                            path_choice,
                        },
                        Some(&|progress| {
                            let _ = tx.send(InstallEvent::Progress(progress));
                        }),
                        None,
                    )
                    .map_err(|err| err.to_string());
                    let _ = tx.send(InstallEvent::Done(result));
                });
                self.install_rx = Some(rx);
                self.install_progress = None;
                self.view = View::InstallProgress;
            }
            KeyCode::Char('s') => {
                self.install_status = crate::install::install_status().ok();
                self.install_scroll = 0;
                self.view = View::InstallStatus;
            }
            KeyCode::Char('u') => {
                self.start_update_check(View::Install)?;
            }
            _ => self.handle_text_input(key),
        }
        Ok(false)
    }

    fn handle_dashboard(&mut self, key: KeyEvent) -> anyhow::Result<bool> {
        match key.code {
            KeyCode::Esc => self.view = View::Main,
            KeyCode::Char('t') => {
                self.show_target_stats = !self.show_target_stats;
                debug!(
                    show_target_stats = self.show_target_stats,
                    "Toggled target stats"
                );
            }
            KeyCode::Char('s') => {
                self.view = View::SyncStatus;
            }
            KeyCode::Char('r') => {
                self.start_sync_run(false)?;
            }
            KeyCode::Char('f') => {
                self.start_sync_run(true)?;
            }
            KeyCode::Char('u') => {
                self.start_update_check(View::Dashboard)?;
            }
            _ => {}
        }
        Ok(false)
    }

    fn handle_config_root(&mut self, key: KeyEvent) -> anyhow::Result<bool> {
        match key.code {
            KeyCode::Esc => self.view = View::Main,
            KeyCode::Enter => {
                let value = self
                    .input_fields
                    .first()
                    .map(|f| f.value.trim().to_string());
                if let Some(path) = value {
                    if path.is_empty() {
                        warn!("Root path empty in config root view");
                        self.validation_message = Some("Root path cannot be empty.".to_string());
                        self.message = "Root path cannot be empty.".to_string();
                        self.view = View::Message;
                        return Ok(false);
                    }
                    self.config.root = Some(path.into());
                    self.config.save(&self.config_path)?;
                    self.validation_message = None;
                    info!("Saved config root from TUI");
                    let audit_id = self.audit.record(
                        "tui.config.root",
                        AuditStatus::Ok,
                        Some("tui"),
                        None,
                        None,
                    )?;
                    self.message = format!("Root saved. Audit ID: {audit_id}");
                    self.view = View::Message;
                }
            }
            _ => {
                self.handle_text_input(key);
            }
        }
        Ok(false)
    }

    fn handle_repo_overview(&mut self, key: KeyEvent) -> anyhow::Result<bool> {
        match key.code {
            KeyCode::Esc => self.view = View::Main,
            KeyCode::Char('r') => {
                self.start_repo_status_refresh()?;
            }
            KeyCode::Char('c') => {
                self.repo_overview_compact = !self.repo_overview_compact;
                debug!(
                    compact = self.repo_overview_compact,
                    "Toggled repo overview compact mode"
                );
            }
            KeyCode::Down => {
                self.repo_overview_selected = self.repo_overview_selected.saturating_add(1);
            }
            KeyCode::Up => {
                self.repo_overview_selected = self.repo_overview_selected.saturating_sub(1);
            }
            KeyCode::PageDown => {
                self.repo_overview_selected = self.repo_overview_selected.saturating_add(10);
            }
            KeyCode::PageUp => {
                self.repo_overview_selected = self.repo_overview_selected.saturating_sub(10);
            }
            KeyCode::Home => {
                self.repo_overview_selected = 0;
            }
            KeyCode::End => {
                self.repo_overview_selected = usize::MAX;
            }
            KeyCode::Enter => {
                let rows = self.current_overview_rows();
                let visible = self.visible_overview_rows(&rows);
                if let Some(row) = visible.get(self.repo_overview_selected)
                    && !row.is_leaf
                {
                    if self.repo_overview_collapsed.contains(&row.id) {
                        self.repo_overview_collapsed.remove(&row.id);
                    } else {
                        self.repo_overview_collapsed.insert(row.id.clone());
                    }
                }
            }
            _ => {}
        }
        Ok(false)
    }

    fn handle_targets(&mut self, key: KeyEvent) -> anyhow::Result<bool> {
        match key.code {
            KeyCode::Esc => self.view = View::Main,
            KeyCode::Char('a') => {
                self.validation_message = None;
                self.view = View::TargetAdd;
                self.input_fields = vec![
                    InputField::new("Scope (space-separated)"),
                    InputField::new("Host (optional)"),
                    InputField::new("Labels (comma-separated)"),
                ];
                self.input_index = 0;
                self.provider_index = 0;
            }
            KeyCode::Char('d') => {
                self.validation_message = None;
                self.view = View::TargetRemove;
                self.input_fields = vec![InputField::new("Target id")];
                self.input_index = 0;
            }
            _ => {}
        }
        Ok(false)
    }

    fn handle_target_add(&mut self, key: KeyEvent) -> anyhow::Result<bool> {
        match key.code {
            KeyCode::Esc => self.view = View::Targets,
            KeyCode::Tab => self.input_index = (self.input_index + 1) % self.input_fields.len(),
            KeyCode::Left => self.provider_index = self.provider_index.saturating_sub(1),
            KeyCode::Right => {
                self.provider_index = (self.provider_index + 1).min(2);
            }
            KeyCode::Enter => {
                let provider = provider_kind(self.provider_index);
                let spec = spec_for(provider.clone());
                let scope_raw = self.input_fields[0].value.trim();
                let scope = match spec.parse_scope(
                    scope_raw
                        .split_whitespace()
                        .map(|s| s.to_string())
                        .collect(),
                ) {
                    Ok(scope) => {
                        self.validation_message = None;
                        scope
                    }
                    Err(err) => {
                        warn!(error = %err, "Invalid scope for target add");
                        self.validation_message = Some(format!("Scope invalid: {err}"));
                        return Ok(false);
                    }
                };
                let host = optional_text(&self.input_fields[1].value);
                let labels = split_labels(&self.input_fields[2].value);
                let id = target_id(provider.clone(), host.as_deref(), &scope);
                if self.config.targets.iter().any(|t| t.id == id) {
                    warn!(target_id = %id, "Target already exists");
                    self.validation_message = Some("Target already exists.".to_string());
                    self.message = "Target already exists.".to_string();
                    self.view = View::Message;
                    let audit_id = self.audit.record_with_context(
                        "tui.target.add",
                        AuditStatus::Skipped,
                        Some("tui"),
                        AuditContext {
                            provider: Some(provider.as_prefix().to_string()),
                            scope: Some(scope.segments().join("/")),
                            repo_id: None,
                            path: None,
                        },
                        None,
                        Some("target already exists"),
                    )?;
                    self.message = format!("Target already exists. Audit ID: {audit_id}");
                    self.view = View::Message;
                    return Ok(false);
                }
                let scope_label = scope.segments().join("/");
                self.config.targets.push(TargetConfig {
                    id: id.clone(),
                    provider: provider.clone(),
                    scope: scope.clone(),
                    host,
                    labels,
                });
                self.config.save(&self.config_path)?;
                self.validation_message = None;
                info!(target_id = %id, provider = %provider.as_prefix(), "Target added");
                let audit_id = self.audit.record_with_context(
                    "tui.target.add",
                    AuditStatus::Ok,
                    Some("tui"),
                    AuditContext {
                        provider: Some(provider.as_prefix().to_string()),
                        scope: Some(scope_label),
                        repo_id: Some(id.clone()),
                        path: None,
                    },
                    None,
                    None,
                )?;
                self.message = format!("Target added. Audit ID: {audit_id}");
                self.view = View::Message;
            }
            _ => self.handle_text_input(key),
        }
        Ok(false)
    }

    fn handle_target_remove(&mut self, key: KeyEvent) -> anyhow::Result<bool> {
        match key.code {
            KeyCode::Esc => self.view = View::Targets,
            KeyCode::Enter => {
                let id = self.input_fields[0].value.trim().to_string();
                if id.is_empty() {
                    warn!("Target remove attempted without id");
                    self.validation_message = Some("Target id is required.".to_string());
                    self.message = "Target id required.".to_string();
                    self.view = View::Message;
                    return Ok(false);
                }
                let before = self.config.targets.len();
                self.config.targets.retain(|t| t.id != id);
                let after = self.config.targets.len();
                if before == after {
                    warn!(target_id = %id, "Target not found for removal");
                    self.validation_message = Some("Target not found.".to_string());
                    let audit_id = self.audit.record(
                        "tui.target.remove",
                        AuditStatus::Skipped,
                        Some("tui"),
                        None,
                        Some("target not found"),
                    )?;
                    self.message = format!("No target found. Audit ID: {audit_id}");
                } else {
                    self.config.save(&self.config_path)?;
                    self.validation_message = None;
                    info!(target_id = %id, "Target removed");
                    let audit_id = self.audit.record(
                        "tui.target.remove",
                        AuditStatus::Ok,
                        Some("tui"),
                        None,
                        None,
                    )?;
                    self.message = format!("Target removed. Audit ID: {audit_id}");
                }
                self.view = View::Message;
            }
            _ => self.handle_text_input(key),
        }
        Ok(false)
    }

    fn handle_token_menu(&mut self, key: KeyEvent) -> anyhow::Result<bool> {
        match key.code {
            KeyCode::Esc => self.view = View::Main,
            KeyCode::Down => self.token_menu_index = (self.token_menu_index + 1) % 4,
            KeyCode::Up => {
                if self.token_menu_index == 0 {
                    self.token_menu_index = 3;
                } else {
                    self.token_menu_index -= 1;
                }
            }
            KeyCode::Enter => match self.token_menu_index {
                0 => self.view = View::TokenList,
                1 => {
                    self.validation_message = None;
                    self.view = View::TokenSet;
                    self.input_fields = vec![
                        InputField::new("Scope (space-separated)"),
                        InputField::new("Host (optional)"),
                        InputField::with_mask("Token"),
                    ];
                    self.input_index = 0;
                    self.provider_index = 0;
                }
                2 => {
                    self.validation_message = None;
                    self.view = View::TokenValidate;
                    self.input_fields = vec![
                        InputField::new("Scope (space-separated)"),
                        InputField::new("Host (optional)"),
                    ];
                    self.input_index = 0;
                    self.provider_index = 0;
                }
                _ => self.view = View::Main,
            },
            _ => {}
        }
        Ok(false)
    }

    fn handle_token_list(&mut self, key: KeyEvent) -> anyhow::Result<bool> {
        if key.code == KeyCode::Esc {
            self.view = View::TokenMenu
        }
        Ok(false)
    }

    fn handle_token_set(&mut self, key: KeyEvent) -> anyhow::Result<bool> {
        match key.code {
            KeyCode::Esc => self.view = View::TokenMenu,
            KeyCode::Tab => self.input_index = (self.input_index + 1) % self.input_fields.len(),
            KeyCode::Left => self.provider_index = self.provider_index.saturating_sub(1),
            KeyCode::Right => {
                self.provider_index = (self.provider_index + 1).min(2);
            }
            KeyCode::Enter => {
                let provider = provider_kind(self.provider_index);
                let spec = spec_for(provider.clone());
                let scope_raw = self.input_fields[0].value.trim();
                let scope = match spec.parse_scope(
                    scope_raw
                        .split_whitespace()
                        .map(|s| s.to_string())
                        .collect(),
                ) {
                    Ok(scope) => {
                        self.validation_message = None;
                        scope
                    }
                    Err(err) => {
                        warn!(error = %err, "Invalid scope for token set");
                        self.validation_message = Some(format!("Scope invalid: {err}"));
                        return Ok(false);
                    }
                };
                let host = optional_text(&self.input_fields[1].value);
                let host = host_or_default(host.as_deref(), spec.as_ref());
                let account = spec.account_key(&host, &scope)?;
                let token = self.input_fields[2].value.trim().to_string();
                if token.is_empty() {
                    warn!(account = %account, "Token missing in token set");
                    self.validation_message = Some("Token cannot be empty.".to_string());
                    self.message = "Token cannot be empty.".to_string();
                    self.view = View::Message;
                    return Ok(false);
                }
                auth::set_pat(&account, &token)?;
                let runtime_target = mirror_core::model::ProviderTarget {
                    provider: provider.clone(),
                    scope: scope.clone(),
                    host: Some(host.clone()),
                };
                let validity = crate::token_check::check_token_validity(&runtime_target);
                if validity.status != crate::token_check::TokenValidity::Ok {
                    let _ = auth::delete_pat(&account);
                    let message = validity.message(&runtime_target);
                    warn!(account = %account, status = ?validity.status, "Token validity check failed");
                    self.validation_message = Some(message.clone());
                    self.message = message;
                    self.view = View::Message;
                    return Ok(false);
                }
                let validation =
                    self.validate_token(provider.clone(), scope.clone(), Some(host.clone()));
                let validation_message = match &validation {
                    Ok(record) => {
                        self.token_validation
                            .insert(account.clone(), record.clone());
                        record.display()
                    }
                    Err(err) => format!("validation failed: {err}"),
                };
                self.validation_message = None;
                let audit_id = self.audit.record_with_context(
                    "tui.token.set",
                    AuditStatus::Ok,
                    Some("tui"),
                    AuditContext {
                        provider: Some(provider.as_prefix().to_string()),
                        scope: Some(scope.segments().join("/")),
                        repo_id: None,
                        path: None,
                    },
                    None,
                    None,
                )?;
                info!(account = %account, provider = %provider.as_prefix(), "Token stored");
                self.message = format!(
                    "Token stored for {account}. {validation_message}. Audit ID: {audit_id}"
                );
                self.view = View::Message;
            }
            _ => self.handle_text_input(key),
        }
        Ok(false)
    }

    fn handle_token_validate(&mut self, key: KeyEvent) -> anyhow::Result<bool> {
        match key.code {
            KeyCode::Esc => self.view = View::TokenMenu,
            KeyCode::Tab => self.input_index = (self.input_index + 1) % self.input_fields.len(),
            KeyCode::Left => self.provider_index = self.provider_index.saturating_sub(1),
            KeyCode::Right => {
                self.provider_index = (self.provider_index + 1).min(2);
            }
            KeyCode::Enter => {
                let provider = provider_kind(self.provider_index);
                let spec = spec_for(provider.clone());
                let scope_raw = self.input_fields[0].value.trim();
                let scope = match spec.parse_scope(
                    scope_raw
                        .split_whitespace()
                        .map(|s| s.to_string())
                        .collect(),
                ) {
                    Ok(scope) => {
                        self.validation_message = None;
                        scope
                    }
                    Err(err) => {
                        warn!(error = %err, "Invalid scope for token validation");
                        self.validation_message = Some(format!("Scope invalid: {err}"));
                        return Ok(false);
                    }
                };
                let host = optional_text(&self.input_fields[1].value);
                let host = host_or_default(host.as_deref(), spec.as_ref());
                let account = spec.account_key(&host, &scope)?;
                let validation =
                    self.validate_token(provider.clone(), scope.clone(), Some(host.clone()));
                match validation {
                    Ok(record) => {
                        self.token_validation
                            .insert(account.clone(), record.clone());
                        let status = record.display();
                        self.validation_message = None;
                        let audit_status = match record.status {
                            TokenValidationStatus::Ok => AuditStatus::Ok,
                            TokenValidationStatus::MissingScopes(_) => AuditStatus::Failed,
                            TokenValidationStatus::Unsupported => AuditStatus::Ok,
                        };
                        let audit_detail = match &record.status {
                            TokenValidationStatus::Unsupported => {
                                Some("auth-based validation used (scope validation not supported)")
                            }
                            _ => None,
                        };
                        let audit_id = self.audit.record_with_context(
                            "tui.token.validate",
                            audit_status,
                            Some("tui"),
                            AuditContext {
                                provider: Some(provider.as_prefix().to_string()),
                                scope: Some(scope.segments().join("/")),
                                repo_id: None,
                                path: None,
                            },
                            None,
                            audit_detail,
                        )?;
                        info!(account = %account, status = ?record.status, "Token validation completed");
                        self.message = format!("{status}. Audit ID: {audit_id}");
                    }
                    Err(err) => {
                        error!(error = %err, "Token validation failed");
                        self.validation_message = Some(format!("Validation failed: {err}"));
                        let _ = self.audit.record_with_context(
                            "tui.token.validate",
                            AuditStatus::Failed,
                            Some("tui"),
                            AuditContext {
                                provider: Some(provider.as_prefix().to_string()),
                                scope: Some(scope.segments().join("/")),
                                repo_id: None,
                                path: None,
                            },
                            None,
                            Some(&err.to_string()),
                        );
                        self.message = format!("Validation failed: {err}");
                    }
                }
                self.view = View::Message;
            }
            _ => self.handle_text_input(key),
        }
        Ok(false)
    }

    fn handle_message(&mut self, key: KeyEvent) -> anyhow::Result<bool> {
        if key.code == KeyCode::Enter {
            let return_view = self.message_return_view;
            self.message_return_view = View::Main;
            self.view = return_view;
        }
        Ok(false)
    }

    fn handle_audit_log(&mut self, key: KeyEvent) -> anyhow::Result<bool> {
        if self.audit_search_active {
            match key.code {
                KeyCode::Esc => {
                    self.audit_search.clear();
                    self.audit_search_active = false;
                    self.audit_scroll = 0;
                }
                KeyCode::Enter => {
                    self.audit_search_active = false;
                    self.audit_scroll = 0;
                }
                KeyCode::Backspace => {
                    self.audit_search.pop();
                }
                KeyCode::Char(ch) => {
                    if !ch.is_control() {
                        self.audit_search.push(ch);
                    }
                }
                _ => {}
            }
            return Ok(false);
        }

        match key.code {
            KeyCode::Esc => self.view = View::Main,
            KeyCode::Char('f') => {
                self.audit_filter = AuditFilter::Failures;
                self.audit_scroll = 0;
            }
            KeyCode::Char('a') => {
                self.audit_filter = AuditFilter::All;
                self.audit_scroll = 0;
            }
            KeyCode::Char('/') => {
                self.audit_search_active = true;
            }
            KeyCode::Down => {
                self.audit_scroll = self.audit_scroll.saturating_add(1);
            }
            KeyCode::Up => {
                self.audit_scroll = self.audit_scroll.saturating_sub(1);
            }
            KeyCode::PageDown => {
                self.audit_scroll = self.audit_scroll.saturating_add(10);
            }
            KeyCode::PageUp => {
                self.audit_scroll = self.audit_scroll.saturating_sub(10);
            }
            KeyCode::Home => {
                self.audit_scroll = 0;
            }
            _ => {}
        }
        Ok(false)
    }

    fn handle_service(&mut self, key: KeyEvent) -> anyhow::Result<bool> {
        match key.code {
            KeyCode::Esc => self.view = View::Main,
            KeyCode::Char('i') => {
                let exe = std::env::current_exe().context("resolve current executable")?;
                info!("Service install requested");
                let result = mirror_core::service::install_service(&exe);
                match result {
                    Ok(()) => {
                        let audit_id = self.audit.record(
                            "tui.service.install",
                            AuditStatus::Ok,
                            Some("tui"),
                            None,
                            None,
                        )?;
                        self.message = format!("Service installed. Audit ID: {audit_id}");
                    }
                    Err(err) => {
                        error!(error = %err, "Service install failed");
                        let _ = self.audit.record(
                            "tui.service.install",
                            AuditStatus::Failed,
                            Some("tui"),
                            None,
                            Some(&err.to_string()),
                        );
                        self.message = format!("Install failed: {err}");
                    }
                }
                self.view = View::Message;
            }
            KeyCode::Char('u') => {
                info!("Service uninstall requested");
                let result = mirror_core::service::uninstall_service();
                match result {
                    Ok(()) => {
                        let _ = crate::install::remove_marker();
                        let _ = crate::install::remove_manifest();
                        let audit_id = self.audit.record(
                            "tui.service.uninstall",
                            AuditStatus::Ok,
                            Some("tui"),
                            None,
                            None,
                        )?;
                        self.message = format!("Service uninstalled. Audit ID: {audit_id}");
                    }
                    Err(err) => {
                        error!(error = %err, "Service uninstall failed");
                        let _ = self.audit.record(
                            "tui.service.uninstall",
                            AuditStatus::Failed,
                            Some("tui"),
                            None,
                            Some(&err.to_string()),
                        );
                        self.message = format!("Uninstall failed: {err}");
                    }
                }
                self.view = View::Message;
            }
            _ => {}
        }
        Ok(false)
    }

    fn handle_install_progress(&mut self, _key: KeyEvent) -> anyhow::Result<bool> {
        Ok(false)
    }

    fn handle_update_prompt(&mut self, key: KeyEvent) -> anyhow::Result<bool> {
        match key.code {
            KeyCode::Char('y') | KeyCode::Char('Y') => {
                if let Some(check) = self.update_prompt.take() {
                    self.start_update_apply(check)?;
                } else {
                    self.message = "No update available.".to_string();
                    self.message_return_view = self.update_return_view;
                    self.view = View::Message;
                }
            }
            KeyCode::Char('n') | KeyCode::Char('N') | KeyCode::Esc => {
                self.update_prompt = None;
                self.view = self.update_return_view;
            }
            _ => {}
        }
        Ok(false)
    }

    fn handle_update_progress(&mut self, _key: KeyEvent) -> anyhow::Result<bool> {
        Ok(false)
    }

    fn handle_install_status(&mut self, key: KeyEvent) -> anyhow::Result<bool> {
        match key.code {
            KeyCode::Enter | KeyCode::Esc => {
                self.install_scroll = 0;
                self.view = View::Install;
            }
            KeyCode::Down => self.install_scroll = self.install_scroll.saturating_add(1),
            KeyCode::Up => self.install_scroll = self.install_scroll.saturating_sub(1),
            KeyCode::PageDown => self.install_scroll = self.install_scroll.saturating_add(10),
            KeyCode::PageUp => self.install_scroll = self.install_scroll.saturating_sub(10),
            KeyCode::Home => self.install_scroll = 0,
            _ => {}
        }
        Ok(false)
    }

    fn handle_sync_status(&mut self, key: KeyEvent) -> anyhow::Result<bool> {
        match key.code {
            KeyCode::Enter | KeyCode::Esc => self.view = View::Dashboard,
            _ => {}
        }
        Ok(false)
    }

    fn poll_install_events(&mut self) -> anyhow::Result<()> {
        let Some(rx) = self.install_rx.take() else {
            return Ok(());
        };
        let mut done = false;
        while let Ok(event) = rx.try_recv() {
            match event {
                InstallEvent::Progress(progress) => {
                    let state = self
                        .install_progress
                        .get_or_insert_with(|| InstallProgressState::new(progress.total));
                    state.update(progress);
                }
                InstallEvent::Done(result) => {
                    self.release_install_guard();
                    match result {
                        Ok(report) => {
                            info!("Install completed");
                            let audit_id = self.audit.record(
                                "tui.install",
                                AuditStatus::Ok,
                                Some("tui"),
                                None,
                                None,
                            )?;
                            self.message = format!(
                                "{}\n{}\n{}\nAudit ID: {audit_id}",
                                report.install, report.service, report.path
                            );
                        }
                        Err(err) => {
                            error!(error = %err, "Install failed");
                            let _ = self.audit.record(
                                "tui.install",
                                AuditStatus::Failed,
                                Some("tui"),
                                None,
                                Some(&err),
                            );
                            self.message = format!("Install failed: {err}");
                        }
                    }
                    self.view = View::Message;
                    done = true;
                }
            }
        }
        if !done {
            self.install_rx = Some(rx);
        }
        Ok(())
    }

    fn poll_repo_status_events(&mut self) -> anyhow::Result<()> {
        let Some(rx) = self.repo_status_rx.take() else {
            return Ok(());
        };
        let mut done = false;
        while let Ok(result) = rx.try_recv() {
            self.repo_status_refreshing = false;
            match result {
                Ok(statuses) => {
                    info!(count = statuses.len(), "Repo status refreshed");
                    self.update_repo_status_cache(statuses);
                    let refreshed = self
                        .repo_status_last_refresh
                        .map(epoch_to_label)
                        .unwrap_or_else(|| "unknown".to_string());
                    self.repo_overview_message =
                        Some(format!("Repo status refreshed at {refreshed}"));
                }
                Err(err) => {
                    warn!(error = %err, "Repo status refresh failed");
                    self.repo_overview_message = Some(format!("Repo status refresh failed: {err}"));
                }
            }
            done = true;
        }
        if !done {
            self.repo_status_rx = Some(rx);
        }
        Ok(())
    }

    fn poll_sync_events(&mut self) -> anyhow::Result<()> {
        let Some(rx) = self.sync_rx.take() else {
            return Ok(());
        };
        let mut done = false;
        while let Ok(result) = rx.try_recv() {
            self.sync_running = false;
            match result {
                Ok(summary) => {
                    info!(
                        cloned = summary.cloned,
                        fast_forwarded = summary.fast_forwarded,
                        up_to_date = summary.up_to_date,
                        dirty = summary.dirty,
                        diverged = summary.diverged,
                        failed = summary.failed,
                        missing_archived = summary.missing_archived,
                        missing_removed = summary.missing_removed,
                        missing_skipped = summary.missing_skipped,
                        "Sync completed"
                    );
                    self.message = format!(
                        "Sync completed. cloned={} ff={} up={} dirty={} div={} fail={} missA={} missR={} missS={}",
                        summary.cloned,
                        summary.fast_forwarded,
                        summary.up_to_date,
                        summary.dirty,
                        summary.diverged,
                        summary.failed,
                        summary.missing_archived,
                        summary.missing_removed,
                        summary.missing_skipped
                    );
                }
                Err(err) => {
                    error!(error = %err, "Sync failed");
                    self.message = format!("Sync failed: {err}");
                }
            }
            self.view = View::Message;
            done = true;
        }
        if !done {
            self.sync_rx = Some(rx);
        }
        Ok(())
    }

    fn poll_update_events(&mut self) -> anyhow::Result<()> {
        let Some(rx) = self.update_rx.take() else {
            return Ok(());
        };
        let mut done = false;
        while let Ok(event) = rx.try_recv() {
            match event {
                UpdateEvent::Progress(message) => {
                    let state = self
                        .update_progress
                        .get_or_insert_with(UpdateProgressState::new);
                    state.messages.push(message);
                }
                UpdateEvent::Checked(result) => {
                    match result {
                        Ok(check) => {
                            info!(
                                current = %check.current,
                                latest = %check.latest,
                                is_newer = check.is_newer,
                                "Update check completed"
                            );
                            if !check.is_newer {
                                self.message = format!("Up to date ({})", check.current);
                                self.message_return_view = self.update_return_view;
                                self.view = View::Message;
                            } else if check.asset.is_none() {
                                self.message =
                                    "Update available but no asset found for this platform."
                                        .to_string();
                                self.message_return_view = self.update_return_view;
                                self.view = View::Message;
                            } else {
                                self.message = format!(
                                    "Update available: {} -> {}\nApply update? (y/n)",
                                    check.current, check.latest
                                );
                                self.update_prompt = Some(check);
                                self.view = View::UpdatePrompt;
                            }
                        }
                        Err(err) => {
                            warn!(error = %err, "Update check failed");
                            self.message = format!("Update check failed: {err}");
                            self.message_return_view = self.update_return_view;
                            self.view = View::Message;
                        }
                    }
                    done = true;
                }
                UpdateEvent::Done(result) => {
                    match result {
                        Ok(report) => {
                            info!("Update applied");
                            self.message =
                                format!("{}\n{}\n{}", report.install, report.service, report.path);
                            self.restart_requested = true;
                        }
                        Err(err) => {
                            error!(error = %err, "Update apply failed");
                            self.message = format!("Update failed: {err}");
                        }
                    }
                    self.message_return_view = self.update_return_view;
                    self.view = View::Message;
                    done = true;
                }
            }
        }
        if !done {
            self.update_rx = Some(rx);
        }
        Ok(())
    }

    fn enter_repo_overview(&mut self) -> anyhow::Result<()> {
        info!("Entered repo overview view");
        self.view = View::RepoOverview;
        self.repo_overview_message = None;
        self.repo_overview_selected = 0;
        self.repo_overview_scroll = 0;
        self.load_repo_status_from_cache();
        if self.repo_status_is_stale() {
            self.start_repo_status_refresh()?;
        }
        Ok(())
    }

    fn load_repo_status_from_cache(&mut self) {
        if let Ok(cache_path) = default_cache_path()
            && let Ok(cache) = RepoCache::load(&cache_path)
        {
            self.update_repo_status_cache(cache.repo_status);
        }
    }

    fn update_repo_status_cache(&mut self, statuses: HashMap<String, RepoLocalStatus>) {
        self.repo_status_last_refresh = statuses.values().map(|s| s.checked_at).max();
        self.repo_status = statuses;
    }

    fn current_overview_rows(&self) -> Vec<repo_overview::OverviewRow> {
        let cache_path = default_cache_path().ok();
        let cache = cache_path
            .as_ref()
            .and_then(|path| RepoCache::load(path).ok())
            .unwrap_or_default();
        let root = self.config.root.as_deref();
        if cache.repos.is_empty() {
            vec![repo_overview::OverviewRow {
                id: "empty".to_string(),
                depth: 0,
                name: "No repos in cache yet.".to_string(),
                branch: None,
                pulled: None,
                ahead_behind: None,
                touched: None,
                is_leaf: true,
            }]
        } else {
            let tree = repo_overview::build_repo_tree(cache.repos.iter(), root);
            repo_overview::render_repo_tree_rows(&tree, &cache, &self.repo_status)
        }
    }

    fn visible_overview_rows(
        &self,
        rows: &[repo_overview::OverviewRow],
    ) -> Vec<repo_overview::OverviewRow> {
        let mut visible = Vec::new();
        let mut stack: Vec<(usize, bool)> = Vec::new();
        for row in rows {
            while let Some((depth, _)) = stack.last() {
                if row.depth <= *depth {
                    stack.pop();
                } else {
                    break;
                }
            }
            let hidden = stack.iter().any(|(_, collapsed)| *collapsed);
            if !hidden {
                visible.push(row.clone());
            }
            if !row.is_leaf {
                let collapsed = self.repo_overview_collapsed.contains(&row.id);
                if !hidden {
                    stack.push((row.depth, collapsed));
                } else if collapsed {
                    stack.push((row.depth, true));
                }
            }
        }
        visible
    }

    fn repo_status_is_stale(&self) -> bool {
        match self.repo_status_last_refresh {
            Some(timestamp) => {
                current_epoch_seconds().saturating_sub(timestamp) > REPO_STATUS_TTL_SECS
            }
            None => true,
        }
    }

    fn start_repo_status_refresh(&mut self) -> anyhow::Result<()> {
        if self.repo_status_refreshing {
            debug!("Repo status refresh already running");
            return Ok(());
        }
        let cache_path = default_cache_path()?;
        info!(path = %cache_path.display(), "Starting repo status refresh");
        let (tx, rx) = mpsc::channel::<Result<HashMap<String, RepoLocalStatus>, String>>();
        self.repo_status_rx = Some(rx);
        self.repo_status_refreshing = true;
        self.repo_overview_message = Some("Refreshing repo status...".to_string());
        thread::spawn(move || {
            let result =
                repo_overview::refresh_repo_status(&cache_path).map_err(|err| err.to_string());
            let _ = tx.send(result);
        });
        Ok(())
    }

    fn start_update_check(&mut self, return_view: View) -> anyhow::Result<()> {
        info!(return_view = ?return_view, "Starting update check");
        self.update_return_view = return_view;
        self.update_progress = Some(UpdateProgressState::new());
        self.view = View::UpdateProgress;
        let (tx, rx) = mpsc::channel::<UpdateEvent>();
        self.update_rx = Some(rx);
        thread::spawn(move || {
            let result = update::check_for_update(None).map_err(|err| err.to_string());
            let _ = tx.send(UpdateEvent::Checked(result));
        });
        Ok(())
    }

    fn start_update_apply(&mut self, check: update::UpdateCheck) -> anyhow::Result<()> {
        info!(current = %check.current, latest = %check.latest, "Applying update");
        self.update_progress = Some(UpdateProgressState {
            messages: vec!["Starting update...".to_string()],
        });
        self.view = View::UpdateProgress;
        let (tx, rx) = mpsc::channel::<UpdateEvent>();
        self.update_rx = Some(rx);
        thread::spawn(move || {
            let result = update::apply_update_with_progress(
                &check,
                Some(&|message| {
                    let _ = tx.send(UpdateEvent::Progress(message.to_string()));
                }),
            )
            .map_err(|err| err.to_string());
            let _ = tx.send(UpdateEvent::Done(result));
        });
        Ok(())
    }

    fn start_sync_run(&mut self, force_refresh_all: bool) -> anyhow::Result<()> {
        if self.sync_running {
            warn!("Sync already running");
            self.message = "Sync already running.".to_string();
            self.view = View::Message;
            return Ok(());
        }
        let root = match self.config.root.clone() {
            Some(root) => root,
            None => {
                warn!("Sync requested without configured root");
                self.message = "Config missing root; run config init.".to_string();
                self.view = View::Message;
                return Ok(());
            }
        };
        if self.config.targets.is_empty() {
            warn!("Sync requested without configured targets");
            self.message = "No targets configured.".to_string();
            self.view = View::Message;
            return Ok(());
        }

        let lock_path = default_lock_path()?;
        let lock = match LockFile::try_acquire(&lock_path)? {
            Some(lock) => lock,
            None => {
                warn!(path = %lock_path.display(), "Sync lock already held");
                self.message = "Sync already running (lock held).".to_string();
                self.view = View::Message;
                return Ok(());
            }
        };

        let targets = self.config.targets.clone();
        let audit = self.audit.clone();
        let cache_path = default_cache_path()?;
        let (tx, rx) = mpsc::channel::<Result<SyncSummary, String>>();
        self.sync_rx = Some(rx);
        self.sync_running = true;
        self.view = View::SyncStatus;
        info!(
            targets = targets.len(),
            root = %root.display(),
            force_refresh_all,
            "Starting sync"
        );

        thread::spawn(move || {
            let _lock = lock;
            let result = run_tui_sync(&targets, &root, &cache_path, &audit, force_refresh_all);
            if let Err(err) = &result {
                let _ = audit.record(
                    "tui.sync.finish",
                    AuditStatus::Failed,
                    Some("tui"),
                    None,
                    Some(&err.to_string()),
                );
            }
            let _ = tx.send(result.map_err(|err| err.to_string()));
        });

        Ok(())
    }

    fn handle_text_input(&mut self, key: KeyEvent) {
        if self.input_fields.is_empty() {
            return;
        }
        let field = &mut self.input_fields[self.input_index];
        match key.code {
            KeyCode::Char('c') if key.modifiers.contains(KeyModifiers::CONTROL) => {
                field.value.clear();
                self.validation_message = None;
            }
            KeyCode::Backspace => {
                field.pop();
                self.validation_message = None;
            }
            KeyCode::Char(ch) => {
                field.push(ch);
                self.validation_message = None;
            }
            _ => {}
        }
    }

    fn audit_log_path(&self) -> std::path::PathBuf {
        let base_dir = self.audit.base_dir();
        let date = time::OffsetDateTime::now_utc()
            .format(&time::format_description::parse("[year][month][day]").unwrap())
            .unwrap();
        base_dir.join(format!("audit-{date}.jsonl"))
    }

    fn form_context(&self) -> Option<&'static str> {
        match self.view {
            View::TargetAdd => Some("Context: Add a provider target"),
            View::TargetRemove => Some("Context: Remove a target by id"),
            View::TokenSet => Some("Context: Store a token for a provider scope"),
            View::TokenValidate => Some("Context: Validate token scopes"),
            _ => None,
        }
    }

    fn form_hint(&self) -> Option<&'static str> {
        match self.view {
            View::TargetAdd => {
                let provider = provider_kind(self.provider_index);
                Some(provider_scope_hint(provider))
            }
            View::TargetRemove => Some("Find target ids on the Targets screen"),
            View::TokenSet | View::TokenValidate => {
                let provider = provider_kind(self.provider_index);
                Some(provider_scope_hint_with_host(provider))
            }
            _ => None,
        }
    }

    fn token_entries(&self) -> Vec<TokenEntry> {
        let mut entries = Vec::new();
        let mut seen = HashSet::new();
        for target in &self.config.targets {
            let spec = spec_for(target.provider.clone());
            let host = host_or_default(target.host.as_deref(), spec.as_ref());
            let account = match spec.account_key(&host, &target.scope) {
                Ok(account) => account,
                Err(_) => continue,
            };
            if !seen.insert(account.clone()) {
                continue;
            }
            let present = auth::get_pat(&account).is_ok();
            let validation = self.token_validation.get(&account).cloned();
            entries.push(TokenEntry {
                account,
                provider: target.provider.clone(),
                scope: target.scope.segments().join("/"),
                host,
                present,
                validation,
            });
        }
        entries
    }

    fn validate_token(
        &self,
        provider: ProviderKind,
        scope: mirror_core::model::ProviderScope,
        host: Option<String>,
    ) -> anyhow::Result<TokenValidation> {
        let runtime_target = mirror_core::model::ProviderTarget {
            provider: provider.clone(),
            scope: scope.clone(),
            host,
        };
        let registry = ProviderRegistry::new();
        let adapter = registry.provider(provider.clone())?;
        let scopes = adapter.token_scopes(&runtime_target)?;
        let help = pat_help(provider.clone());
        let status = match scopes {
            Some(scopes) => {
                let missing: Vec<&str> = help
                    .scopes
                    .iter()
                    .copied()
                    .filter(|required| !scopes.iter().any(|s| s == required))
                    .collect();
                if missing.is_empty() {
                    TokenValidationStatus::Ok
                } else {
                    TokenValidationStatus::MissingScopes(
                        missing.iter().map(|s| s.to_string()).collect(),
                    )
                }
            }
            None => {
                let token_check_result = crate::token_check::check_token_validity(&runtime_target);
                crate::token_check::ensure_token_valid(&token_check_result, &runtime_target)
                    .context(
                        "Auth-based token validation failed; verify your token is valid and not expired",
                    )?;
                TokenValidationStatus::Unsupported
            }
        };
        Ok(TokenValidation {
            status,
            at: validation_timestamp(),
        })
    }

    fn dashboard_stats(&self) -> DashboardStats {
        let cache_path = default_cache_path().ok();
        let cache = cache_path
            .as_ref()
            .and_then(|path| mirror_core::cache::RepoCache::load(path).ok());
        let audit_entries = self.audit_log_count();
        let now = current_epoch_seconds();
        let mut healthy = 0;
        let mut backoff = 0;
        let mut no_success = 0;
        let mut last_sync: Option<String> = None;
        let mut targets = Vec::new();

        for target in &self.config.targets {
            let id = target.id.clone();
            let last_success = cache
                .as_ref()
                .and_then(|c| c.target_last_success.get(&id).copied());
            let backoff_until = cache
                .as_ref()
                .and_then(|c| c.target_backoff_until.get(&id).copied());
            let status = if let Some(until) = backoff_until {
                if until > now {
                    backoff += 1;
                    "backoff"
                } else {
                    healthy += 1;
                    "ok"
                }
            } else if last_success.is_some() {
                healthy += 1;
                "ok"
            } else {
                no_success += 1;
                "unknown"
            };

            let last_success_label = last_success
                .map(epoch_to_label)
                .unwrap_or_else(|| "none".to_string());
            if last_sync.is_none() {
                last_sync = last_success.map(epoch_to_label);
            }

            targets.push(DashboardTarget {
                id,
                status: status.to_string(),
                last_success: last_success_label,
            });
        }

        DashboardStats {
            total_targets: self.config.targets.len(),
            healthy_targets: healthy,
            backoff_targets: backoff,
            no_success_targets: no_success,
            last_sync,
            audit_entries,
            targets,
        }
    }

    fn sync_status_lines(&self) -> anyhow::Result<Vec<Line<'_>>> {
        let cache_path = default_cache_path()?;
        let cache = RepoCache::load(&cache_path).unwrap_or_default();
        let empty_summary = SyncSummarySnapshot::default();
        let mut lines = Vec::new();
        for target in &self.config.targets {
            let label = format!(
                "{} | {}",
                target.provider.as_prefix(),
                target.scope.segments().join("/")
            );
            let status = cache.target_sync_status.get(&target.id);
            let state = status
                .map(|s| if s.in_progress { "running" } else { "idle" })
                .unwrap_or("idle");
            let action = status
                .and_then(|s| s.last_action.as_deref())
                .unwrap_or("unknown");
            let repo = status.and_then(|s| s.last_repo.as_deref()).unwrap_or("-");
            let updated = status
                .map(|s| epoch_to_label(s.last_updated))
                .unwrap_or_else(|| "unknown".to_string());
            let summary = status.map(|s| &s.summary).unwrap_or(&empty_summary);
            let total = status.map(|s| s.total_repos).unwrap_or(0);
            let processed = status.map(|s| s.processed_repos).unwrap_or(0);
            let bar = progress_bar(processed.min(total), total, 20);
            let error = last_sync_error(&self.audit, &target.id).unwrap_or_default();
            lines.push(Line::from(Span::raw(format!(
                "{} | {} | {} | {} | {}",
                label, state, action, repo, updated
            ))));
            lines.push(Line::from(Span::raw(format!(
                "progress: {}/{} {}",
                processed, total, bar
            ))));
            lines.push(Line::from(Span::raw(format!(
                "counts: cl={} ff={} up={} dirty={} div={} fail={} missA={} missR={} missS={}",
                summary.cloned,
                summary.fast_forwarded,
                summary.up_to_date,
                summary.dirty,
                summary.diverged,
                summary.failed,
                summary.missing_archived,
                summary.missing_removed,
                summary.missing_skipped
            ))));
            if !error.is_empty() {
                lines.push(Line::from(Span::raw(format!("last error: {error}"))));
            }
            lines.push(Line::from(Span::raw("")));
        }
        Ok(lines)
    }

    fn audit_log_count(&self) -> usize {
        let path = self.audit_log_path();
        if let Ok(contents) = std::fs::read_to_string(path) {
            return contents.lines().count();
        }
        0
    }
}

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
enum AuditFilter {
    All,
    Failures,
}

#[derive(Clone)]
struct TokenEntry {
    account: String,
    provider: ProviderKind,
    scope: String,
    host: String,
    present: bool,
    validation: Option<TokenValidation>,
}

#[derive(Clone)]
struct TokenValidation {
    status: TokenValidationStatus,
    at: String,
}

impl TokenValidation {
    fn display(&self) -> String {
        match &self.status {
            TokenValidationStatus::Ok => format!("verified ok at {}", self.at),
            TokenValidationStatus::MissingScopes(scopes) => {
                format!("missing scopes ({}) at {}", scopes.join(", "), self.at)
            }
            TokenValidationStatus::Unsupported => format!(
                "token valid (scope validation not supported) at {}",
                self.at
            ),
        }
    }
}

#[derive(Clone, Debug)]
enum TokenValidationStatus {
    Ok,
    MissingScopes(Vec<String>),
    Unsupported,
}

struct DashboardStats {
    total_targets: usize,
    healthy_targets: usize,
    backoff_targets: usize,
    no_success_targets: usize,
    last_sync: Option<String>,
    audit_entries: usize,
    targets: Vec<DashboardTarget>,
}

struct DashboardTarget {
    id: String,
    status: String,
    last_success: String,
}

fn provider_kind(index: usize) -> ProviderKind {
    match index {
        1 => ProviderKind::GitHub,
        2 => ProviderKind::GitLab,
        _ => ProviderKind::AzureDevOps,
    }
}

fn provider_label(index: usize) -> &'static str {
    match index {
        1 => "github",
        2 => "gitlab",
        _ => "azure-devops",
    }
}

fn provider_selector_line(index: usize) -> String {
    let labels = ["azure-devops", "github", "gitlab"];
    let mut parts = Vec::with_capacity(labels.len());
    for (idx, label) in labels.iter().enumerate() {
        if idx == index {
            parts.push(format!("[{label}]"));
        } else {
            parts.push(label.to_string());
        }
    }
    format!("Provider: {}", parts.join(" "))
}

fn provider_scope_hint(provider: ProviderKind) -> &'static str {
    match provider {
        ProviderKind::AzureDevOps => "Scope uses space-separated segments (org project)",
        ProviderKind::GitHub => "Scope uses a single segment (org or user)",
        ProviderKind::GitLab => "Scope uses space-separated group/subgroup segments",
    }
}

fn provider_scope_hint_with_host(provider: ProviderKind) -> &'static str {
    match provider {
        ProviderKind::AzureDevOps => {
            "Scope uses space-separated segments (org project). Host optional."
        }
        ProviderKind::GitHub => "Scope uses a single segment (org or user). Host optional.",
        ProviderKind::GitLab => {
            "Scope uses space-separated group/subgroup segments. Host optional."
        }
    }
}

fn slice_with_scroll(lines: &[String], scroll: usize, height: usize) -> Vec<String> {
    if lines.is_empty() || height == 0 {
        return Vec::new();
    }
    let start = scroll.min(lines.len());
    let end = (start + height).min(lines.len());
    lines[start..end].to_vec()
}

fn clamp_index(index: usize, len: usize) -> usize {
    if len == 0 { 0 } else { index.min(len - 1) }
}

fn adjust_scroll(selected: usize, scroll: usize, height: usize, len: usize) -> usize {
    if len == 0 || height == 0 {
        return 0;
    }
    if selected < scroll {
        return selected;
    }
    let last_visible = scroll.saturating_add(height).saturating_sub(1);
    if selected > last_visible {
        let new_scroll = selected.saturating_sub(height - 1);
        return new_scroll.min(len.saturating_sub(1));
    }
    scroll
}

fn name_column_width(total_width: usize, compact: bool) -> usize {
    let fixed = if compact {
        2 + 12 + 2 + 10 // separators + columns
    } else {
        2 + 12 + 2 + 16 + 2 + 10 + 2 + 16
    };
    let available = total_width.saturating_sub(fixed);
    available.max(20)
}

fn format_overview_header(name_width: usize, compact: bool) -> String {
    if compact {
        format!(
            "{:<name_width$} | {:<12} | {:<10}",
            "name",
            "branch",
            "ahead/behind",
            name_width = name_width
        )
    } else {
        format!(
            "{:<name_width$} | {:<12} | {:<16} | {:<10} | {:<16}",
            "name",
            "branch",
            "pulled",
            "ahead/behind",
            "touched",
            name_width = name_width
        )
    }
}

fn format_overview_row(
    row: &repo_overview::OverviewRow,
    name_width: usize,
    compact: bool,
) -> String {
    let name = truncate_with_ellipsis(&row.name, name_width);
    let branch = row.branch.as_deref().unwrap_or("-");
    let ahead = row.ahead_behind.as_deref().unwrap_or("-");
    if compact {
        format!(
            "{:<name_width$} | {:<12} | {:<10}",
            name,
            branch,
            ahead,
            name_width = name_width
        )
    } else {
        let pulled = row.pulled.as_deref().unwrap_or("-");
        let touched = row.touched.as_deref().unwrap_or("-");
        format!(
            "{:<name_width$} | {:<12} | {:<16} | {:<10} | {:<16}",
            name,
            branch,
            pulled,
            ahead,
            touched,
            name_width = name_width
        )
    }
}

fn truncate_with_ellipsis(value: &str, max: usize) -> String {
    if max == 0 {
        return String::new();
    }
    if value.len() <= max {
        return value.to_string();
    }
    if max <= 1 {
        return "…".to_string();
    }
    let mut truncated = value.chars().take(max - 1).collect::<String>();
    truncated.push('…');
    truncated
}

fn optional_text(value: &str) -> Option<String> {
    let trimmed = value.trim();
    if trimmed.is_empty() {
        None
    } else {
        Some(trimmed.to_string())
    }
}

fn split_labels(value: &str) -> Vec<String> {
    value
        .split(',')
        .map(|label| label.trim())
        .filter(|label| !label.is_empty())
        .map(|label| label.to_string())
        .collect()
}

enum InstallEvent {
    Progress(crate::install::InstallProgress),
    Done(Result<crate::install::InstallReport, String>),
}

enum UpdateEvent {
    Progress(String),
    Checked(Result<update::UpdateCheck, String>),
    Done(Result<crate::install::InstallReport, String>),
}

struct InstallProgressState {
    current: usize,
    total: usize,
    messages: Vec<String>,
}

impl InstallProgressState {
    fn new(total: usize) -> Self {
        Self {
            current: 0,
            total,
            messages: Vec::new(),
        }
    }

    fn update(&mut self, progress: crate::install::InstallProgress) {
        self.current = progress.step;
        self.total = progress.total.max(1);
        self.messages.push(progress.message);
    }
}

struct UpdateProgressState {
    messages: Vec<String>,
}

impl UpdateProgressState {
    fn new() -> Self {
        Self {
            messages: vec!["Checking for updates...".to_string()],
        }
    }
}

fn progress_bar(step: usize, total: usize, width: usize) -> String {
    if total == 0 || width == 0 {
        return "[]".to_string();
    }
    let filled = ((step as f32 / total as f32) * width as f32).round() as usize;
    let filled = filled.min(width);
    let empty = width.saturating_sub(filled);
    format!("[{}{}]", "#".repeat(filled), "-".repeat(empty))
}

fn dashboard_footer_text() -> &'static str {
    "t: toggle targets | s: sync status | r: sync now | f: force refresh all | u: check updates | Esc: back"
}

fn read_audit_lines(path: &std::path::Path, filter: AuditFilter) -> anyhow::Result<Vec<String>> {
    if !path.exists() {
        return Ok(vec!["No audit log found for today.".to_string()]);
    }
    let contents = std::fs::read_to_string(path)?;
    let mut lines = Vec::new();
    for line in contents.lines().rev().take(100) {
        if filter == AuditFilter::Failures && !line.contains("\"status\":\"failed\"") {
            continue;
        }
        lines.push(line.to_string());
    }
    Ok(lines)
}

fn run_tui_sync(
    targets: &[TargetConfig],
    root: &std::path::Path,
    cache_path: &std::path::Path,
    audit: &AuditLogger,
    force_refresh_all: bool,
) -> anyhow::Result<SyncSummary> {
    let audit_id = audit.record(
        "tui.sync.start",
        AuditStatus::Ok,
        Some("tui"),
        Some(serde_json::json!({ "force_refresh_all": force_refresh_all })),
        None,
    )?;
    let _ = audit_id;
    let registry = ProviderRegistry::new();
    let mut total = SyncSummary::default();

    for target in targets {
        let provider_kind = target.provider.clone();
        let provider = registry.provider(provider_kind.clone())?;
        let runtime_target = ProviderTarget {
            provider: provider_kind,
            scope: target.scope.clone(),
            host: target.host.clone(),
        };
        let filter = |repo: &RemoteRepo| !repo.archived;
        let options = RunSyncOptions {
            missing_policy: MissingRemotePolicy::Skip,
            missing_decider: None,
            repo_filter: Some(&filter),
            progress: None,
            jobs: 1,
            detect_missing: true,
            refresh: force_refresh_all,
            verify: false,
        };
        let summary = match run_sync_filtered(
            provider.as_ref(),
            &runtime_target,
            root,
            cache_path,
            options,
        ) {
            Ok(summary) => summary,
            Err(err) => {
                let context = AuditContext {
                    provider: Some(target.provider.as_prefix().to_string()),
                    scope: Some(target.scope.segments().join("/")),
                    repo_id: Some(target.id.clone()),
                    path: None,
                };
                let _ = audit.record_with_context(
                    "tui.sync.target",
                    AuditStatus::Failed,
                    Some("tui"),
                    context,
                    None,
                    Some(&err.to_string()),
                );
                return Err(err);
            }
        };
        let details = serde_json::json!({
            "cloned": summary.cloned,
            "fast_forwarded": summary.fast_forwarded,
            "up_to_date": summary.up_to_date,
            "dirty": summary.dirty,
            "diverged": summary.diverged,
            "failed": summary.failed,
            "missing_archived": summary.missing_archived,
            "missing_removed": summary.missing_removed,
            "missing_skipped": summary.missing_skipped,
        });
        let context = AuditContext {
            provider: Some(target.provider.as_prefix().to_string()),
            scope: Some(target.scope.segments().join("/")),
            repo_id: Some(target.id.clone()),
            path: None,
        };
        let _ = audit.record_with_context(
            "tui.sync.target",
            AuditStatus::Ok,
            Some("tui"),
            context,
            Some(details),
            None,
        );
        accumulate_summary(&mut total, summary);
    }

    let details = serde_json::json!({
        "cloned": total.cloned,
        "fast_forwarded": total.fast_forwarded,
        "up_to_date": total.up_to_date,
        "dirty": total.dirty,
        "diverged": total.diverged,
        "failed": total.failed,
        "missing_archived": total.missing_archived,
        "missing_removed": total.missing_removed,
        "missing_skipped": total.missing_skipped,
        "force_refresh_all": force_refresh_all,
    });
    let _ = audit.record(
        "tui.sync.finish",
        AuditStatus::Ok,
        Some("tui"),
        Some(details),
        None,
    );
    Ok(total)
}

fn accumulate_summary(total: &mut SyncSummary, next: SyncSummary) {
    total.cloned += next.cloned;
    total.fast_forwarded += next.fast_forwarded;
    total.up_to_date += next.up_to_date;
    total.dirty += next.dirty;
    total.diverged += next.diverged;
    total.failed += next.failed;
    total.missing_archived += next.missing_archived;
    total.missing_removed += next.missing_removed;
    total.missing_skipped += next.missing_skipped;
}

fn last_sync_error(audit: &AuditLogger, target_id: &str) -> anyhow::Result<String> {
    let base_dir = audit.base_dir();
    let date = time::OffsetDateTime::now_utc()
        .format(&time::format_description::parse("[year][month][day]").unwrap())
        .unwrap();
    let path = base_dir.join(format!("audit-{date}.jsonl"));
    if !path.exists() {
        return Ok(String::new());
    }
    let contents = std::fs::read_to_string(&path)?;
    for line in contents.lines().rev().take(200) {
        if !line.contains("\"status\":\"failed\"") {
            continue;
        }
        if !line.contains("\"event\":\"sync.target\"")
            && !line.contains("\"event\":\"daemon.sync.target\"")
        {
            continue;
        }
        if !line.contains(&format!("\"repo_id\":\"{target_id}\"")) {
            continue;
        }
        if let Ok(value) = serde_json::from_str::<serde_json::Value>(line)
            && let Some(error) = value.get("error").and_then(|v| v.as_str())
        {
            return Ok(error.to_string());
        }
    }
    Ok(String::new())
}

fn validation_timestamp() -> String {
    time::OffsetDateTime::now_utc()
        .format(
            &time::format_description::parse("[year]-[month]-[day] [hour]:[minute]:[second]")
                .unwrap(),
        )
        .unwrap_or_else(|_| "unknown".to_string())
}

fn epoch_to_label(epoch: u64) -> String {
    let ts = time::OffsetDateTime::from_unix_timestamp(epoch as i64)
        .unwrap_or_else(|_| time::OffsetDateTime::now_utc());
    ts.format(&time::format_description::parse("[year]-[month]-[day] [hour]:[minute]").unwrap())
        .unwrap_or_else(|_| "unknown".to_string())
}

fn format_delayed_start(delay: Option<u64>) -> String {
    match delay.filter(|value| *value > 0) {
        Some(value) => format!("{value}s"),
        None => "none".to_string(),
    }
}

fn install_action_from_status(status: Option<&crate::install::InstallStatus>) -> InstallAction {
    let Some(status) = status else {
        return InstallAction::Install;
    };
    let running_from_install = status
        .installed_path
        .as_ref()
        .and_then(|path| std::env::current_exe().ok().map(|exe| exe == *path))
        .unwrap_or(false);
    install_action_for_versions(
        status.installed,
        status.installed_version.as_deref(),
        env!("CARGO_PKG_VERSION"),
        running_from_install,
    )
}

fn install_action_for_versions(
    installed: bool,
    installed_version: Option<&str>,
    current_version: &str,
    running_from_install: bool,
) -> InstallAction {
    if !installed {
        return InstallAction::Install;
    }
    if !running_from_install
        && let (Ok(current), Some(installed)) = (
            Version::parse(current_version),
            installed_version.and_then(|value| Version::parse(value).ok()),
        )
        && current > installed
    {
        return InstallAction::Update;
    }
    InstallAction::Reinstall
}

fn install_state_from_status(
    status: Option<&crate::install::InstallStatus>,
    action: InstallAction,
) -> InstallState {
    let Some(status) = status else {
        return InstallState::Unknown;
    };
    if !status.installed {
        return InstallState::NotInstalled;
    }
    if matches!(action, InstallAction::Update) {
        return InstallState::UpdateReady;
    }
    InstallState::Installed
}

fn current_epoch_seconds() -> u64 {
    use std::time::{SystemTime, UNIX_EPOCH};
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap_or_default()
        .as_secs()
}

#[cfg(test)]
mod tests {
    use super::*;
    use tempfile::TempDir;

    #[test]
    fn optional_text_handles_empty() {
        assert_eq!(optional_text(""), None);
        assert_eq!(optional_text("  "), None);
        assert_eq!(optional_text("hi"), Some("hi".to_string()));
    }

    #[test]
    fn split_labels_parses_list() {
        let labels = split_labels("a, b, ,c");
        assert_eq!(
            labels,
            vec!["a".to_string(), "b".to_string(), "c".to_string()]
        );
    }

    #[test]
    fn format_delayed_start_reports_none() {
        assert_eq!(format_delayed_start(None), "none");
        assert_eq!(format_delayed_start(Some(0)), "none");
        assert_eq!(format_delayed_start(Some(15)), "15s");
    }

    #[test]
    fn menu_index_wraps_with_service_item() {
        let tmp = TempDir::new().unwrap();
        let mut app = TuiApp {
            config_path: std::path::PathBuf::from("/tmp/config.json"),
            config: AppConfigV2::default(),
            view: View::Main,
            menu_index: 9,
            message: String::new(),
            input_index: 0,
            input_fields: Vec::new(),
            provider_index: 0,
            token_menu_index: 0,
            token_validation: HashMap::new(),
            audit: AuditLogger::new_with_dir(tmp.path().to_path_buf(), 1024).unwrap(),
            log_buffer: LogBuffer::new(50),
            audit_filter: AuditFilter::All,
            validation_message: None,
            show_target_stats: false,
            repo_status: HashMap::new(),
            repo_status_last_refresh: None,
            repo_status_refreshing: false,
            repo_status_rx: None,
            repo_overview_message: None,
            sync_running: false,
            sync_rx: None,
            install_guard: None,
            install_rx: None,
            install_progress: None,
            install_status: None,
            install_scroll: 0,
            update_rx: None,
            update_progress: None,
            update_prompt: None,
            update_return_view: View::Main,
            restart_requested: false,
            message_return_view: View::Main,
            audit_scroll: 0,
            audit_search: String::new(),
            audit_search_active: false,
            repo_overview_selected: 0,
            repo_overview_scroll: 0,
            repo_overview_collapsed: HashSet::new(),
            repo_overview_compact: false,
        };
        let key = KeyEvent::new(KeyCode::Down, KeyModifiers::empty());
        app.handle_main(key).unwrap();
        assert_eq!(app.menu_index, 0);
    }

    #[test]
    fn read_audit_lines_handles_missing_file() {
        let tmp = TempDir::new().unwrap();
        let path = tmp.path().join("missing.jsonl");
        let lines = read_audit_lines(&path, AuditFilter::All).unwrap();
        assert_eq!(lines[0], "No audit log found for today.");
    }

    #[test]
    fn read_audit_lines_filters_failures() {
        let tmp = TempDir::new().unwrap();
        let path = tmp.path().join("audit.jsonl");
        std::fs::write(
            &path,
            r#"{"status":"ok","event":"a"}
{"status":"failed","event":"b"}
"#,
        )
        .unwrap();
        let lines = read_audit_lines(&path, AuditFilter::Failures).unwrap();
        assert_eq!(lines.len(), 1);
        assert!(lines[0].contains("\"status\":\"failed\""));
    }

    #[test]
    fn token_menu_enter_moves_to_set_view() {
        let tmp = TempDir::new().unwrap();
        let mut app = TuiApp {
            config_path: std::path::PathBuf::from("/tmp/config.json"),
            config: AppConfigV2::default(),
            view: View::TokenMenu,
            menu_index: 0,
            message: String::new(),
            input_index: 0,
            input_fields: Vec::new(),
            provider_index: 0,
            token_menu_index: 1,
            token_validation: HashMap::new(),
            audit: AuditLogger::new_with_dir(tmp.path().to_path_buf(), 1024).unwrap(),
            log_buffer: LogBuffer::new(50),
            audit_filter: AuditFilter::All,
            validation_message: None,
            show_target_stats: false,
            repo_status: HashMap::new(),
            repo_status_last_refresh: None,
            repo_status_refreshing: false,
            repo_status_rx: None,
            repo_overview_message: None,
            sync_running: false,
            sync_rx: None,
            install_guard: None,
            install_rx: None,
            install_progress: None,
            install_status: None,
            install_scroll: 0,
            update_rx: None,
            update_progress: None,
            update_prompt: None,
            update_return_view: View::Main,
            restart_requested: false,
            message_return_view: View::Main,
            audit_scroll: 0,
            audit_search: String::new(),
            audit_search_active: false,
            repo_overview_selected: 0,
            repo_overview_scroll: 0,
            repo_overview_collapsed: HashSet::new(),
            repo_overview_compact: false,
        };
        let key = KeyEvent::new(KeyCode::Enter, KeyModifiers::empty());
        app.handle_token_menu(key).unwrap();
        assert_eq!(app.view, View::TokenSet);
    }

    #[test]
    fn token_validation_display_reports_missing_scopes() {
        let validation = TokenValidation {
            status: TokenValidationStatus::MissingScopes(vec![
                "repo".to_string(),
                "read:org".to_string(),
            ]),
            at: "2026-02-04 12:00:00".to_string(),
        };
        let message = validation.display();
        assert!(message.contains("missing scopes"));
        assert!(message.contains("repo"));
    }

    #[test]
    fn token_validation_display_reports_unsupported_scopes() {
        let validation = TokenValidation {
            status: TokenValidationStatus::Unsupported,
            at: "2026-02-04 12:00:00".to_string(),
        };
        let message = validation.display();
        assert_eq!(
            message,
            "token valid (scope validation not supported) at 2026-02-04 12:00:00"
        );
    }

    #[test]
    fn form_hint_is_present_for_target_add() {
        let tmp = TempDir::new().unwrap();
        let app = TuiApp {
            config_path: std::path::PathBuf::from("/tmp/config.json"),
            config: AppConfigV2::default(),
            view: View::TargetAdd,
            menu_index: 0,
            message: String::new(),
            input_index: 0,
            input_fields: Vec::new(),
            provider_index: 0,
            token_menu_index: 0,
            token_validation: HashMap::new(),
            audit: AuditLogger::new_with_dir(tmp.path().to_path_buf(), 1024).unwrap(),
            log_buffer: LogBuffer::new(50),
            audit_filter: AuditFilter::All,
            validation_message: None,
            show_target_stats: false,
            repo_status: HashMap::new(),
            repo_status_last_refresh: None,
            repo_status_refreshing: false,
            repo_status_rx: None,
            repo_overview_message: None,
            sync_running: false,
            sync_rx: None,
            install_guard: None,
            install_rx: None,
            install_progress: None,
            install_status: None,
            install_scroll: 0,
            update_rx: None,
            update_progress: None,
            update_prompt: None,
            update_return_view: View::Main,
            restart_requested: false,
            message_return_view: View::Main,
            audit_scroll: 0,
            audit_search: String::new(),
            audit_search_active: false,
            repo_overview_selected: 0,
            repo_overview_scroll: 0,
            repo_overview_collapsed: HashSet::new(),
            repo_overview_compact: false,
        };
        assert!(app.form_hint().is_some());
    }

    #[test]
    fn install_action_for_versions_detects_update() {
        let action = install_action_for_versions(true, Some("1.2.3"), "1.3.0", false);
        assert_eq!(action, InstallAction::Update);
    }

    #[test]
    fn install_action_for_versions_install_when_missing() {
        let action = install_action_for_versions(false, None, "1.2.3", false);
        assert_eq!(action, InstallAction::Install);
    }

    #[test]
    fn install_action_for_versions_reinstall_when_current() {
        let action = install_action_for_versions(true, Some("1.2.3"), "1.2.3", false);
        assert_eq!(action, InstallAction::Reinstall);
        let action = install_action_for_versions(true, Some("1.2.3"), "1.3.0", true);
        assert_eq!(action, InstallAction::Reinstall);
    }

    #[test]
    fn dashboard_footer_includes_force_hotkey() {
        assert!(dashboard_footer_text().contains("f: force refresh all"));
    }
}

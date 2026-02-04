use anyhow::Context;
use crossterm::{
    event::{self, Event, KeyCode, KeyEvent, KeyEventKind, KeyModifiers},
    execute,
    terminal::{disable_raw_mode, enable_raw_mode, EnterAlternateScreen, LeaveAlternateScreen},
};
use mirror_core::audit::{AuditContext, AuditLogger, AuditStatus};
use mirror_core::cache::{RepoCache, SyncSummarySnapshot};
use mirror_core::config::{
    default_cache_path, default_config_path, load_or_migrate, target_id, AppConfigV2, TargetConfig,
};
use mirror_core::model::ProviderKind;
use mirror_providers::auth;
use mirror_providers::spec::{host_or_default, pat_help, spec_for};
use mirror_providers::ProviderRegistry;
use ratatui::{
    Terminal,
    backend::CrosstermBackend,
    layout::{Constraint, Direction, Layout},
    style::{Modifier, Style},
    text::{Line, Span},
    widgets::{Block, Borders, List, ListItem, Paragraph, Wrap},
};
use std::io::{self, Stdout};
use std::collections::{HashMap, HashSet};
use std::sync::mpsc;
use std::thread;
use std::time::{Duration, Instant};

#[derive(Clone, Copy, Debug)]
pub enum StartView {
    Main,
    Dashboard,
    Install,
}

pub fn run_tui(audit: &AuditLogger, start_view: StartView) -> anyhow::Result<()> {
    enable_raw_mode().context("enable raw mode")?;
    let mut stdout = io::stdout();
    execute!(stdout, EnterAlternateScreen).context("enter alternate screen")?;
    let backend = CrosstermBackend::new(stdout);
    let mut terminal = Terminal::new(backend).context("create terminal")?;

    let _ = audit.record("tui.start", AuditStatus::Ok, Some("tui"), None, None)?;
    let result = run_app(&mut terminal, audit, start_view);

    disable_raw_mode().ok();
    execute!(terminal.backend_mut(), LeaveAlternateScreen).ok();
    terminal.show_cursor().ok();

    if result.is_ok() {
        let _ = audit.record("tui.exit", AuditStatus::Ok, Some("tui"), None, None);
    }
    result
}

fn run_app(
    terminal: &mut Terminal<CrosstermBackend<Stdout>>,
    audit: &AuditLogger,
    start_view: StartView,
) -> anyhow::Result<()> {
    let mut app = TuiApp::load(audit.clone(), start_view)?;
    let mut last_tick = Instant::now();
    let tick_rate = Duration::from_millis(200);

    loop {
        terminal.draw(|frame| app.draw(frame))?;

        let timeout = tick_rate
            .checked_sub(last_tick.elapsed())
            .unwrap_or_else(|| Duration::from_secs(0));

        if event::poll(timeout)?
            && let Event::Key(key) = event::read()?
                && app.handle_key(key)? {
                    break;
                }

        if last_tick.elapsed() >= tick_rate {
            last_tick = Instant::now();
        }

        app.poll_install_events()?;
    }

    Ok(())
}

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
enum View {
    Main,
    Dashboard,
    Install,
    SyncStatus,
    InstallStatus,
    ConfigRoot,
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
    audit_filter: AuditFilter,
    validation_message: Option<String>,
    show_target_stats: bool,
    install_guard: Option<crate::install::InstallGuard>,
    install_rx: Option<mpsc::Receiver<InstallEvent>>,
    install_progress: Option<InstallProgressState>,
    install_status: Option<crate::install::InstallStatus>,
}

impl TuiApp {
    fn load(audit: AuditLogger, start_view: StartView) -> anyhow::Result<Self> {
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
            audit_filter: AuditFilter::All,
            validation_message: None,
            show_target_stats: false,
            install_guard: None,
            install_rx: None,
            install_progress: None,
            install_status: None,
        };
        if app.view == View::Install {
            app.enter_install_view()?;
        }
        Ok(app)
    }

    fn prepare_install_form(&mut self) {
        self.input_fields = vec![
            InputField::new("Delayed start seconds (optional)"),
            InputField::new("Add CLI to PATH? (y/n)"),
        ];
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
        self.view = View::Install;
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
            .constraints([Constraint::Length(3), Constraint::Min(0), Constraint::Length(3)])
            .split(frame.size());

        let header = Paragraph::new("Git Project Sync — Terminal UI")
            .block(Block::default().borders(Borders::ALL).title("Header"));
        frame.render_widget(header, layout[0]);

        match self.view {
            View::Main => self.draw_main(frame, layout[1]),
            View::Dashboard => self.draw_dashboard(frame, layout[1]),
            View::Install => self.draw_install(frame, layout[1]),
            View::SyncStatus => self.draw_sync_status(frame, layout[1]),
            View::InstallStatus => self.draw_install_status(frame, layout[1]),
            View::ConfigRoot => self.draw_config_root(frame, layout[1]),
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

        let footer = Paragraph::new(self.footer_text())
            .block(Block::default().borders(Borders::ALL).title("Help"));
        frame.render_widget(footer, layout[2]);
    }

    fn footer_text(&self) -> String {
        match self.view {
            View::Main => "Up/Down: navigate | Enter: select | q: quit".to_string(),
            View::Dashboard => "t: toggle targets | s: sync status | Esc: back".to_string(),
            View::Install => "Tab: next | Enter: run install | s: status | Esc: back".to_string(),
            View::SyncStatus => "Enter/Esc: back".to_string(),
            View::InstallStatus => "Enter/Esc: back".to_string(),
            View::ConfigRoot => "Enter: save | Esc: back".to_string(),
            View::Targets => "a: add | d: remove | Esc: back".to_string(),
            View::TargetAdd | View::TargetRemove | View::TokenSet | View::TokenValidate => {
                "Tab: next field | Enter: submit | Esc: back".to_string()
            }
            View::TokenMenu => "Up/Down: navigate | Enter: select | Esc: back".to_string(),
            View::TokenList => "Esc: back".to_string(),
            View::Service => "i: install | u: uninstall | Esc: back".to_string(),
            View::AuditLog => "f: failures | a: all | Esc: back".to_string(),
            View::Message => "Enter: back".to_string(),
            View::InstallProgress => "Installing... please wait".to_string(),
        }
    }

    fn draw_main(&self, frame: &mut ratatui::Frame, area: ratatui::layout::Rect) {
        let items = ["Dashboard",
            "Installer",
            "Config",
            "Targets",
            "Tokens",
            "Service",
            "Audit Log",
            "Quit"];
        let list_items: Vec<ListItem> = items
            .iter()
            .enumerate()
            .map(|(idx, item)| {
                let mut line = Line::from(Span::raw(*item));
                if idx == self.menu_index {
                    line = line.style(Style::default().add_modifier(Modifier::BOLD));
                }
                ListItem::new(line)
            })
            .collect();
        let list = List::new(list_items)
            .block(Block::default().borders(Borders::ALL).title("Main Menu"));
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
            Line::from(Span::raw(format!("No recent success: {}", stats.no_success_targets))),
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
            lines.push(Line::from(Span::raw(
                "Press t to show per-target status",
            )));
        }
        let widget = Paragraph::new(lines)
            .wrap(Wrap { trim: false })
            .block(Block::default().borders(Borders::ALL).title("Dashboard"));
        frame.render_widget(widget, area);
    }

    fn draw_install(&self, frame: &mut ratatui::Frame, area: ratatui::layout::Rect) {
        let mut lines = vec![
            Line::from(Span::raw("Context: Install daemon and optionally register PATH")),
            Line::from(Span::raw("")),
        ];
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
        let widget = Paragraph::new(lines)
            .wrap(Wrap { trim: false })
            .block(Block::default().borders(Borders::ALL).title("Installer"));
        frame.render_widget(widget, area);
    }

    fn draw_install_progress(&self, frame: &mut ratatui::Frame, area: ratatui::layout::Rect) {
        let mut lines = vec![
            Line::from(Span::raw("Context: Installing... please wait")),
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
            .block(Block::default().borders(Borders::ALL).title("Installer"));
        frame.render_widget(widget, area);
    }

    fn draw_install_status(&self, frame: &mut ratatui::Frame, area: ratatui::layout::Rect) {
        let mut lines = vec![
            Line::from(Span::raw("Context: Installer status")),
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
            lines.push(Line::from(Span::raw(format!(
                "{} installed: {}",
                service_label,
                if status.service_installed { "yes" } else { "no" }
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
                lines.push(Line::from(Span::raw("Task name: git-project-sync")));
            }
            lines.push(Line::from(Span::raw(format!(
                "PATH contains install dir (current shell): {}",
                if status.path_in_env { "yes" } else { "no" }
            ))));
        } else {
            lines.push(Line::from(Span::raw("Status unavailable.")));
        }
        let widget = Paragraph::new(lines)
            .wrap(Wrap { trim: false })
            .block(Block::default().borders(Borders::ALL).title("Installer Status"));
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

    fn draw_audit_log(&self, frame: &mut ratatui::Frame, area: ratatui::layout::Rect) {
        let log_path = self.audit_log_path();
        let lines = match read_audit_lines(&log_path, self.audit_filter) {
            Ok(lines) => lines,
            Err(err) => vec![format!("Failed to read audit log: {err}")],
        };
        let mut list_items: Vec<ListItem> = Vec::new();
        list_items.push(ListItem::new(Line::from(Span::raw(
            "Context: Audit log entries (newest first)",
        ))));
        list_items.push(ListItem::new(Line::from(Span::raw(""))));
        list_items.extend(lines.into_iter().map(|line| ListItem::new(Line::from(Span::raw(line)))));
        let list = List::new(list_items)
            .block(Block::default().borders(Borders::ALL).title("Audit Log Viewer"));
        frame.render_widget(list, area);
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
        let list = List::new(list_items)
            .block(Block::default().borders(Borders::ALL).title("Token Management"));
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
                let status = if entry.present {
                    "stored"
                } else {
                    "missing"
                };
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
        let list = List::new(items)
            .block(Block::default().borders(Borders::ALL).title("Token List"));
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
        lines.push(Line::from(Span::raw("Tip: Scope uses space-separated segments.")));
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
        let widget = Paragraph::new(lines)
            .wrap(Wrap { trim: false })
            .block(Block::default().borders(Borders::ALL).title("Set/Update Token"));
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
        lines.push(Line::from(Span::raw("Tip: Host optional; defaults to provider host.")));
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
        let widget = Paragraph::new(lines)
            .wrap(Wrap { trim: false })
            .block(Block::default().borders(Borders::ALL).title("Validate Token"));
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
        let widget = Paragraph::new(lines)
            .wrap(Wrap { trim: false })
            .block(Block::default().borders(Borders::ALL).title("Service Installer"));
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
            Line::from(Span::raw("Tip: Use an absolute path (e.g. /path/to/mirrors)")),
        ];
        if let Some(message) = self.validation_message.as_deref() {
            lines.push(Line::from(Span::raw("")));
            lines.push(Line::from(Span::raw(format!("Validation: {message}"))));
        }
        lines.push(Line::from(Span::raw("")));
        lines.push(Line::from(Span::raw("New root:")));
        lines.push(Line::from(Span::raw(
            self.input_fields.first()
                .map(|f| f.display_value())
                .unwrap_or_default(),
        )));
        let widget = Paragraph::new(lines)
            .wrap(Wrap { trim: false })
            .block(Block::default().borders(Borders::ALL).title("Config Root"));
        frame.render_widget(widget, area);
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
                let host = target.host.clone().unwrap_or_else(|| "(default)".to_string());
                items.push(ListItem::new(Line::from(Span::raw(format!(
                    "{} | {} | {} | {}",
                    target.id,
                    target.provider.as_prefix(),
                    target.scope.segments().join("/"),
                    host
                )))));
            }
        }
        let list = List::new(items)
            .block(Block::default().borders(Borders::ALL).title("Targets"));
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
        if matches!(self.view, View::TargetAdd | View::TokenSet) {
            lines.push(Line::from(Span::raw(format!(
                "Provider: {}",
                provider_label(self.provider_index)
            ))));
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

    fn handle_key(&mut self, key: KeyEvent) -> anyhow::Result<bool> {
        if key.kind != KeyEventKind::Press {
            return Ok(false);
        }
        match self.view {
            View::Main => self.handle_main(key),
            View::Dashboard => self.handle_dashboard(key),
            View::Install => self.handle_install(key),
            View::ConfigRoot => self.handle_config_root(key),
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
            KeyCode::Down => self.menu_index = (self.menu_index + 1) % 8,
            KeyCode::Up => {
                if self.menu_index == 0 {
                    self.menu_index = 7;
                } else {
                    self.menu_index -= 1;
                }
            }
            KeyCode::Enter => match self.menu_index {
                0 => self.view = View::Dashboard,
                1 => {
                    if let Err(err) = self.enter_install_view() {
                        self.message = format!("Installer unavailable: {err}");
                        self.view = View::Message;
                    }
                }
                2 => {
                    self.view = View::ConfigRoot;
                    self.input_fields = vec![InputField::new("Root path")];
                    self.input_index = 0;
                }
                3 => self.view = View::Targets,
                4 => {
                    self.view = View::TokenMenu;
                    self.token_menu_index = 0;
                }
                5 => self.view = View::Service,
                6 => self.view = View::AuditLog,
                _ => return Ok(true),
            },
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
            KeyCode::Enter => {
                if let Err(err) = self.ensure_install_guard() {
                    self.message = format!("Installer unavailable: {err}");
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
                            self.validation_message = Some("Delayed start must be a number.".to_string());
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
                self.view = View::InstallStatus;
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
            }
            KeyCode::Char('s') => {
                self.view = View::SyncStatus;
            }
            _ => {}
        }
        Ok(false)
    }

    fn handle_config_root(&mut self, key: KeyEvent) -> anyhow::Result<bool> {
        match key.code {
            KeyCode::Esc => self.view = View::Main,
            KeyCode::Enter => {
                let value = self.input_fields.first().map(|f| f.value.trim().to_string());
                if let Some(path) = value {
                    if path.is_empty() {
                        self.validation_message = Some("Root path cannot be empty.".to_string());
                        self.message = "Root path cannot be empty.".to_string();
                        self.view = View::Message;
                        return Ok(false);
                    }
                    self.config.root = Some(path.into());
                    self.config.save(&self.config_path)?;
                    self.validation_message = None;
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
                        self.validation_message = Some(format!("Scope invalid: {err}"));
                        return Ok(false);
                    }
                };
                let host = optional_text(&self.input_fields[1].value);
                let labels = split_labels(&self.input_fields[2].value);
                let id = target_id(provider.clone(), host.as_deref(), &scope);
                if self.config.targets.iter().any(|t| t.id == id) {
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
                    self.validation_message = Some("Target id is required.".to_string());
                    self.message = "Target id required.".to_string();
                    self.view = View::Message;
                    return Ok(false);
                }
                let before = self.config.targets.len();
                self.config.targets.retain(|t| t.id != id);
                let after = self.config.targets.len();
                if before == after {
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
        if key.code == KeyCode::Esc { self.view = View::TokenMenu }
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
                        self.validation_message = Some(format!("Scope invalid: {err}"));
                        return Ok(false);
                    }
                };
                let host = optional_text(&self.input_fields[1].value);
                let host = host_or_default(host.as_deref(), spec.as_ref());
                let account = spec.account_key(&host, &scope)?;
                let token = self.input_fields[2].value.trim().to_string();
                if token.is_empty() {
                    self.validation_message = Some("Token cannot be empty.".to_string());
                    self.message = "Token cannot be empty.".to_string();
                    self.view = View::Message;
                    return Ok(false);
                }
                auth::set_pat(&account, &token)?;
                let validation = self.validate_token(provider.clone(), scope.clone(), Some(host.clone()));
                let validation_message = match &validation {
                    Ok(record) => {
                        self.token_validation.insert(account.clone(), record.clone());
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
                        self.validation_message = Some(format!("Scope invalid: {err}"));
                        return Ok(false);
                    }
                };
                let host = optional_text(&self.input_fields[1].value);
                let host = host_or_default(host.as_deref(), spec.as_ref());
                let account = spec.account_key(&host, &scope)?;
                let validation = self.validate_token(provider.clone(), scope.clone(), Some(host.clone()));
                match validation {
                    Ok(record) => {
                        self.token_validation.insert(account.clone(), record.clone());
                        let status = record.display();
                        self.validation_message = None;
                        let audit_status = match record.status {
                            TokenValidationStatus::Ok => AuditStatus::Ok,
                            TokenValidationStatus::MissingScopes(_) => AuditStatus::Failed,
                            TokenValidationStatus::Unsupported => AuditStatus::Skipped,
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
                            None,
                        )?;
                        self.message = format!("{status}. Audit ID: {audit_id}");
                    }
                    Err(err) => {
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
        if key.code == KeyCode::Enter { self.view = View::Main }
        Ok(false)
    }

    fn handle_audit_log(&mut self, key: KeyEvent) -> anyhow::Result<bool> {
        match key.code {
            KeyCode::Esc => self.view = View::Main,
            KeyCode::Char('f') => {
                self.audit_filter = AuditFilter::Failures;
            }
            KeyCode::Char('a') => {
                self.audit_filter = AuditFilter::All;
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

    fn handle_install_status(&mut self, key: KeyEvent) -> anyhow::Result<bool> {
        match key.code {
            KeyCode::Enter | KeyCode::Esc => self.view = View::Install,
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
                    let state = self.install_progress.get_or_insert_with(|| {
                        InstallProgressState::new(progress.total)
                    });
                    state.update(progress);
                }
                InstallEvent::Done(result) => {
                    self.release_install_guard();
                    match result {
                        Ok(report) => {
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
            View::TargetAdd => Some("Scope uses space-separated segments (org project)"),
            View::TargetRemove => Some("Find target ids on the Targets screen"),
            View::TokenSet => Some("Scope uses space-separated segments (org project)"),
            View::TokenValidate => Some("Host optional; defaults to provider host"),
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
                    TokenValidationStatus::MissingScopes(missing.iter().map(|s| s.to_string()).collect())
                }
            }
            None => TokenValidationStatus::Unsupported,
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
            let repo = status
                .and_then(|s| s.last_repo.as_deref())
                .unwrap_or("-");
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
            TokenValidationStatus::Unsupported => format!("validation unsupported at {}", self.at),
        }
    }
}

#[derive(Clone)]
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

fn progress_bar(step: usize, total: usize, width: usize) -> String {
    if total == 0 || width == 0 {
        return "[]".to_string();
    }
    let filled = ((step as f32 / total as f32) * width as f32).round() as usize;
    let filled = filled.min(width);
    let empty = width.saturating_sub(filled);
    format!("[{}{}]", "#".repeat(filled), "-".repeat(empty))
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
        if let Ok(value) = serde_json::from_str::<serde_json::Value>(line) {
            if let Some(error) = value.get("error").and_then(|v| v.as_str()) {
                return Ok(error.to_string());
            }
        }
    }
    Ok(String::new())
}

fn validation_timestamp() -> String {
    time::OffsetDateTime::now_utc()
        .format(&time::format_description::parse("[year]-[month]-[day] [hour]:[minute]:[second]").unwrap())
        .unwrap_or_else(|_| "unknown".to_string())
}

fn epoch_to_label(epoch: u64) -> String {
    let ts = time::OffsetDateTime::from_unix_timestamp(epoch as i64)
        .unwrap_or_else(|_| time::OffsetDateTime::now_utc());
    ts.format(&time::format_description::parse("[year]-[month]-[day] [hour]:[minute]").unwrap())
        .unwrap_or_else(|_| "unknown".to_string())
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
        assert_eq!(labels, vec!["a".to_string(), "b".to_string(), "c".to_string()]);
    }

    #[test]
    fn menu_index_wraps_with_service_item() {
        let tmp = TempDir::new().unwrap();
        let mut app = TuiApp {
            config_path: std::path::PathBuf::from("/tmp/config.json"),
            config: AppConfigV2::default(),
            view: View::Main,
            menu_index: 7,
            message: String::new(),
            input_index: 0,
            input_fields: Vec::new(),
            provider_index: 0,
            token_menu_index: 0,
            token_validation: HashMap::new(),
            audit: AuditLogger::new_with_dir(tmp.path().to_path_buf(), 1024).unwrap(),
            audit_filter: AuditFilter::All,
            validation_message: None,
            show_target_stats: false,
            install_guard: None,
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
            audit_filter: AuditFilter::All,
            validation_message: None,
            show_target_stats: false,
            install_guard: None,
        };
        let key = KeyEvent::new(KeyCode::Enter, KeyModifiers::empty());
        app.handle_token_menu(key).unwrap();
        assert_eq!(app.view, View::TokenSet);
    }

    #[test]
    fn token_validation_display_reports_missing_scopes() {
        let validation = TokenValidation {
            status: TokenValidationStatus::MissingScopes(vec!["repo".to_string(), "read:org".to_string()]),
            at: "2026-02-04 12:00:00".to_string(),
        };
        let message = validation.display();
        assert!(message.contains("missing scopes"));
        assert!(message.contains("repo"));
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
            audit_filter: AuditFilter::All,
            validation_message: None,
            show_target_stats: false,
            install_guard: None,
        };
        assert!(app.form_hint().is_some());
    }
}

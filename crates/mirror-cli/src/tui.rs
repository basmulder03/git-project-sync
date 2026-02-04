use anyhow::Context;
use crossterm::{
    event::{self, Event, KeyCode, KeyEvent, KeyModifiers},
    execute,
    terminal::{disable_raw_mode, enable_raw_mode, EnterAlternateScreen, LeaveAlternateScreen},
};
use mirror_core::audit::{AuditContext, AuditLogger, AuditStatus};
use mirror_core::config::{default_config_path, load_or_migrate, target_id, AppConfigV2, TargetConfig};
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
use std::time::{Duration, Instant};

pub fn run_tui(audit: &AuditLogger) -> anyhow::Result<()> {
    enable_raw_mode().context("enable raw mode")?;
    let mut stdout = io::stdout();
    execute!(stdout, EnterAlternateScreen).context("enter alternate screen")?;
    let backend = CrosstermBackend::new(stdout);
    let mut terminal = Terminal::new(backend).context("create terminal")?;

    let _ = audit.record("tui.start", AuditStatus::Ok, Some("tui"), None, None)?;
    let result = run_app(&mut terminal, audit);

    disable_raw_mode().ok();
    execute!(terminal.backend_mut(), LeaveAlternateScreen).ok();
    terminal.show_cursor().ok();

    if result.is_ok() {
        let _ = audit.record("tui.exit", AuditStatus::Ok, Some("tui"), None, None);
    }
    result
}

fn run_app(terminal: &mut Terminal<CrosstermBackend<Stdout>>, audit: &AuditLogger) -> anyhow::Result<()> {
    let mut app = TuiApp::load(audit.clone())?;
    let mut last_tick = Instant::now();
    let tick_rate = Duration::from_millis(200);

    loop {
        terminal.draw(|frame| app.draw(frame))?;

        let timeout = tick_rate
            .checked_sub(last_tick.elapsed())
            .unwrap_or_else(|| Duration::from_secs(0));

        if event::poll(timeout)? {
            if let Event::Key(key) = event::read()? {
                if app.handle_key(key)? {
                    break;
                }
            }
        }

        if last_tick.elapsed() >= tick_rate {
            last_tick = Instant::now();
        }
    }

    Ok(())
}

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
enum View {
    Main,
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
}

impl TuiApp {
    fn load(audit: AuditLogger) -> anyhow::Result<Self> {
        let config_path = default_config_path()?;
        let (config, migrated) = load_or_migrate(&config_path)?;
        if migrated {
            config.save(&config_path)?;
        }
        Ok(Self {
            config_path,
            config,
            view: View::Main,
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
        })
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
        }

        let footer = Paragraph::new(self.footer_text())
            .block(Block::default().borders(Borders::ALL).title("Help"));
        frame.render_widget(footer, layout[2]);
    }

    fn footer_text(&self) -> String {
        match self.view {
            View::Main => "Up/Down: navigate | Enter: select | q: quit".to_string(),
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
        }
    }

    fn draw_main(&self, frame: &mut ratatui::Frame, area: ratatui::layout::Rect) {
        let items = vec![
            "Config",
            "Targets",
            "Tokens",
            "Service",
            "Audit Log",
            "Quit",
        ];
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
        let items = vec!["List", "Set/Update", "Validate", "Back"];
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
            self.input_fields
                .get(0)
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
        match self.view {
            View::Main => self.handle_main(key),
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
        }
    }

    fn handle_main(&mut self, key: KeyEvent) -> anyhow::Result<bool> {
        match key.code {
            KeyCode::Char('q') => return Ok(true),
            KeyCode::Down => self.menu_index = (self.menu_index + 1) % 6,
            KeyCode::Up => {
                if self.menu_index == 0 {
                    self.menu_index = 5;
                } else {
                    self.menu_index -= 1;
                }
            }
            KeyCode::Enter => match self.menu_index {
                0 => {
                    self.view = View::ConfigRoot;
                    self.input_fields = vec![InputField::new("Root path")];
                    self.input_index = 0;
                }
                1 => self.view = View::Targets,
                2 => {
                    self.view = View::TokenMenu;
                    self.token_menu_index = 0;
                }
                3 => self.view = View::Service,
                4 => self.view = View::AuditLog,
                _ => return Ok(true),
            },
            _ => {}
        }
        Ok(false)
    }

    fn handle_config_root(&mut self, key: KeyEvent) -> anyhow::Result<bool> {
        match key.code {
            KeyCode::Esc => self.view = View::Main,
            KeyCode::Enter => {
                let value = self.input_fields.get(0).map(|f| f.value.trim().to_string());
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
                    id,
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
                        repo_id: Some(self.config.targets.last().unwrap().id.clone()),
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
        match key.code {
            KeyCode::Esc => self.view = View::TokenMenu,
            _ => {}
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
        match key.code {
            KeyCode::Enter => self.view = View::Main,
            _ => {}
        }
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

fn validation_timestamp() -> String {
    time::OffsetDateTime::now_utc()
        .format(&time::format_description::parse("[year]-[month]-[day] [hour]:[minute]:[second]").unwrap())
        .unwrap_or_else(|_| "unknown".to_string())
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
            menu_index: 5,
            message: String::new(),
            input_index: 0,
            input_fields: Vec::new(),
            provider_index: 0,
            token_menu_index: 0,
            token_validation: HashMap::new(),
            audit: AuditLogger::new_with_dir(tmp.path().to_path_buf(), 1024).unwrap(),
            audit_filter: AuditFilter::All,
            validation_message: None,
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
        };
        assert!(app.form_hint().is_some());
    }
}

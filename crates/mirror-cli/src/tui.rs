use anyhow::Context;
use crossterm::{
    event::{self, Event, KeyCode, KeyEvent, KeyModifiers},
    execute,
    terminal::{disable_raw_mode, enable_raw_mode, EnterAlternateScreen, LeaveAlternateScreen},
};
use mirror_core::config::{default_config_path, load_or_migrate, target_id, AppConfigV2, TargetConfig};
use mirror_core::model::{ProviderKind, ProviderScope};
use mirror_providers::auth;
use mirror_providers::spec::{host_or_default, spec_for};
use ratatui::{
    Terminal,
    backend::CrosstermBackend,
    layout::{Constraint, Direction, Layout},
    style::{Modifier, Style},
    text::{Line, Span},
    widgets::{Block, Borders, List, ListItem, Paragraph, Wrap},
};
use std::io::{self, Stdout};
use std::time::{Duration, Instant};

pub fn run_tui() -> anyhow::Result<()> {
    enable_raw_mode().context("enable raw mode")?;
    let mut stdout = io::stdout();
    execute!(stdout, EnterAlternateScreen).context("enter alternate screen")?;
    let backend = CrosstermBackend::new(stdout);
    let mut terminal = Terminal::new(backend).context("create terminal")?;

    let result = run_app(&mut terminal);

    disable_raw_mode().ok();
    execute!(terminal.backend_mut(), LeaveAlternateScreen).ok();
    terminal.show_cursor().ok();

    result
}

fn run_app(terminal: &mut Terminal<CrosstermBackend<Stdout>>) -> anyhow::Result<()> {
    let mut app = TuiApp::load()?;
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
    TokenSetup,
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
}

impl TuiApp {
    fn load() -> anyhow::Result<Self> {
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
            View::TokenSetup => self.draw_form(frame, layout[1], "Token Setup"),
            View::Message => self.draw_message(frame, layout[1]),
        }

        let footer = Paragraph::new(self.footer_text())
            .block(Block::default().borders(Borders::ALL).title("Help"));
        frame.render_widget(footer, layout[2]);
    }

    fn footer_text(&self) -> String {
        match self.view {
            View::Main => "Up/Down: navigate | Enter: select | q: quit".to_string(),
            View::ConfigRoot => "Type path | Enter: save | Esc: back".to_string(),
            View::Targets => "a: add | d: delete | Esc: back".to_string(),
            View::TargetAdd | View::TargetRemove | View::TokenSetup => {
                "Tab: next field | Enter: submit | Esc: back".to_string()
            }
            View::Message => "Enter: back".to_string(),
        }
    }

    fn draw_main(&self, frame: &mut ratatui::Frame, area: ratatui::layout::Rect) {
        let items = vec![
            "Config Root",
            "Targets",
            "Token Setup",
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

    fn draw_config_root(&self, frame: &mut ratatui::Frame, area: ratatui::layout::Rect) {
        let current = self
            .config
            .root
            .as_ref()
            .map(|p| p.display().to_string())
            .unwrap_or_else(|| "<unset>".to_string());
        let body = format!(
            "Current root: {current}\n\nNew root:\n{}",
            self.input_fields
                .get(0)
                .map(|f| f.display_value())
                .unwrap_or_default()
        );
        let widget = Paragraph::new(body)
            .wrap(Wrap { trim: false })
            .block(Block::default().borders(Borders::ALL).title("Config Root"));
        frame.render_widget(widget, area);
    }

    fn draw_targets(&self, frame: &mut ratatui::Frame, area: ratatui::layout::Rect) {
        let mut items: Vec<ListItem> = Vec::new();
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
        if matches!(self.view, View::TargetAdd | View::TokenSetup) {
            lines.push(Line::from(Span::raw(format!(
                "Provider: {}",
                provider_label(self.provider_index)
            ))));
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
            View::TokenSetup => self.handle_token_setup(key),
            View::Message => self.handle_message(key),
        }
    }

    fn handle_main(&mut self, key: KeyEvent) -> anyhow::Result<bool> {
        match key.code {
            KeyCode::Char('q') => return Ok(true),
            KeyCode::Down => self.menu_index = (self.menu_index + 1) % 4,
            KeyCode::Up => {
                if self.menu_index == 0 {
                    self.menu_index = 3;
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
                    self.view = View::TokenSetup;
                    self.input_fields = vec![
                        InputField::new("Scope (space-separated)"),
                        InputField::new("Host (optional)"),
                        InputField::with_mask("Token"),
                    ];
                    self.input_index = 0;
                    self.provider_index = 0;
                }
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
                        self.message = "Root path cannot be empty.".to_string();
                        self.view = View::Message;
                        return Ok(false);
                    }
                    self.config.root = Some(path.into());
                    self.config.save(&self.config_path)?;
                    self.message = "Root saved.".to_string();
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
                let scope = spec.parse_scope(scope_raw.split_whitespace().map(|s| s.to_string()).collect())
                    .context("invalid scope")?;
                let host = optional_text(&self.input_fields[1].value);
                let labels = split_labels(&self.input_fields[2].value);
                let id = target_id(provider.clone(), host.as_deref(), &scope);
                if self.config.targets.iter().any(|t| t.id == id) {
                    self.message = "Target already exists.".to_string();
                    self.view = View::Message;
                    return Ok(false);
                }
                self.config.targets.push(TargetConfig {
                    id,
                    provider,
                    scope,
                    host,
                    labels,
                });
                self.config.save(&self.config_path)?;
                self.message = "Target added.".to_string();
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
                    self.message = "Target id required.".to_string();
                    self.view = View::Message;
                    return Ok(false);
                }
                let before = self.config.targets.len();
                self.config.targets.retain(|t| t.id != id);
                let after = self.config.targets.len();
                if before == after {
                    self.message = "No target found with that id.".to_string();
                } else {
                    self.config.save(&self.config_path)?;
                    self.message = "Target removed.".to_string();
                }
                self.view = View::Message;
            }
            _ => self.handle_text_input(key),
        }
        Ok(false)
    }

    fn handle_token_setup(&mut self, key: KeyEvent) -> anyhow::Result<bool> {
        match key.code {
            KeyCode::Esc => self.view = View::Main,
            KeyCode::Tab => self.input_index = (self.input_index + 1) % self.input_fields.len(),
            KeyCode::Left => self.provider_index = self.provider_index.saturating_sub(1),
            KeyCode::Right => {
                self.provider_index = (self.provider_index + 1).min(2);
            }
            KeyCode::Enter => {
                let provider = provider_kind(self.provider_index);
                let spec = spec_for(provider.clone());
                let scope_raw = self.input_fields[0].value.trim();
                let scope = spec.parse_scope(scope_raw.split_whitespace().map(|s| s.to_string()).collect())
                    .context("invalid scope")?;
                let host = optional_text(&self.input_fields[1].value);
                let host = host_or_default(host.as_deref(), spec.as_ref());
                let account = spec.account_key(&host, &scope)?;
                let token = self.input_fields[2].value.trim().to_string();
                if token.is_empty() {
                    self.message = "Token cannot be empty.".to_string();
                    self.view = View::Message;
                    return Ok(false);
                }
                auth::set_pat(&account, &token)?;
                self.message = format!("Token stored for {account}");
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

    fn handle_text_input(&mut self, key: KeyEvent) {
        if self.input_fields.is_empty() {
            return;
        }
        let field = &mut self.input_fields[self.input_index];
        match key.code {
            KeyCode::Char('c') if key.modifiers.contains(KeyModifiers::CONTROL) => {
                field.value.clear();
            }
            KeyCode::Backspace => {
                field.pop();
            }
            KeyCode::Char(ch) => {
                field.push(ch);
            }
            _ => {}
        }
    }
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

#[cfg(test)]
mod tests {
    use super::*;

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
}

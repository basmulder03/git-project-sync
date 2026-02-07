use super::*;
use crate::i18n::{key, tr};

impl TuiApp {
    pub(in crate::tui) fn draw_token_menu(
        &mut self,
        frame: &mut ratatui::Frame,
        area: ratatui::layout::Rect,
    ) {
        let items = ["List", "Set/Update", "Validate", "Back"];
        let body_height = area.height.saturating_sub(2) as usize;
        let max_scroll = items.len().saturating_sub(body_height);
        let mut scroll = self.scroll_offset(View::TokenMenu).min(max_scroll);
        scroll = adjust_scroll(self.token_menu_index, scroll, body_height, items.len());
        self.set_scroll_offset(View::TokenMenu, scroll);
        let end = (scroll + body_height).min(items.len());
        let list_items: Vec<ListItem> = items[scroll..end]
            .iter()
            .enumerate()
            .map(|(offset, item)| {
                let idx = scroll + offset;
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
                .title(tr(key::TOKENS_TITLE)),
        );
        frame.render_widget(list, area);
    }

    pub(in crate::tui) fn draw_token_list(
        &mut self,
        frame: &mut ratatui::Frame,
        area: ratatui::layout::Rect,
    ) {
        let entries = self.token_entries();
        let mut lines: Vec<Line> = Vec::new();
        lines.push(Line::from(Span::raw(tr(
            "Context: Tokens per configured target",
        ))));
        lines.push(Line::from(Span::raw("")));
        if entries.is_empty() {
            lines.push(Line::from(Span::raw(tr(key::TOKENS_NONE_CONFIGURED_YET))));
        } else {
            for entry in entries {
                let status = if entry.present { "stored" } else { "missing" };
                let validation = entry
                    .validation
                    .as_ref()
                    .map(|v| format!(" | {}", v.display()))
                    .unwrap_or_else(|| " | not verified".to_string());
                lines.push(Line::from(Span::raw(format!(
                    "{} | {} | {} | {} | {}{}",
                    entry.account,
                    entry.provider.as_prefix(),
                    entry.scope,
                    entry.host,
                    status,
                    validation
                ))));
            }
        }
        let max_scroll = max_scroll_for_lines(lines.len(), area.height);
        let scroll = self.scroll_offset(View::TokenList).min(max_scroll);
        self.set_scroll_offset(View::TokenList, scroll);
        let widget = Paragraph::new(lines)
            .wrap(Wrap { trim: false })
            .scroll((scroll as u16, 0))
            .block(
                Block::default()
                    .borders(Borders::ALL)
                    .title(tr(key::TOKENS_TITLE)),
            );
        frame.render_widget(widget, area);
    }

    pub(in crate::tui) fn draw_token_set(
        &mut self,
        frame: &mut ratatui::Frame,
        area: ratatui::layout::Rect,
    ) {
        let provider = provider_kind(self.provider_index);
        let help = pat_help(provider.clone());
        let mut lines = Vec::new();
        lines.push(Line::from(Span::raw(tr(key::TOKENS_CONTEXT_SET))));
        lines.push(Line::from(Span::raw("")));
        lines.push(Line::from(Span::raw(format!(
            "Provider: {}",
            provider_label(self.provider_index)
        ))));
        lines.push(Line::from(Span::raw(format!("Create PAT: {}", help.url))));
        lines.push(Line::from(Span::raw(format!(
            "Required access: {}",
            help.scopes.join(", ")
        ))));
        lines.push(Line::from(Span::raw(tr(
            "Tip: Scope uses space-separated segments.",
        ))));
        if let Some(message) = self.validation_message.as_deref() {
            lines.push(Line::from(Span::raw("")));
            lines.push(Line::from(Span::raw(format!(
                "{} {message}",
                tr(key::VALIDATION_LABEL)
            ))));
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
        let max_scroll = max_scroll_for_lines(lines.len(), area.height);
        let scroll = self.scroll_offset(View::TokenSet).min(max_scroll);
        self.set_scroll_offset(View::TokenSet, scroll);
        let widget = Paragraph::new(lines)
            .wrap(Wrap { trim: false })
            .scroll((scroll as u16, 0))
            .block(
                Block::default()
                    .borders(Borders::ALL)
                    .title(tr(key::TOKEN_SET_TITLE)),
            );
        frame.render_widget(widget, area);
    }

    pub(in crate::tui) fn draw_token_validate(
        &mut self,
        frame: &mut ratatui::Frame,
        area: ratatui::layout::Rect,
    ) {
        let provider = provider_kind(self.provider_index);
        let help = pat_help(provider.clone());
        let mut lines = Vec::new();
        lines.push(Line::from(Span::raw(tr(
            "Context: Validate required scopes",
        ))));
        lines.push(Line::from(Span::raw("")));
        lines.push(Line::from(Span::raw(format!(
            "Provider: {}",
            provider_label(self.provider_index)
        ))));
        lines.push(Line::from(Span::raw(format!(
            "Required access: {}",
            help.scopes.join(", ")
        ))));
        lines.push(Line::from(Span::raw(tr(
            "Tip: Host optional; defaults to provider host.",
        ))));
        if let Some(message) = self.validation_message.as_deref() {
            lines.push(Line::from(Span::raw("")));
            lines.push(Line::from(Span::raw(format!(
                "{} {message}",
                tr(key::VALIDATION_LABEL)
            ))));
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
        let max_scroll = max_scroll_for_lines(lines.len(), area.height);
        let scroll = self.scroll_offset(View::TokenValidate).min(max_scroll);
        self.set_scroll_offset(View::TokenValidate, scroll);
        let widget = Paragraph::new(lines)
            .wrap(Wrap { trim: false })
            .scroll((scroll as u16, 0))
            .block(
                Block::default()
                    .borders(Borders::ALL)
                    .title(tr(key::TOKEN_VALIDATE_TITLE)),
            );
        frame.render_widget(widget, area);
    }

    pub(in crate::tui) fn draw_service(
        &mut self,
        frame: &mut ratatui::Frame,
        area: ratatui::layout::Rect,
    ) {
        let os = std::env::consts::OS;
        let os_hint = match os {
            "linux" => "systemd user service",
            "macos" => "LaunchAgent",
            "windows" => "Windows service",
            _ => "service helper",
        };
        let lines = vec![
            Line::from(Span::raw(tr(
                "Install or uninstall the background service.",
            ))),
            Line::from(Span::raw(format!("Detected OS: {os} ({os_hint})"))),
            Line::from(Span::raw("")),
            Line::from(Span::raw(tr(key::SERVICE_PRESS_INSTALL))),
            Line::from(Span::raw(tr(key::SERVICE_PRESS_UNINSTALL))),
        ];
        let max_scroll = max_scroll_for_lines(lines.len(), area.height);
        let scroll = self.scroll_offset(View::Service).min(max_scroll);
        self.set_scroll_offset(View::Service, scroll);
        let widget = Paragraph::new(lines)
            .wrap(Wrap { trim: false })
            .scroll((scroll as u16, 0))
            .block(
                Block::default()
                    .borders(Borders::ALL)
                    .title(tr(key::SERVICE_TITLE)),
            );
        frame.render_widget(widget, area);
    }

    pub(in crate::tui) fn draw_config_root(
        &mut self,
        frame: &mut ratatui::Frame,
        area: ratatui::layout::Rect,
    ) {
        let current = self
            .config
            .root
            .as_ref()
            .map(|p| p.display().to_string())
            .unwrap_or_else(|| "<unset>".to_string());
        let mut lines = vec![
            Line::from(Span::raw(tr(key::CONFIG_ROOT_CONTEXT))),
            Line::from(Span::raw(format!("Current root: {current}"))),
            Line::from(Span::raw("")),
            Line::from(Span::raw(tr(
                "Tip: Use an absolute path (e.g. /path/to/mirrors)",
            ))),
        ];
        if let Some(message) = self.validation_message.as_deref() {
            lines.push(Line::from(Span::raw("")));
            lines.push(Line::from(Span::raw(format!(
                "{} {message}",
                tr(key::VALIDATION_LABEL)
            ))));
        }
        lines.push(Line::from(Span::raw("")));
        lines.push(Line::from(Span::raw(tr(key::CONFIG_ROOT_NEW_ROOT))));
        lines.push(Line::from(Span::raw(
            self.input_fields
                .first()
                .map(|f| f.display_value())
                .unwrap_or_default(),
        )));
        let max_scroll = max_scroll_for_lines(lines.len(), area.height);
        let scroll = self.scroll_offset(View::ConfigRoot).min(max_scroll);
        self.set_scroll_offset(View::ConfigRoot, scroll);
        let widget = Paragraph::new(lines)
            .wrap(Wrap { trim: false })
            .scroll((scroll as u16, 0))
            .block(
                Block::default()
                    .borders(Borders::ALL)
                    .title(tr(key::CONFIG_ROOT_TITLE)),
            );
        frame.render_widget(widget, area);
    }
}

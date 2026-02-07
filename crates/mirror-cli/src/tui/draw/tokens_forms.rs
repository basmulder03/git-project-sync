use super::*;

impl TuiApp {
    pub(in crate::tui) fn draw_token_menu(
        &self,
        frame: &mut ratatui::Frame,
        area: ratatui::layout::Rect,
    ) {
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

    pub(in crate::tui) fn draw_token_list(
        &self,
        frame: &mut ratatui::Frame,
        area: ratatui::layout::Rect,
    ) {
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

    pub(in crate::tui) fn draw_token_set(
        &self,
        frame: &mut ratatui::Frame,
        area: ratatui::layout::Rect,
    ) {
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
            "Required access: {}",
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

    pub(in crate::tui) fn draw_token_validate(
        &self,
        frame: &mut ratatui::Frame,
        area: ratatui::layout::Rect,
    ) {
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
            "Required access: {}",
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

    pub(in crate::tui) fn draw_service(
        &self,
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

    pub(in crate::tui) fn draw_config_root(
        &self,
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
}

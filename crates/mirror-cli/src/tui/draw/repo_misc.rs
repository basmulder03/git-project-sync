use super::*;

impl TuiApp {
    pub(in crate::tui) fn draw_repo_overview(
        &mut self,
        frame: &mut ratatui::Frame,
        area: ratatui::layout::Rect,
    ) {
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

    pub(in crate::tui) fn draw_targets(
        &mut self,
        frame: &mut ratatui::Frame,
        area: ratatui::layout::Rect,
    ) {
        let mut lines: Vec<Line> = Vec::new();
        lines.push(Line::from(Span::raw(
            "Context: Targets map to provider + scope",
        )));
        lines.push(Line::from(Span::raw("Tip: Press a to add or d to remove")));
        lines.push(Line::from(Span::raw("")));
        if self.config.targets.is_empty() {
            lines.push(Line::from(Span::raw("No targets configured.")));
        } else {
            for target in &self.config.targets {
                let host = target
                    .host
                    .clone()
                    .unwrap_or_else(|| "(default)".to_string());
                lines.push(Line::from(Span::raw(format!(
                    "{} | {} | {} | {}",
                    target.id,
                    target.provider.as_prefix(),
                    target.scope.segments().join("/"),
                    host
                ))));
            }
        }
        let max_scroll = max_scroll_for_lines(lines.len(), area.height);
        let scroll = self.scroll_offset(View::Targets).min(max_scroll);
        self.set_scroll_offset(View::Targets, scroll);
        let widget = Paragraph::new(lines)
            .wrap(Wrap { trim: false })
            .scroll((scroll as u16, 0))
            .block(Block::default().borders(Borders::ALL).title("Targets"));
        frame.render_widget(widget, area);
    }

    pub(in crate::tui) fn draw_form(
        &mut self,
        frame: &mut ratatui::Frame,
        area: ratatui::layout::Rect,
        title: &str,
    ) {
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
        let max_scroll = max_scroll_for_lines(lines.len(), area.height);
        let scroll = self.scroll_offset(self.view).min(max_scroll);
        self.set_scroll_offset(self.view, scroll);
        let widget = Paragraph::new(lines)
            .wrap(Wrap { trim: false })
            .scroll((scroll as u16, 0))
            .block(Block::default().borders(Borders::ALL).title(title));
        frame.render_widget(widget, area);
    }

    pub(in crate::tui) fn draw_message(
        &mut self,
        frame: &mut ratatui::Frame,
        area: ratatui::layout::Rect,
    ) {
        let lines = self.message.lines().count().max(1);
        let max_scroll = max_scroll_for_lines(lines, area.height);
        let scroll = self.scroll_offset(View::Message).min(max_scroll);
        self.set_scroll_offset(View::Message, scroll);
        let widget = Paragraph::new(self.message.clone())
            .wrap(Wrap { trim: false })
            .scroll((scroll as u16, 0))
            .block(Block::default().borders(Borders::ALL).title("Message"));
        frame.render_widget(widget, area);
    }

    pub(in crate::tui) fn draw_log_panel(
        &self,
        frame: &mut ratatui::Frame,
        area: ratatui::layout::Rect,
    ) {
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
}

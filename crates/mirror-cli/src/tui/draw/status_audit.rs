use super::*;

impl TuiApp {
    pub(in crate::tui) fn draw_install_progress(
        &mut self,
        frame: &mut ratatui::Frame,
        area: ratatui::layout::Rect,
    ) {
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
                lines.push(Line::from(Span::raw(line.clone())));
            }
        } else {
            lines.push(Line::from(Span::raw("Starting installer...")));
        }
        let max_scroll = max_scroll_for_lines(lines.len(), area.height);
        let scroll = self.scroll_offset(View::InstallProgress).min(max_scroll);
        self.set_scroll_offset(View::InstallProgress, scroll);
        let widget = Paragraph::new(lines)
            .wrap(Wrap { trim: false })
            .scroll((scroll as u16, 0))
            .block(Block::default().borders(Borders::ALL).title("Setup"));
        frame.render_widget(widget, area);
    }

    pub(in crate::tui) fn draw_update_prompt(
        &mut self,
        frame: &mut ratatui::Frame,
        area: ratatui::layout::Rect,
    ) {
        let lines = self.message.lines().count().max(1);
        let max_scroll = max_scroll_for_lines(lines, area.height);
        let scroll = self.scroll_offset(View::UpdatePrompt).min(max_scroll);
        self.set_scroll_offset(View::UpdatePrompt, scroll);
        let widget = Paragraph::new(self.message.clone())
            .wrap(Wrap { trim: false })
            .scroll((scroll as u16, 0))
            .block(Block::default().borders(Borders::ALL).title("Update"));
        frame.render_widget(widget, area);
    }

    pub(in crate::tui) fn draw_update_progress(
        &mut self,
        frame: &mut ratatui::Frame,
        area: ratatui::layout::Rect,
    ) {
        let mut lines = vec![
            Line::from(Span::raw("Context: Updating... please wait")),
            Line::from(Span::raw("")),
        ];
        if let Some(progress) = &self.update_progress {
            for line in &progress.messages {
                lines.push(Line::from(Span::raw(line.clone())));
            }
        } else {
            lines.push(Line::from(Span::raw("Starting update...")));
        }
        let max_scroll = max_scroll_for_lines(lines.len(), area.height);
        let scroll = self.scroll_offset(View::UpdateProgress).min(max_scroll);
        self.set_scroll_offset(View::UpdateProgress, scroll);
        let widget = Paragraph::new(lines)
            .wrap(Wrap { trim: false })
            .scroll((scroll as u16, 0))
            .block(Block::default().borders(Borders::ALL).title("Update"));
        frame.render_widget(widget, area);
    }

    pub(in crate::tui) fn draw_install_status(
        &mut self,
        frame: &mut ratatui::Frame,
        area: ratatui::layout::Rect,
    ) {
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
        let max_scroll = max_scroll_for_lines(lines.len(), area.height);
        let scroll = self.scroll_offset(View::InstallStatus).min(max_scroll);
        self.set_scroll_offset(View::InstallStatus, scroll);
        let widget = Paragraph::new(lines)
            .wrap(Wrap { trim: false })
            .scroll((scroll as u16, 0))
            .block(Block::default().borders(Borders::ALL).title("Setup Status"));
        frame.render_widget(widget, area);
    }

    pub(in crate::tui) fn draw_sync_status(
        &mut self,
        frame: &mut ratatui::Frame,
        area: ratatui::layout::Rect,
    ) {
        let mut lines = vec![
            Line::from(Span::raw("Context: Sync status by target")),
            Line::from(Span::raw("")),
        ];
        match self.sync_status_lines() {
            Ok(status_lines) => {
                if status_lines.is_empty() {
                    lines.push(Line::from(Span::raw("No targets configured.")));
                } else {
                    for line in status_lines {
                        lines.push(Line::from(Span::raw(line)));
                    }
                }
            }
            Err(err) => {
                lines.push(Line::from(Span::raw(format!(
                    "Failed to load sync status: {err}"
                ))));
            }
        }
        let max_scroll = max_scroll_for_lines(lines.len(), area.height);
        let scroll = self.scroll_offset(View::SyncStatus).min(max_scroll);
        self.set_scroll_offset(View::SyncStatus, scroll);
        let widget = Paragraph::new(lines)
            .wrap(Wrap { trim: false })
            .scroll((scroll as u16, 0))
            .block(Block::default().borders(Borders::ALL).title("Sync Status"));
        frame.render_widget(widget, area);
    }

    pub(in crate::tui) fn draw_audit_log(
        &mut self,
        frame: &mut ratatui::Frame,
        area: ratatui::layout::Rect,
    ) {
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
        let scroll = self.scroll_offset(View::AuditLog).min(max_scroll);
        self.set_scroll_offset(View::AuditLog, scroll);
        let visible_lines = slice_with_scroll(&lines, scroll, body_height);
        let list_items: Vec<ListItem> = visible_lines
            .into_iter()
            .map(|line| ListItem::new(Line::from(Span::raw(line))))
            .collect();
        let list = List::new(list_items).block(Block::default().borders(Borders::ALL));
        frame.render_widget(list, layout[1]);
    }
}

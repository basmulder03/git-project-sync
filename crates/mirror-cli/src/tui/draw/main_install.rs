use super::*;
use crate::i18n::{active_locale, key, supported_locales, tr};

impl TuiApp {
    pub(in crate::tui) fn draw_main(
        &mut self,
        frame: &mut ratatui::Frame,
        area: ratatui::layout::Rect,
    ) {
        let status = crate::install::install_status().ok();
        let action = install_action_from_status(status.as_ref());
        let state = install_state_from_status(status.as_ref(), action);
        let items = vec![
            tr(key::DASHBOARD),
            state.menu_label().to_string(),
            "Config".to_string(),
            "Targets".to_string(),
            "Tokens".to_string(),
            "Service".to_string(),
            "Audit Log".to_string(),
            "Repo Overview".to_string(),
            "Update".to_string(),
            format!("{}: {}", tr(key::LANGUAGE_LABEL), active_locale().label()),
            "Quit".to_string(),
        ];
        let body_height = area.height.saturating_sub(2) as usize;
        let max_scroll = items.len().saturating_sub(body_height);
        let mut scroll = self.scroll_offset(View::Main).min(max_scroll);
        scroll = adjust_scroll(self.menu_index, scroll, body_height, items.len());
        self.set_scroll_offset(View::Main, scroll);
        let end = (scroll + body_height).min(items.len());
        let list_items: Vec<ListItem> = items[scroll..end]
            .iter()
            .enumerate()
            .map(|(offset, item)| {
                let idx = scroll + offset;
                let mut line = Line::from(Span::raw(item.as_str()));
                if idx == self.menu_index {
                    line = line.style(Style::default().add_modifier(Modifier::BOLD));
                }
                ListItem::new(line)
            })
            .collect();
        let list = List::new(list_items).block(
            Block::default()
                .borders(Borders::ALL)
                .title(tr(key::MAIN_MENU)),
        );
        frame.render_widget(list, area);
    }

    pub(in crate::tui) fn draw_dashboard(
        &mut self,
        frame: &mut ratatui::Frame,
        area: ratatui::layout::Rect,
    ) {
        let stats = self.dashboard_stats();
        let mut lines = vec![
            Line::from(Span::raw(tr(key::DASHBOARD_SYSTEM_STATUS))),
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
            lines.push(Line::from(Span::raw(tr(key::DASHBOARD_PER_TARGET))));
            for target in stats.targets {
                lines.push(Line::from(Span::raw(format!(
                    "{} | {} | {}",
                    target.id, target.status, target.last_success
                ))));
            }
        } else {
            lines.push(Line::from(Span::raw("")));
            lines.push(Line::from(Span::raw(tr(
                "Press t to show per-target status",
            ))));
        }
        let max_scroll = max_scroll_for_lines(lines.len(), area.height);
        let scroll = self.scroll_offset(View::Dashboard).min(max_scroll);
        self.set_scroll_offset(View::Dashboard, scroll);
        let widget = Paragraph::new(lines)
            .wrap(Wrap { trim: false })
            .scroll((scroll as u16, 0))
            .block(
                Block::default()
                    .borders(Borders::ALL)
                    .title(tr(key::DASHBOARD)),
            );
        frame.render_widget(widget, area);
    }

    pub(in crate::tui) fn draw_install(
        &mut self,
        frame: &mut ratatui::Frame,
        area: ratatui::layout::Rect,
    ) {
        let status = crate::install::install_status().ok();
        let action = install_action_from_status(status.as_ref());
        let mut lines = vec![
            Line::from(Span::raw(tr(
                "Context: Setup status with install/update actions",
            ))),
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
                lines.push(Line::from(Span::raw(tr(key::TASK_NAME))));
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
            lines.push(Line::from(Span::raw(tr(key::STATUS_UNAVAILABLE))));
            lines.push(Line::from(Span::raw("")));
        }
        lines.push(Line::from(Span::raw(tr(
            "Tip: Press u to check for updates.",
        ))));
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
            lines.push(Line::from(Span::raw(format!(
                "{} {message}",
                tr(key::VALIDATION_LABEL)
            ))));
        }
        let max_scroll = max_scroll_for_lines(lines.len(), area.height);
        let scroll = self.scroll_offset(View::Install).min(max_scroll);
        self.set_scroll_offset(View::Install, scroll);
        let widget = Paragraph::new(lines)
            .wrap(Wrap { trim: false })
            .scroll((scroll as u16, 0))
            .block(
                Block::default()
                    .borders(Borders::ALL)
                    .title(tr(key::SETUP_TITLE)),
            );
        frame.render_widget(widget, area);
    }

    pub(in crate::tui) fn draw_language(
        &mut self,
        frame: &mut ratatui::Frame,
        area: ratatui::layout::Rect,
    ) {
        let mut lines = vec![
            Line::from(Span::raw(tr(key::LANGUAGE_CONTEXT))),
            Line::from(Span::raw("")),
        ];
        for (idx, locale) in supported_locales().iter().enumerate() {
            let marker = if idx == self.language_index { ">" } else { " " };
            let selected = if *locale == active_locale() {
                " (active)"
            } else {
                ""
            };
            lines.push(Line::from(Span::raw(format!(
                "{marker} {} ({}){selected}",
                locale.label(),
                locale.as_bcp47()
            ))));
        }
        let max_scroll = max_scroll_for_lines(lines.len(), area.height);
        let scroll = self.scroll_offset(View::Language).min(max_scroll);
        self.set_scroll_offset(View::Language, scroll);
        let widget = Paragraph::new(lines)
            .wrap(Wrap { trim: false })
            .scroll((scroll as u16, 0))
            .block(
                Block::default()
                    .borders(Borders::ALL)
                    .title(tr(key::LANGUAGE_LABEL)),
            );
        frame.render_widget(widget, area);
    }
}

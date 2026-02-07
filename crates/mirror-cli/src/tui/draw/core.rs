use super::*;

impl TuiApp {
    pub(in crate::tui) fn draw(&mut self, frame: &mut ratatui::Frame) {
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

    pub(in crate::tui) fn footer_text(&self) -> String {
        match self.view {
            View::Main => "Up/Down: navigate | Enter: open | q: quit".to_string(),
            View::Dashboard => dashboard_footer_text().to_string(),
            View::Install => {
                let status = crate::install::install_status().ok();
                let action = install_action_from_status(status.as_ref());
                format!(
                    "Tab: next field | Enter: {} | s: status | u: updates | Esc: back | PgUp/PgDn/Home/End: scroll",
                    action.verb()
                )
            }
            View::UpdatePrompt => {
                "y: apply update | n: cancel | Esc: back | PgUp/PgDn/Home/End: scroll".to_string()
            }
            View::UpdateProgress => "Updating... | Esc: back | PgUp/PgDn/Home/End: scroll".to_string(),
            View::SyncStatus => "Enter: refresh focus | Esc: back | PgUp/PgDn/Home/End: scroll".to_string(),
            View::InstallStatus => "Esc: back | PgUp/PgDn/Home/End: scroll".to_string(),
            View::ConfigRoot => "Enter: save | Esc: back | PgUp/PgDn/Home/End: scroll".to_string(),
            View::RepoOverview => {
                "Up/Down: scroll | PgUp/PgDn | Enter: collapse | c: compact | r: refresh | Esc: back"
                    .to_string()
            }
            View::Targets => "a: add | d: remove | Esc: back | PgUp/PgDn/Home/End: scroll".to_string(),
            View::TargetAdd | View::TargetRemove | View::TokenSet | View::TokenValidate => {
                "Tab: next field | Enter: submit | Esc: back | PgUp/PgDn/Home/End: scroll".to_string()
            }
            View::TokenMenu => "Up/Down: navigate | Enter: select | Esc: back".to_string(),
            View::TokenList => "Esc: back | PgUp/PgDn/Home/End: scroll".to_string(),
            View::Service => "i: install | u: uninstall | Esc: back | PgUp/PgDn/Home/End: scroll".to_string(),
            View::AuditLog => {
                "Up/Down: scroll | PgUp/PgDn | /: search | f: failures | a: all | Esc: back"
                    .to_string()
            }
            View::Message => "Enter/Esc: back | PgUp/PgDn/Home/End: scroll".to_string(),
            View::InstallProgress => "Applying setup... | Esc: back | PgUp/PgDn/Home/End: scroll".to_string(),
        }
    }
}

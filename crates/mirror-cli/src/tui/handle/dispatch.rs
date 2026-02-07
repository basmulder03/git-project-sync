use super::*;

impl TuiApp {
    pub(in crate::tui) fn handle_key(&mut self, key: KeyEvent) -> anyhow::Result<bool> {
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
}

use super::*;

impl TuiApp {
    pub(in crate::tui) fn handle_message(&mut self, key: KeyEvent) -> anyhow::Result<bool> {
        if key.code == KeyCode::Enter {
            self.go_back();
        }
        Ok(false)
    }

    pub(in crate::tui) fn handle_audit_log(&mut self, key: KeyEvent) -> anyhow::Result<bool> {
        if self.audit_search_active {
            match key.code {
                KeyCode::Esc => {
                    self.audit_search.clear();
                    self.audit_search_active = false;
                    self.set_scroll_offset(View::AuditLog, 0);
                }
                KeyCode::Enter => {
                    self.audit_search_active = false;
                    self.set_scroll_offset(View::AuditLog, 0);
                }
                KeyCode::Backspace => {
                    self.audit_search.pop();
                }
                KeyCode::Char(ch) => {
                    if !ch.is_control() {
                        self.audit_search.push(ch);
                    }
                }
                _ => {}
            }
            return Ok(false);
        }

        match key.code {
            KeyCode::Esc => self.view = View::Main,
            KeyCode::Char('f') => {
                self.audit_filter = AuditFilter::Failures;
                self.set_scroll_offset(View::AuditLog, 0);
            }
            KeyCode::Char('a') => {
                self.audit_filter = AuditFilter::All;
                self.set_scroll_offset(View::AuditLog, 0);
            }
            KeyCode::Char('/') => {
                self.audit_search_active = true;
            }
            KeyCode::Down => {
                self.scroll_by(View::AuditLog, 1);
            }
            KeyCode::Up => {
                self.scroll_by(View::AuditLog, -1);
            }
            KeyCode::PageDown => {
                self.scroll_by(View::AuditLog, 10);
            }
            KeyCode::PageUp => {
                self.scroll_by(View::AuditLog, -10);
            }
            KeyCode::Home => {
                self.set_scroll_offset(View::AuditLog, 0);
            }
            _ => {}
        }
        Ok(false)
    }

    pub(in crate::tui) fn handle_service(&mut self, key: KeyEvent) -> anyhow::Result<bool> {
        match key.code {
            KeyCode::Esc => self.view = View::Main,
            KeyCode::Char('i') => {
                let exe = std::env::current_exe().context("resolve current executable")?;
                info!("Service install requested");
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
                        error!(error = %err, "Service install failed");
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
                self.navigate_to(View::Message);
            }
            KeyCode::Char('u') => {
                info!("Service uninstall requested");
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
                        error!(error = %err, "Service uninstall failed");
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
                self.navigate_to(View::Message);
            }
            _ => {}
        }
        Ok(false)
    }

    pub(in crate::tui) fn handle_install_progress(
        &mut self,
        _key: KeyEvent,
    ) -> anyhow::Result<bool> {
        Ok(false)
    }

    pub(in crate::tui) fn handle_update_prompt(&mut self, key: KeyEvent) -> anyhow::Result<bool> {
        match key.code {
            KeyCode::Char('y') | KeyCode::Char('Y') => {
                if let Some(check) = self.update_prompt.take() {
                    self.start_update_apply(check)?;
                } else {
                    self.message = "No update available.".to_string();
                    self.message_return_view = self.update_return_view;
                    self.navigate_to(View::Message);
                }
            }
            KeyCode::Char('n') | KeyCode::Char('N') | KeyCode::Esc => {
                self.update_prompt = None;
                self.view = self.update_return_view;
            }
            _ => {}
        }
        Ok(false)
    }

    pub(in crate::tui) fn handle_update_progress(
        &mut self,
        _key: KeyEvent,
    ) -> anyhow::Result<bool> {
        Ok(false)
    }

    pub(in crate::tui) fn handle_install_status(&mut self, key: KeyEvent) -> anyhow::Result<bool> {
        match key.code {
            KeyCode::Enter | KeyCode::Esc => {
                self.set_scroll_offset(View::Install, 0);
                self.navigate_to(View::Install);
            }
            KeyCode::Down => self.scroll_by(View::InstallStatus, 1),
            KeyCode::Up => self.scroll_by(View::InstallStatus, -1),
            _ => {}
        }
        Ok(false)
    }

    pub(in crate::tui) fn handle_sync_status(&mut self, key: KeyEvent) -> anyhow::Result<bool> {
        match key.code {
            KeyCode::Enter | KeyCode::Esc => self.view = View::Dashboard,
            _ => {}
        }
        Ok(false)
    }
}

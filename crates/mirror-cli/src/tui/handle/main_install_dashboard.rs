use super::*;

impl TuiApp {
    pub(in crate::tui) fn handle_main(&mut self, key: KeyEvent) -> anyhow::Result<bool> {
        match key.code {
            KeyCode::Char('q') => return Ok(true),
            KeyCode::Down => self.menu_index = (self.menu_index + 1) % 10,
            KeyCode::Up => {
                if self.menu_index == 0 {
                    self.menu_index = 9;
                } else {
                    self.menu_index -= 1;
                }
            }
            KeyCode::Enter => {
                info!(selection = self.menu_index, "Main menu selected");
                match self.menu_index {
                    0 => {
                        info!("Switching to dashboard view");
                        self.navigate_to(View::Dashboard);
                    }
                    1 => {
                        if let Err(err) = self.enter_install_view() {
                            error!(error = %err, "Setup unavailable");
                            self.message = format!("Setup unavailable: {err}");
                            self.navigate_to(View::Message);
                        }
                    }
                    2 => {
                        info!("Switching to config root view");
                        self.navigate_to(View::ConfigRoot);
                        self.input_fields = vec![InputField::new("Root path")];
                        self.input_index = 0;
                    }
                    3 => {
                        info!("Switching to targets view");
                        self.navigate_to(View::Targets);
                    }
                    4 => {
                        info!("Switching to token menu view");
                        self.navigate_to(View::TokenMenu);
                        self.token_menu_index = 0;
                    }
                    5 => {
                        info!("Switching to service view");
                        self.navigate_to(View::Service);
                    }
                    6 => {
                        info!("Switching to audit log view");
                        self.navigate_to(View::AuditLog);
                        self.set_scroll_offset(View::AuditLog, 0);
                        self.audit_search_active = false;
                    }
                    7 => {
                        if let Err(err) = self.enter_repo_overview() {
                            error!(error = %err, "Repo overview unavailable");
                            self.message = format!("Repo overview unavailable: {err}");
                            self.navigate_to(View::Message);
                        }
                    }
                    8 => {
                        info!("Starting update check from main menu");
                        self.start_update_check(View::Main)?;
                    }
                    _ => return Ok(true),
                }
            }
            _ => {}
        }
        Ok(false)
    }

    pub(in crate::tui) fn handle_install(&mut self, key: KeyEvent) -> anyhow::Result<bool> {
        match key.code {
            KeyCode::Esc => {
                self.release_install_guard();
                self.view = View::Main;
            }
            KeyCode::Tab => self.input_index = (self.input_index + 1) % self.input_fields.len(),
            KeyCode::Down => self.scroll_by(View::Install, 1),
            KeyCode::Up => self.scroll_by(View::Install, -1),
            KeyCode::Enter => {
                if let Err(err) = self.ensure_install_guard() {
                    error!(error = %err, "Setup lock unavailable");
                    self.message = format!("Setup unavailable: {err}");
                    self.navigate_to(View::Message);
                    return Ok(false);
                }
                let delay_raw = self.input_fields[0].value.trim();
                let delayed_start = if delay_raw.is_empty() {
                    None
                } else {
                    match delay_raw.parse::<u64>() {
                        Ok(value) => Some(value),
                        Err(_) => {
                            warn!(value = delay_raw, "Invalid delayed start input");
                            self.validation_message =
                                Some("Delayed start must be a number.".to_string());
                            return Ok(false);
                        }
                    }
                };
                let path_raw = self.input_fields[1].value.trim().to_lowercase();
                let path_choice = if path_raw == "y" || path_raw == "yes" {
                    crate::install::PathChoice::Add
                } else {
                    crate::install::PathChoice::Skip
                };
                let exec = std::env::current_exe().context("resolve current executable")?;
                info!(
                    delayed_start = delayed_start,
                    path_choice = ?path_choice,
                    "Starting install"
                );
                let (tx, rx) = mpsc::channel::<InstallEvent>();
                thread::spawn(move || {
                    let result = crate::install::perform_install_with_progress(
                        &exec,
                        crate::install::InstallOptions {
                            delayed_start,
                            path_choice,
                        },
                        Some(&|progress| {
                            let _ = tx.send(InstallEvent::Progress(progress));
                        }),
                        None,
                    )
                    .map_err(|err| err.to_string());
                    let _ = tx.send(InstallEvent::Done(result));
                });
                self.install_rx = Some(rx);
                self.install_progress = None;
                self.navigate_to(View::InstallProgress);
            }
            KeyCode::Char('s') => {
                self.install_status = crate::install::install_status().ok();
                self.set_scroll_offset(View::InstallStatus, 0);
                self.navigate_to(View::InstallStatus);
            }
            KeyCode::Char('u') => {
                self.start_update_check(View::Install)?;
            }
            _ => self.handle_text_input(key),
        }
        Ok(false)
    }

    pub(in crate::tui) fn handle_dashboard(&mut self, key: KeyEvent) -> anyhow::Result<bool> {
        match key.code {
            KeyCode::Esc => self.view = View::Main,
            KeyCode::Char('t') => {
                self.show_target_stats = !self.show_target_stats;
                debug!(
                    show_target_stats = self.show_target_stats,
                    "Toggled target stats"
                );
            }
            KeyCode::Char('s') => {
                self.navigate_to(View::SyncStatus);
            }
            KeyCode::Char('r') => {
                self.start_sync_run(false)?;
            }
            KeyCode::Char('f') => {
                self.start_sync_run(true)?;
            }
            KeyCode::Char('u') => {
                self.start_update_check(View::Dashboard)?;
            }
            _ => {}
        }
        Ok(false)
    }
}

use super::*;
use crate::i18n::{key, tf, tr};

impl TuiApp {
    pub(in crate::tui) fn handle_config_root(&mut self, key: KeyEvent) -> anyhow::Result<bool> {
        match key.code {
            KeyCode::Esc => self.view = View::Main,
            KeyCode::Enter => {
                let value = self
                    .input_fields
                    .first()
                    .map(|f| f.value.trim().to_string());
                if let Some(path) = value {
                    if path.is_empty() {
                        warn!("Root path empty in config root view");
                        self.validation_message = Some(tr(key::ROOT_PATH_EMPTY).to_string());
                        self.message = tr(key::ROOT_PATH_EMPTY).to_string();
                        self.navigate_to(View::Message);
                        return Ok(false);
                    }
                    self.config.root = Some(path.into());
                    self.config.save(&self.config_path)?;
                    self.validation_message = None;
                    info!("Saved config root from TUI");
                    let audit_id = self.audit.record(
                        "tui.config.root",
                        AuditStatus::Ok,
                        Some("tui"),
                        None,
                        None,
                    )?;
                    self.message = tf(
                        "Root saved. Audit ID: {audit_id}",
                        &[("audit_id", audit_id)],
                    );
                    self.navigate_to(View::Message);
                }
            }
            _ => {
                self.handle_text_input(key);
            }
        }
        Ok(false)
    }

    pub(in crate::tui) fn handle_repo_overview(&mut self, key: KeyEvent) -> anyhow::Result<bool> {
        match key.code {
            KeyCode::Esc => self.view = View::Main,
            KeyCode::Char('r') => {
                self.start_repo_status_refresh()?;
            }
            KeyCode::Char('c') => {
                self.repo_overview_compact = !self.repo_overview_compact;
                debug!(
                    compact = self.repo_overview_compact,
                    "Toggled repo overview compact mode"
                );
            }
            KeyCode::Down => {
                self.repo_overview_selected = self.repo_overview_selected.saturating_add(1);
            }
            KeyCode::Up => {
                self.repo_overview_selected = self.repo_overview_selected.saturating_sub(1);
            }
            KeyCode::PageDown => {
                self.repo_overview_selected = self.repo_overview_selected.saturating_add(10);
            }
            KeyCode::PageUp => {
                self.repo_overview_selected = self.repo_overview_selected.saturating_sub(10);
            }
            KeyCode::Home => {
                self.repo_overview_selected = 0;
            }
            KeyCode::End => {
                self.repo_overview_selected = usize::MAX;
            }
            KeyCode::Enter => {
                let rows = self.current_overview_rows();
                let visible = self.visible_overview_rows(&rows);
                if let Some(row) = visible.get(self.repo_overview_selected)
                    && !row.is_leaf
                {
                    if self.repo_overview_collapsed.contains(&row.id) {
                        self.repo_overview_collapsed.remove(&row.id);
                    } else {
                        self.repo_overview_collapsed.insert(row.id.clone());
                    }
                }
            }
            _ => {}
        }
        Ok(false)
    }

    pub(in crate::tui) fn handle_targets(&mut self, key: KeyEvent) -> anyhow::Result<bool> {
        match key.code {
            KeyCode::Esc => self.view = View::Main,
            KeyCode::Char('a') => {
                self.validation_message = None;
                self.navigate_to(View::TargetAdd);
                self.input_fields = vec![
                    InputField::new(tr(key::LABEL_SCOPE_SPACED)),
                    InputField::new(tr(key::LABEL_HOST_OPTIONAL)),
                    InputField::new(tr(key::LABEL_LABELS_COMMA)),
                ];
                self.input_index = 0;
                self.provider_index = 0;
            }
            KeyCode::Char('d') => {
                self.validation_message = None;
                self.navigate_to(View::TargetRemove);
                self.input_fields = vec![InputField::new(tr(key::LABEL_TARGET_ID))];
                self.input_index = 0;
            }
            _ => {}
        }
        Ok(false)
    }

    pub(in crate::tui) fn handle_target_add(&mut self, key: KeyEvent) -> anyhow::Result<bool> {
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
                let scope = match spec.parse_scope(
                    scope_raw
                        .split_whitespace()
                        .map(|s| s.to_string())
                        .collect(),
                ) {
                    Ok(scope) => {
                        self.validation_message = None;
                        scope
                    }
                    Err(err) => {
                        warn!(error = %err, "Invalid scope for target add");
                        self.validation_message = Some(format!("Scope invalid: {err}"));
                        return Ok(false);
                    }
                };
                let host = optional_text(&self.input_fields[1].value);
                let labels = split_labels(&self.input_fields[2].value);
                let id = target_id(provider.clone(), host.as_deref(), &scope);
                if self.config.targets.iter().any(|t| t.id == id) {
                    warn!(target_id = %id, "Target already exists");
                    self.validation_message = Some("Target already exists.".to_string());
                    self.message = "Target already exists.".to_string();
                    self.navigate_to(View::Message);
                    let audit_id = self.audit.record_with_context(
                        "tui.target.add",
                        AuditStatus::Skipped,
                        Some("tui"),
                        AuditContext {
                            provider: Some(provider.as_prefix().to_string()),
                            scope: Some(scope.segments().join("/")),
                            repo_id: None,
                            path: None,
                        },
                        None,
                        Some("target already exists"),
                    )?;
                    self.message = tf(
                        "Target already exists. Audit ID: {audit_id}",
                        &[("audit_id", audit_id)],
                    );
                    self.navigate_to(View::Message);
                    return Ok(false);
                }
                let scope_label = scope.segments().join("/");
                self.config.targets.push(TargetConfig {
                    id: id.clone(),
                    provider: provider.clone(),
                    scope: scope.clone(),
                    host,
                    labels,
                });
                self.config.save(&self.config_path)?;
                self.validation_message = None;
                info!(target_id = %id, provider = %provider.as_prefix(), "Target added");
                let audit_id = self.audit.record_with_context(
                    "tui.target.add",
                    AuditStatus::Ok,
                    Some("tui"),
                    AuditContext {
                        provider: Some(provider.as_prefix().to_string()),
                        scope: Some(scope_label),
                        repo_id: Some(id.clone()),
                        path: None,
                    },
                    None,
                    None,
                )?;
                self.message = tf(
                    "Target added. Audit ID: {audit_id}",
                    &[("audit_id", audit_id)],
                );
                self.navigate_to(View::Message);
            }
            _ => self.handle_text_input(key),
        }
        Ok(false)
    }

    pub(in crate::tui) fn handle_target_remove(&mut self, key: KeyEvent) -> anyhow::Result<bool> {
        match key.code {
            KeyCode::Esc => self.view = View::Targets,
            KeyCode::Enter => {
                let id = self.input_fields[0].value.trim().to_string();
                if id.is_empty() {
                    warn!("Target remove attempted without id");
                    self.validation_message = Some("Target id is required.".to_string());
                    self.message = "Target id required.".to_string();
                    self.navigate_to(View::Message);
                    return Ok(false);
                }
                let before = self.config.targets.len();
                self.config.targets.retain(|t| t.id != id);
                let after = self.config.targets.len();
                if before == after {
                    warn!(target_id = %id, "Target not found for removal");
                    self.validation_message = Some("Target not found.".to_string());
                    let audit_id = self.audit.record(
                        "tui.target.remove",
                        AuditStatus::Skipped,
                        Some("tui"),
                        None,
                        Some("target not found"),
                    )?;
                    self.message = tf(
                        "No target found. Audit ID: {audit_id}",
                        &[("audit_id", audit_id)],
                    );
                } else {
                    self.config.save(&self.config_path)?;
                    self.validation_message = None;
                    info!(target_id = %id, "Target removed");
                    let audit_id = self.audit.record(
                        "tui.target.remove",
                        AuditStatus::Ok,
                        Some("tui"),
                        None,
                        None,
                    )?;
                    self.message = tf(
                        "Target removed. Audit ID: {audit_id}",
                        &[("audit_id", audit_id)],
                    );
                }
                self.navigate_to(View::Message);
            }
            _ => self.handle_text_input(key),
        }
        Ok(false)
    }
}

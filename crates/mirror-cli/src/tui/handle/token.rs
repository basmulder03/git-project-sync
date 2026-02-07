use super::*;

impl TuiApp {
    pub(in crate::tui) fn handle_token_menu(&mut self, key: KeyEvent) -> anyhow::Result<bool> {
        match key.code {
            KeyCode::Esc => self.view = View::Main,
            KeyCode::Down => self.token_menu_index = (self.token_menu_index + 1) % 4,
            KeyCode::Up => {
                if self.token_menu_index == 0 {
                    self.token_menu_index = 3;
                } else {
                    self.token_menu_index -= 1;
                }
            }
            KeyCode::Enter => match self.token_menu_index {
                0 => self.view = View::TokenList,
                1 => {
                    self.validation_message = None;
                    self.view = View::TokenSet;
                    self.input_fields = vec![
                        InputField::new("Scope (space-separated)"),
                        InputField::new("Host (optional)"),
                        InputField::with_mask("Token"),
                    ];
                    self.input_index = 0;
                    self.provider_index = 0;
                }
                2 => {
                    self.validation_message = None;
                    self.view = View::TokenValidate;
                    self.input_fields = vec![
                        InputField::new("Scope (space-separated)"),
                        InputField::new("Host (optional)"),
                    ];
                    self.input_index = 0;
                    self.provider_index = 0;
                }
                _ => self.view = View::Main,
            },
            _ => {}
        }
        Ok(false)
    }

    pub(in crate::tui) fn handle_token_list(&mut self, key: KeyEvent) -> anyhow::Result<bool> {
        if key.code == KeyCode::Esc {
            self.view = View::TokenMenu
        }
        Ok(false)
    }

    pub(in crate::tui) fn handle_token_set(&mut self, key: KeyEvent) -> anyhow::Result<bool> {
        match key.code {
            KeyCode::Esc => self.view = View::TokenMenu,
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
                        warn!(error = %err, "Invalid scope for token set");
                        self.validation_message = Some(format!("Scope invalid: {err}"));
                        return Ok(false);
                    }
                };
                let host = optional_text(&self.input_fields[1].value);
                let host = host_or_default(host.as_deref(), spec.as_ref());
                let account = spec.account_key(&host, &scope)?;
                let token = self.input_fields[2].value.trim().to_string();
                if token.is_empty() {
                    warn!(account = %account, "Token missing in token set");
                    self.validation_message = Some("Token cannot be empty.".to_string());
                    self.message = "Token cannot be empty.".to_string();
                    self.view = View::Message;
                    return Ok(false);
                }
                auth::set_pat(&account, &token)?;
                if let Err(err) =
                    auth::get_pat(&account).context("read token from keyring after write")
                {
                    let _ = auth::delete_pat(&account);
                    let message = format!("Token storage failed: {err:#}");
                    warn!(account = %account, error = %message, "Token read-back failed");
                    self.validation_message = Some(message.clone());
                    self.message = message;
                    self.view = View::Message;
                    return Ok(false);
                }
                let runtime_target = mirror_core::model::ProviderTarget {
                    provider: provider.clone(),
                    scope: scope.clone(),
                    host: Some(host.clone()),
                };
                let validity = mirror_core::provider::block_on(
                    crate::token_check::check_token_validity_async(&runtime_target),
                );
                if validity.status != crate::token_check::TokenValidity::Ok {
                    let _ = auth::delete_pat(&account);
                    let mut message = validity.message(&runtime_target);
                    if let Some(error) = validity.error.as_deref() {
                        message.push_str(": ");
                        message.push_str(error);
                    }
                    warn!(account = %account, status = ?validity.status, "Token validity check failed");
                    self.validation_message = Some(message.clone());
                    self.message = message;
                    self.view = View::Message;
                    return Ok(false);
                }
                let validation = mirror_core::provider::block_on(self.validate_token(
                    provider.clone(),
                    scope.clone(),
                    Some(host.clone()),
                ));
                let validation_message = match &validation {
                    Ok(record) => {
                        self.token_validation
                            .insert(account.clone(), record.clone());
                        record.display()
                    }
                    Err(err) => format!("validation failed: {err}"),
                };
                self.validation_message = None;
                let audit_id = self.audit.record_with_context(
                    "tui.token.set",
                    AuditStatus::Ok,
                    Some("tui"),
                    AuditContext {
                        provider: Some(provider.as_prefix().to_string()),
                        scope: Some(scope.segments().join("/")),
                        repo_id: None,
                        path: None,
                    },
                    None,
                    None,
                )?;
                info!(account = %account, provider = %provider.as_prefix(), "Token stored");
                self.message = format!(
                    "Token stored for {account}. {validation_message}. Audit ID: {audit_id}"
                );
                self.view = View::Message;
            }
            _ => self.handle_text_input(key),
        }
        Ok(false)
    }

    pub(in crate::tui) fn handle_token_validate(&mut self, key: KeyEvent) -> anyhow::Result<bool> {
        match key.code {
            KeyCode::Esc => self.view = View::TokenMenu,
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
                        warn!(error = %err, "Invalid scope for token validation");
                        self.validation_message = Some(format!("Scope invalid: {err}"));
                        return Ok(false);
                    }
                };
                let host = optional_text(&self.input_fields[1].value);
                let host = host_or_default(host.as_deref(), spec.as_ref());
                let account = spec.account_key(&host, &scope)?;
                let validation = mirror_core::provider::block_on(self.validate_token(
                    provider.clone(),
                    scope.clone(),
                    Some(host.clone()),
                ));
                match validation {
                    Ok(record) => {
                        self.token_validation
                            .insert(account.clone(), record.clone());
                        let status = record.display();
                        self.validation_message = None;
                        let audit_status = match record.status {
                            TokenValidationStatus::Ok => AuditStatus::Ok,
                            TokenValidationStatus::MissingScopes(_) => AuditStatus::Failed,
                            TokenValidationStatus::Unsupported => AuditStatus::Ok,
                        };
                        let audit_detail = match &record.status {
                            TokenValidationStatus::Unsupported => {
                                Some("auth-based validation used (scope validation not supported)")
                            }
                            _ => None,
                        };
                        let audit_id = self.audit.record_with_context(
                            "tui.token.validate",
                            audit_status,
                            Some("tui"),
                            AuditContext {
                                provider: Some(provider.as_prefix().to_string()),
                                scope: Some(scope.segments().join("/")),
                                repo_id: None,
                                path: None,
                            },
                            None,
                            audit_detail,
                        )?;
                        info!(account = %account, status = ?record.status, "Token validation completed");
                        self.message = format!("{status}. Audit ID: {audit_id}");
                    }
                    Err(err) => {
                        error!(error = %err, "Token validation failed");
                        self.validation_message = Some(format!("Validation failed: {err}"));
                        let _ = self.audit.record_with_context(
                            "tui.token.validate",
                            AuditStatus::Failed,
                            Some("tui"),
                            AuditContext {
                                provider: Some(provider.as_prefix().to_string()),
                                scope: Some(scope.segments().join("/")),
                                repo_id: None,
                                path: None,
                            },
                            None,
                            Some(&err.to_string()),
                        );
                        self.message = format!("Validation failed: {err}");
                    }
                }
                self.view = View::Message;
            }
            _ => self.handle_text_input(key),
        }
        Ok(false)
    }
}

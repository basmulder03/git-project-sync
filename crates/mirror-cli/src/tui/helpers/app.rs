use super::*;

impl TuiApp {
    pub(in crate::tui) fn handle_text_input(&mut self, key: KeyEvent) {
        if self.input_fields.is_empty() {
            return;
        }
        let field = &mut self.input_fields[self.input_index];
        match key.code {
            KeyCode::Char('c') if key.modifiers.contains(KeyModifiers::CONTROL) => {
                field.value.clear();
                self.validation_message = None;
            }
            KeyCode::Backspace => {
                field.pop();
                self.validation_message = None;
            }
            KeyCode::Char(ch) => {
                field.push(ch);
                self.validation_message = None;
            }
            _ => {}
        }
    }

    pub(in crate::tui) fn audit_log_path(&self) -> std::path::PathBuf {
        let base_dir = self.audit.base_dir();
        let date = ::time::OffsetDateTime::now_utc()
            .format(&::time::format_description::parse("[year][month][day]").unwrap())
            .unwrap();
        base_dir.join(format!("audit-{date}.jsonl"))
    }

    pub(in crate::tui) fn form_context(&self) -> Option<&'static str> {
        match self.view {
            View::TargetAdd => Some("Context: Add a provider target"),
            View::TargetRemove => Some("Context: Remove a target by id"),
            View::TokenSet => Some("Context: Store a token for a provider scope"),
            View::TokenValidate => Some("Context: Validate token scopes"),
            _ => None,
        }
    }

    pub(in crate::tui) fn form_hint(&self) -> Option<&'static str> {
        match self.view {
            View::TargetAdd => {
                let provider = provider_kind(self.provider_index);
                Some(provider_scope_hint(provider))
            }
            View::TargetRemove => Some("Find target ids on the Targets screen"),
            View::TokenSet | View::TokenValidate => {
                let provider = provider_kind(self.provider_index);
                Some(provider_scope_hint_with_host(provider))
            }
            _ => None,
        }
    }

    pub(in crate::tui) fn token_entries(&self) -> Vec<TokenEntry> {
        let mut entries = Vec::new();
        let mut seen = HashSet::new();
        for target in &self.config.targets {
            let spec = spec_for(target.provider.clone());
            let host = host_or_default(target.host.as_deref(), spec.as_ref());
            let account = match spec.account_key(&host, &target.scope) {
                Ok(account) => account,
                Err(_) => continue,
            };
            if !seen.insert(account.clone()) {
                continue;
            }
            let present = auth::get_pat(&account).is_ok();
            let validation = self.token_validation.get(&account).cloned();
            entries.push(TokenEntry {
                account,
                provider: target.provider.clone(),
                scope: target.scope.segments().join("/"),
                host,
                present,
                validation,
            });
        }
        entries
    }

    pub(in crate::tui) fn validate_token(
        &self,
        provider: ProviderKind,
        scope: mirror_core::model::ProviderScope,
        host: Option<String>,
    ) -> anyhow::Result<TokenValidation> {
        let runtime_target = mirror_core::model::ProviderTarget {
            provider: provider.clone(),
            scope: scope.clone(),
            host,
        };
        let registry = ProviderRegistry::new();
        let adapter = registry.provider(provider.clone())?;
        let scopes = adapter.token_scopes(&runtime_target)?;
        let help = pat_help(provider.clone());
        let status = match scopes {
            Some(scopes) => {
                let missing: Vec<&str> = help
                    .scopes
                    .iter()
                    .copied()
                    .filter(|required| !scopes.iter().any(|s| s == required))
                    .collect();
                if missing.is_empty() {
                    TokenValidationStatus::Ok
                } else {
                    TokenValidationStatus::MissingScopes(
                        missing.iter().map(|s| s.to_string()).collect(),
                    )
                }
            }
            None => {
                let token_check_result = crate::token_check::check_token_validity(&runtime_target);
                crate::token_check::ensure_token_valid(&token_check_result, &runtime_target)
                    .context(
                        "Auth-based token validation failed; verify your token is valid and not expired",
                    )?;
                TokenValidationStatus::Unsupported
            }
        };
        Ok(TokenValidation {
            status,
            at: validation_timestamp(),
        })
    }

    pub(in crate::tui) fn dashboard_stats(&self) -> DashboardStats {
        let cache_path = default_cache_path().ok();
        let cache = cache_path
            .as_ref()
            .and_then(|path| mirror_core::cache::RepoCache::load(path).ok());
        let audit_entries = self.audit_log_count();
        let now = current_epoch_seconds();
        let mut healthy = 0;
        let mut backoff = 0;
        let mut no_success = 0;
        let mut last_sync: Option<String> = None;
        let mut targets = Vec::new();

        for target in &self.config.targets {
            let id = target.id.clone();
            let last_success = cache
                .as_ref()
                .and_then(|c| c.target_last_success.get(&id).copied());
            let backoff_until = cache
                .as_ref()
                .and_then(|c| c.target_backoff_until.get(&id).copied());
            let status = if let Some(until) = backoff_until {
                if until > now {
                    backoff += 1;
                    "backoff"
                } else {
                    healthy += 1;
                    "ok"
                }
            } else if last_success.is_some() {
                healthy += 1;
                "ok"
            } else {
                no_success += 1;
                "unknown"
            };

            let last_success_label = last_success
                .map(epoch_to_label)
                .unwrap_or_else(|| "none".to_string());
            if last_sync.is_none() {
                last_sync = last_success.map(epoch_to_label);
            }

            targets.push(DashboardTarget {
                id,
                status: status.to_string(),
                last_success: last_success_label,
            });
        }

        DashboardStats {
            total_targets: self.config.targets.len(),
            healthy_targets: healthy,
            backoff_targets: backoff,
            no_success_targets: no_success,
            last_sync,
            audit_entries,
            targets,
        }
    }

    pub(in crate::tui) fn sync_status_lines(&self) -> anyhow::Result<Vec<Line<'_>>> {
        let cache_path = default_cache_path()?;
        let cache = RepoCache::load(&cache_path).unwrap_or_default();
        let empty_summary = SyncSummarySnapshot::default();
        let mut lines = Vec::new();
        for target in &self.config.targets {
            let label = format!(
                "{} | {}",
                target.provider.as_prefix(),
                target.scope.segments().join("/")
            );
            let status = cache.target_sync_status.get(&target.id);
            let state = status
                .map(|s| if s.in_progress { "running" } else { "idle" })
                .unwrap_or("idle");
            let action = status
                .and_then(|s| s.last_action.as_deref())
                .unwrap_or("unknown");
            let repo = status.and_then(|s| s.last_repo.as_deref()).unwrap_or("-");
            let updated = status
                .map(|s| epoch_to_label(s.last_updated))
                .unwrap_or_else(|| "unknown".to_string());
            let summary = status.map(|s| &s.summary).unwrap_or(&empty_summary);
            let total = status.map(|s| s.total_repos).unwrap_or(0);
            let processed = status.map(|s| s.processed_repos).unwrap_or(0);
            let bar = progress_bar(processed.min(total), total, 20);
            let error = last_sync_error(&self.audit, &target.id).unwrap_or_default();
            lines.push(Line::from(Span::raw(format!(
                "{} | {} | {} | {} | {}",
                label, state, action, repo, updated
            ))));
            lines.push(Line::from(Span::raw(format!(
                "progress: {}/{} {}",
                processed, total, bar
            ))));
            lines.push(Line::from(Span::raw(format!(
                "counts: cl={} ff={} up={} dirty={} div={} fail={} missA={} missR={} missS={}",
                summary.cloned,
                summary.fast_forwarded,
                summary.up_to_date,
                summary.dirty,
                summary.diverged,
                summary.failed,
                summary.missing_archived,
                summary.missing_removed,
                summary.missing_skipped
            ))));
            if !error.is_empty() {
                lines.push(Line::from(Span::raw(format!("last error: {error}"))));
            }
            lines.push(Line::from(Span::raw("")));
        }
        Ok(lines)
    }

    pub(in crate::tui) fn audit_log_count(&self) -> usize {
        let path = self.audit_log_path();
        if let Ok(contents) = std::fs::read_to_string(path) {
            return contents.lines().count();
        }
        0
    }
}

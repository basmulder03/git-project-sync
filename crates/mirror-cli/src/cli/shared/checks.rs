use super::*;
pub(in crate::cli) fn should_run_cli_update_check(command: &Commands, check_updates: bool) -> bool {
    !check_updates && !matches!(command, Commands::Update(_) | Commands::Install(_))
}

pub(in crate::cli) fn stdin_is_tty() -> bool {
    std::io::stdin().is_terminal()
}

pub(in crate::cli) fn stdout_is_tty() -> bool {
    std::io::stdout().is_terminal()
}

pub(in crate::cli) fn current_epoch_seconds() -> u64 {
    use std::time::{SystemTime, UNIX_EPOCH};
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap_or_default()
        .as_secs()
}

pub(in crate::cli) fn daemon_backoff_delay(
    interval: std::time::Duration,
    failures: u32,
) -> std::time::Duration {
    if failures == 0 {
        return interval;
    }
    let base = interval.as_secs().max(1);
    let exp = failures.saturating_sub(1).min(5);
    let delay = base.saturating_mul(2u64.saturating_pow(exp));
    std::time::Duration::from_secs(delay.min(3600))
}

pub(in crate::cli) fn should_run_cli_token_check(command: &Commands) -> bool {
    !matches!(command, Commands::Update(_) | Commands::Install(_))
}

pub(in crate::cli) fn run_token_validity_checks(
    config_path: &Path,
    cache_path: &Path,
    audit: &AuditLogger,
    source: &str,
    force: bool,
) -> anyhow::Result<()> {
    let now = current_epoch_seconds();
    let mut cache = RepoCache::load(cache_path).unwrap_or_default();
    if !force && !mirror_core::cache::token_check_due(&cache, now, 86_400) {
        return Ok(());
    }

    let (config, migrated) = load_or_migrate(config_path)?;
    if migrated {
        config.save(config_path)?;
    }

    let mut invalid_accounts = Vec::new();
    let mut seen = std::collections::HashSet::new();
    for target in &config.targets {
        let spec = spec_for(target.provider.clone());
        let host = host_or_default(target.host.as_deref(), spec.as_ref());
        let account = match spec.account_key(&host, &target.scope) {
            Ok(account) => account,
            Err(err) => {
                let _ = audit.record(
                    "token.check",
                    AuditStatus::Failed,
                    Some("token.check"),
                    Some(serde_json::json!({
                        "provider": target.provider.as_prefix(),
                        "scope": target.scope.segments().join("/"),
                        "source": source
                    })),
                    Some(&err.to_string()),
                );
                continue;
            }
        };
        if !seen.insert(account.clone()) {
            continue;
        }
        let runtime_target = ProviderTarget {
            provider: target.provider.clone(),
            scope: target.scope.clone(),
            host: Some(host),
        };
        let validation = token_check::check_token_validity(&runtime_target);
        let status_label = match validation.status {
            token_check::TokenValidity::Ok => "ok",
            token_check::TokenValidity::Invalid => "invalid",
            token_check::TokenValidity::ScopeNotFound => "scope_not_found",
            token_check::TokenValidity::Network => "network",
            token_check::TokenValidity::Error => "error",
        };
        mirror_core::cache::record_token_status(
            &mut cache,
            account.clone(),
            mirror_core::cache::TokenStatus {
                last_checked: now,
                status: status_label.to_string(),
                error: validation.error.clone(),
            },
        );
        if validation.status == token_check::TokenValidity::Invalid {
            invalid_accounts.push(account.clone());
        }
        let audit_status = match validation.status {
            token_check::TokenValidity::Ok => AuditStatus::Ok,
            token_check::TokenValidity::Network => AuditStatus::Skipped,
            _ => AuditStatus::Failed,
        };
        let _ = audit.record(
            "token.check",
            audit_status,
            Some("token.check"),
            Some(serde_json::json!({
                "provider": runtime_target.provider.as_prefix(),
                "scope": runtime_target.scope.segments().join("/"),
                "account": account,
                "status": status_label,
                "source": source
            })),
            validation.error.as_deref(),
        );
    }

    mirror_core::cache::record_token_check(&mut cache, now, source);
    let _ = cache.save(cache_path);

    if !invalid_accounts.is_empty() {
        println!(
            "Warning: {} PAT token(s) are invalid or expired. Run `token set` to refresh.",
            invalid_accounts.len()
        );
    }

    Ok(())
}

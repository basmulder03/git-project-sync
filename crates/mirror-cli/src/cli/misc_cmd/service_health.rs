use super::*;
use crate::i18n::{key, tf};
pub(in crate::cli) fn handle_service(args: ServiceArgs, audit: &AuditLogger) -> anyhow::Result<()> {
    let result: anyhow::Result<()> = (|| {
        let exe = std::env::current_exe().context("resolve current executable")?;
        match args.action {
            ServiceAction::Install => {
                mirror_core::service::install_service(&exe).map_err(|err| {
                    if update::is_permission_error(&err)
                        && maybe_escalate_and_reexec("install service").unwrap_or(false)
                    {
                        return anyhow::anyhow!("service escalated");
                    }
                    err
                })?;
                println!("Service installed for {}", exe.display());
                let audit_id = audit.record(
                    "service.install",
                    AuditStatus::Ok,
                    Some("service.install"),
                    None,
                    None,
                )?;
                println!("{}", tf(key::AUDIT_ID, &[("audit_id", audit_id)]));
            }
            ServiceAction::Uninstall => {
                mirror_core::service::uninstall_service().map_err(|err| {
                    if update::is_permission_error(&err)
                        && maybe_escalate_and_reexec("uninstall service").unwrap_or(false)
                    {
                        return anyhow::anyhow!("service escalated");
                    }
                    err
                })?;
                let _ = install::remove_marker();
                let _ = install::remove_manifest();
                println!("Service uninstalled.");
                let audit_id = audit.record(
                    "service.uninstall",
                    AuditStatus::Ok,
                    Some("service.uninstall"),
                    None,
                    None,
                )?;
                println!("{}", tf(key::AUDIT_ID, &[("audit_id", audit_id)]));
            }
        }
        Ok(())
    })();

    if let Err(err) = &result {
        if err.to_string() == "service escalated" {
            return Ok(());
        }
        let action = match args.action {
            ServiceAction::Install => "service.install",
            ServiceAction::Uninstall => "service.uninstall",
        };
        let _ = audit.record(
            action,
            AuditStatus::Failed,
            Some(action),
            None,
            Some(&err.to_string()),
        );
    }
    result
}

pub(in crate::cli) async fn handle_health(
    args: HealthArgs,
    audit: &AuditLogger,
) -> anyhow::Result<()> {
    let result: anyhow::Result<()> = async {
        let config_path = args.config.unwrap_or(default_config_path()?);
        let (config, migrated) = load_or_migrate(&config_path)?;
        if migrated {
            config.save(&config_path)?;
        }
        let target_id_ignores_selectors =
            args.target_id.is_some() && (args.provider.is_some() || !args.scope.is_empty());

        let selection = select_targets_with_precedence(
            &config,
            args.target_id.as_deref(),
            args.provider,
            &args.scope,
        )?;
        if let Some(warning) = selection.warning.as_deref() {
            println!(
                "{}",
                tf(key::WARNING_GENERIC, &[("warning", warning.to_string())])
            );
        } else if target_id_ignores_selectors {
            println!("{}", tf(key::WARNING_TARGET_ID_PRECEDENCE, &[]));
        }
        let targets = selection.targets;
        if targets.is_empty() {
            println!("{}", tf(key::NO_MATCHING_TARGETS, &[]));
            let audit_id = audit.record(
                "health.check",
                AuditStatus::Skipped,
                Some("health"),
                None,
                Some("no matching targets"),
            )?;
            println!("{}", tf(key::AUDIT_ID, &[("audit_id", audit_id)]));
            return Ok(());
        }

        let registry = ProviderRegistry::new();
        for target in targets {
            let provider_kind = target.provider.clone();
            let provider = registry.provider(provider_kind.clone())?;
            let runtime_target = ProviderTarget {
                provider: provider_kind,
                scope: target.scope.clone(),
                host: target.host.clone(),
            };

            let outcome = provider
                .health_check(&runtime_target)
                .await
                .or_else(|err| map_provider_error(&runtime_target, err));
            match outcome {
                Ok(()) => {
                    println!(
                        "Health OK: {} {}",
                        target.provider.as_prefix(),
                        target.scope.segments().join("/")
                    );
                    let audit_id = audit.record_with_context(
                        "health.check",
                        AuditStatus::Ok,
                        Some("health"),
                        AuditContext {
                            provider: Some(target.provider.as_prefix().to_string()),
                            scope: Some(target.scope.segments().join("/")),
                            repo_id: Some(target.id.clone()),
                            path: None,
                        },
                        None,
                        None,
                    )?;
                    println!("{}", tf(key::AUDIT_ID, &[("audit_id", audit_id)]));
                }
                Err(err) => {
                    eprintln!(
                        "Health FAILED: {} {} -> {err}",
                        target.provider.as_prefix(),
                        target.scope.segments().join("/")
                    );
                    let audit_id = audit.record_with_context(
                        "health.check",
                        AuditStatus::Failed,
                        Some("health"),
                        AuditContext {
                            provider: Some(target.provider.as_prefix().to_string()),
                            scope: Some(target.scope.segments().join("/")),
                            repo_id: Some(target.id.clone()),
                            path: None,
                        },
                        None,
                        Some(&err.to_string()),
                    )?;
                    println!("{}", tf(key::AUDIT_ID, &[("audit_id", audit_id)]));
                }
            }
        }
        Ok(())
    }
    .await;

    if let Err(err) = &result {
        let _ = audit.record(
            "health.check",
            AuditStatus::Failed,
            Some("health"),
            None,
            Some(&err.to_string()),
        );
    }

    result
}

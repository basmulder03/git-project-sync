use super::*;
use crate::i18n::{key, tf};
pub(super) fn handle_target(args: TargetArgs, audit: &AuditLogger) -> anyhow::Result<()> {
    match args.command {
        TargetCommands::Add(args) => handle_add_target(args, audit),
        TargetCommands::List => handle_list_targets(audit),
        TargetCommands::Remove(args) => handle_remove_target(args, audit),
    }
}

pub(super) fn handle_add_target(args: AddTargetArgs, audit: &AuditLogger) -> anyhow::Result<()> {
    let result: anyhow::Result<()> = (|| {
        let config_path = default_config_path()?;
        let (mut config, migrated) = load_or_migrate(&config_path)?;
        let provider: ProviderKind = args.provider.into();
        let spec = spec_for(provider.clone());
        let scope = spec.parse_scope(args.scope)?;

        let host = args
            .host
            .as_ref()
            .map(|value| value.trim_end_matches('/').to_string());
        let id = target_id(provider.clone(), host.as_deref(), &scope);

        if config.targets.iter().any(|target| target.id == id) {
            println!("{}", tf(key::TARGET_EXISTS, &[("id", id.clone())]));
            let audit_id = audit.record_with_context(
                "target.add",
                AuditStatus::Skipped,
                Some("target.add"),
                AuditContext {
                    provider: Some(provider.as_prefix().to_string()),
                    scope: Some(scope.segments().join("/")),
                    repo_id: None,
                    path: None,
                },
                None,
                Some("target already exists"),
            )?;
            println!("{}", tf(key::AUDIT_ID, &[("audit_id", audit_id)]));
            return Ok(());
        }

        let target = TargetConfig {
            id: id.clone(),
            provider: provider.clone(),
            scope: scope.clone(),
            host,
            labels: args.label,
        };

        config.targets.push(target);
        config.save(&config_path)?;
        if migrated {
            println!(
                "Config migrated and target added to {}",
                config_path.display()
            );
        } else {
            println!(
                "{}",
                tf(
                    key::TARGET_ADDED_TO_PATH,
                    &[("path", config_path.display().to_string())]
                )
            );
        }
        let audit_id = audit.record_with_context(
            "target.add",
            AuditStatus::Ok,
            Some("target.add"),
            AuditContext {
                provider: Some(provider.as_prefix().to_string()),
                scope: Some(scope.segments().join("/")),
                repo_id: Some(id),
                path: None,
            },
            None,
            None,
        )?;
        println!("{}", tf(key::AUDIT_ID, &[("audit_id", audit_id)]));
        Ok(())
    })();

    if let Err(err) = &result {
        let _ = audit.record(
            "target.add",
            AuditStatus::Failed,
            Some("target.add"),
            None,
            Some(&err.to_string()),
        );
    }
    result
}

pub(super) fn handle_list_targets(audit: &AuditLogger) -> anyhow::Result<()> {
    let result: anyhow::Result<()> = (|| {
        let config_path = default_config_path()?;
        let (config, migrated) = load_or_migrate(&config_path)?;
        if migrated {
            config.save(&config_path)?;
        }

        if config.targets.is_empty() {
            println!("{}", tf(key::NO_TARGETS_CONFIGURED, &[]));
            return Ok(());
        }

        for target in config.targets {
            let host = target
                .host
                .clone()
                .unwrap_or_else(|| "(default)".to_string());
            println!(
                "{} | {} | {} | {}",
                target.id,
                target.provider.as_prefix(),
                target.scope.segments().join("/"),
                host
            );
        }
        Ok(())
    })();

    if let Err(err) = &result {
        let _ = audit.record(
            "target.list",
            AuditStatus::Failed,
            Some("target.list"),
            None,
            Some(&err.to_string()),
        );
    } else {
        let audit_id = audit.record(
            "target.list",
            AuditStatus::Ok,
            Some("target.list"),
            None,
            None,
        )?;
        println!("{}", tf(key::AUDIT_ID, &[("audit_id", audit_id)]));
    }
    result
}

pub(super) fn handle_remove_target(
    args: RemoveTargetArgs,
    audit: &AuditLogger,
) -> anyhow::Result<()> {
    let result: anyhow::Result<()> = (|| {
        let config_path = default_config_path()?;
        let (mut config, migrated) = load_or_migrate(&config_path)?;
        let before = config.targets.len();
        config.targets.retain(|target| target.id != args.id);
        let after = config.targets.len();
        if before == after {
            println!(
                "{}",
                tf(key::TARGET_NOT_FOUND_ID, &[("id", args.id.clone())])
            );
            let audit_id = audit.record(
                "target.remove",
                AuditStatus::Skipped,
                Some("target.remove"),
                None,
                Some("target not found"),
            )?;
            println!("{}", tf(key::AUDIT_ID, &[("audit_id", audit_id)]));
            return Ok(());
        }
        config.save(&config_path)?;
        if migrated {
            println!(
                "Config migrated and target removed from {}",
                config_path.display()
            );
        } else {
            println!(
                "{}",
                tf(
                    key::TARGET_REMOVED_FROM_PATH,
                    &[("path", config_path.display().to_string())]
                )
            );
        }
        let audit_id = audit.record(
            "target.remove",
            AuditStatus::Ok,
            Some("target.remove"),
            None,
            None,
        )?;
        println!("{}", tf(key::AUDIT_ID, &[("audit_id", audit_id)]));
        Ok(())
    })();

    if let Err(err) = &result {
        let _ = audit.record(
            "target.remove",
            AuditStatus::Failed,
            Some("target.remove"),
            None,
            Some(&err.to_string()),
        );
    }
    result
}

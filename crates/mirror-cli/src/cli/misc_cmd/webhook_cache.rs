use super::*;
use crate::i18n::{key, tf};
pub(in crate::cli) async fn handle_webhook(
    args: WebhookArgs,
    audit: &AuditLogger,
) -> anyhow::Result<()> {
    match args.command {
        WebhookCommands::Register(args) => handle_webhook_register(args, audit).await,
    }
}

pub(in crate::cli) async fn handle_webhook_register(
    args: WebhookRegisterArgs,
    audit: &AuditLogger,
) -> anyhow::Result<()> {
    let result: anyhow::Result<()> = async {
        let provider: ProviderKind = args.provider.into();
        let spec = spec_for(provider.clone());
        let scope = spec.parse_scope(args.scope)?;
        let host = args
            .host
            .as_ref()
            .map(|value| value.trim_end_matches('/').to_string());
        let runtime_target = ProviderTarget {
            provider: provider.clone(),
            scope: scope.clone(),
            host: host.clone(),
        };

        let registry = ProviderRegistry::new();
        let adapter = registry.provider(provider.clone())?;
        adapter
            .register_webhook(&runtime_target, &args.url, args.secret.as_deref())
            .await
            .or_else(|err| map_provider_error(&runtime_target, err))?;

        println!(
            "Webhook registered for {} {}",
            provider.as_prefix(),
            scope.segments().join("/")
        );
        let audit_id = audit.record_with_context(
            "webhook.register",
            AuditStatus::Ok,
            Some("webhook.register"),
            AuditContext {
                provider: Some(provider.as_prefix().to_string()),
                scope: Some(scope.segments().join("/")),
                repo_id: None,
                path: None,
            },
            None,
            None,
        )?;
        println!("{}", tf(key::AUDIT_ID, &[("audit_id", audit_id)]));
        Ok(())
    }
    .await;

    if let Err(err) = &result {
        let _ = audit.record(
            "webhook.register",
            AuditStatus::Failed,
            Some("webhook.register"),
            None,
            Some(&err.to_string()),
        );
    }
    result
}

pub(in crate::cli) fn handle_cache(args: CacheArgs, audit: &AuditLogger) -> anyhow::Result<()> {
    match args.command {
        CacheCommands::Prune(args) => handle_cache_prune(args, audit),
        CacheCommands::Overview(args) => handle_cache_overview(args, audit),
    }
}

pub(in crate::cli) fn handle_cache_prune(
    args: CachePruneArgs,
    audit: &AuditLogger,
) -> anyhow::Result<()> {
    let result: anyhow::Result<()> = (|| {
        let config_path = args.config.unwrap_or(default_config_path()?);
        let cache_path = args.cache.unwrap_or(default_cache_path()?);
        let (config, migrated) = load_or_migrate(&config_path)?;
        if migrated {
            config.save(&config_path)?;
        }
        let target_ids: Vec<String> = config.targets.iter().map(|t| t.id.clone()).collect();
        let removed = mirror_core::cache::prune_cache_for_targets(&cache_path, &target_ids)?;
        println!("Pruned {removed} cache entries.");
        let audit_id = audit.record(
            "cache.prune",
            AuditStatus::Ok,
            Some("cache.prune"),
            None,
            None,
        )?;
        println!("{}", tf(key::AUDIT_ID, &[("audit_id", audit_id)]));
        Ok(())
    })();

    if let Err(err) = &result {
        let _ = audit.record(
            "cache.prune",
            AuditStatus::Failed,
            Some("cache.prune"),
            None,
            Some(&err.to_string()),
        );
    }

    result
}

pub(in crate::cli) fn handle_cache_overview(
    args: CacheOverviewArgs,
    audit: &AuditLogger,
) -> anyhow::Result<()> {
    let result: anyhow::Result<()> = (|| {
        let config_path = args.config.unwrap_or(default_config_path()?);
        let cache_path = args.cache.unwrap_or(default_cache_path()?);
        let (config, migrated) = load_or_migrate(&config_path)?;
        if migrated {
            config.save(&config_path)?;
        }
        if args.refresh {
            let _ = repo_overview::refresh_repo_status(&cache_path)?;
        }
        let cache = RepoCache::load(&cache_path).unwrap_or_default();
        let root = config.root.as_deref();
        let tree = repo_overview::build_repo_tree(cache.repos.iter(), root);
        let lines = repo_overview::render_repo_tree_lines(&tree, &cache, &cache.repo_status);

        let root_label = root
            .map(|p| p.display().to_string())
            .unwrap_or_else(|| "<unset>".to_string());
        println!("Root: {root_label}");
        let last_refresh = cache
            .repo_status
            .values()
            .map(|status| status.checked_at)
            .max()
            .map(repo_overview::format_epoch_label)
            .unwrap_or_else(|| "never".to_string());
        println!("Status refresh: {last_refresh}");
        if lines.is_empty() {
            println!("No repos in cache yet.");
        } else {
            for line in lines {
                println!("{line}");
            }
        }
        let audit_id = audit.record(
            "cache.overview",
            AuditStatus::Ok,
            Some("cache.overview"),
            None,
            None,
        )?;
        println!("{}", tf(key::AUDIT_ID, &[("audit_id", audit_id)]));
        Ok(())
    })();

    if let Err(err) = &result {
        let _ = audit.record(
            "cache.overview",
            AuditStatus::Failed,
            Some("cache.overview"),
            None,
            Some(&err.to_string()),
        );
    }

    result
}

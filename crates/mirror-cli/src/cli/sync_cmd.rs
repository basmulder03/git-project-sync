use super::shared::{
    accumulate_summary, audit_repo_progress, map_azdo_error, print_summary, print_sync_status,
    prompt_action, render_sync_progress, select_targets_with_precedence,
};
use super::*;
pub(super) async fn handle_sync(args: SyncArgs, audit: &AuditLogger) -> anyhow::Result<()> {
    let result: anyhow::Result<()> = async {
        if args.non_interactive && args.missing_remote == MissingRemotePolicyValue::Prompt {
            anyhow::bail!("non-interactive mode requires --missing-remote policy");
        }
        let force_refresh_all = args.force_refresh_all;
        let force_ignores_selectors = args.target_id.is_some()
            || args.provider.is_some()
            || !args.scope.is_empty()
            || args.repo.is_some();
        let target_id_ignores_selectors = args.target_id.is_some()
            && (args.provider.is_some() || !args.scope.is_empty());

        let config_path = args.config.unwrap_or(default_config_path()?);
        let cache_path = args.cache.unwrap_or(default_cache_path()?);
        let (config, migrated) = load_or_migrate(&config_path)?;
        if migrated {
            config.save(&config_path)?;
        }
        let selection = if force_refresh_all {
            let warning = if force_ignores_selectors {
                Some(
                    "--force-refresh-all ignores --target-id/--provider/--scope/--repo and syncs all configured targets/repos."
                        .to_string(),
                )
            } else {
                None
            };
            super::shared::TargetSelection {
                targets: config.targets.clone(),
                warning,
            }
        } else {
            select_targets_with_precedence(
                &config,
                args.target_id.as_deref(),
                args.provider,
                &args.scope,
            )?
        };
        if let Some(warning) = selection.warning.as_deref() {
            println!("Warning: {warning}");
        } else if !force_refresh_all && target_id_ignores_selectors {
            println!("Warning: --target-id takes precedence; ignoring --provider/--scope");
        }

        if args.status_only {
            let targets = selection.targets.clone();
            if targets.is_empty() {
                println!("No matching targets found.");
                let audit_id = audit.record(
                    "sync.status",
                    AuditStatus::Skipped,
                    Some("sync"),
                    None,
                    Some("no matching targets"),
                )?;
                println!("Audit ID: {audit_id}");
                return Ok(());
            }
            print_sync_status(&cache_path, &targets)?;
            let audit_id =
                audit.record("sync.status", AuditStatus::Ok, Some("sync"), None, None)?;
            println!("Audit ID: {audit_id}");
            return Ok(());
        }
        let root = config
            .root
            .as_ref()
            .context("config missing root; run config init")?;

        let targets = selection.targets;
        if targets.is_empty() {
            println!("No matching targets found.");
            let audit_id = audit.record(
                "sync.run",
                AuditStatus::Skipped,
                Some("sync"),
                None,
                Some("no matching targets"),
            )?;
            println!("Audit ID: {audit_id}");
            return Ok(());
        }

        let policy: MissingRemotePolicy = args.missing_remote.into();
        let decider = if policy == MissingRemotePolicy::Prompt {
            Some(&prompt_action as &dyn Fn(&RepoCacheEntry) -> anyhow::Result<DeletedRepoAction>)
        } else {
            None
        };

        let registry = ProviderRegistry::new();
        let mut total = SyncSummary::default();

        let target_count = targets.len();
        for target in targets {
            let provider_kind = target.provider.clone();
            let provider = registry.provider(provider_kind.clone())?;
            let runtime_target = ProviderTarget {
                provider: provider_kind,
                scope: target.scope.clone(),
                host: target.host.clone(),
            };
            let include_archived = args.include_archived;
            let target_label = format!(
                "{}:{}",
                target.provider.as_prefix(),
                target.scope.segments().join("/")
            );
            let last_len = Cell::new(0usize);
            let progress_fn = |progress: SyncProgress| {
                if args.status {
                    render_sync_progress(&target_label, &last_len, &progress);
                }
                if args.audit_repo {
                    audit_repo_progress(audit, "sync", "sync.repo", &runtime_target, &progress);
                }
            };
            let progress: Option<&dyn Fn(SyncProgress)> = if args.status || args.audit_repo {
                Some(&progress_fn)
            } else {
                None
            };
            let summary = if !force_refresh_all && let Some(repo_name) = args.repo.as_ref() {
                let repo_name = repo_name.clone();
                let filter = move |remote: &mirror_core::model::RemoteRepo| {
                    let matches = remote.name == repo_name || remote.id == repo_name;
                    let allowed = include_archived || !remote.archived;
                    allowed && matches
                };
                let options = RunSyncOptions {
                    missing_policy: policy,
                    missing_decider: decider,
                    repo_filter: Some(&filter),
                    progress,
                    jobs: args.jobs,
                    detect_missing: false,
                    refresh: args.refresh || force_refresh_all,
                    verify: args.verify,
                };
                run_sync_filtered(
                    provider.as_ref(),
                    &runtime_target,
                    root,
                    &cache_path,
                    options,
                )
                .await
                .or_else(|err| map_azdo_error(&runtime_target, err))?
            } else {
                let filter =
                    move |repo: &mirror_core::model::RemoteRepo| include_archived || !repo.archived;
                let options = RunSyncOptions {
                    missing_policy: policy,
                    missing_decider: decider,
                    repo_filter: Some(&filter),
                    progress,
                    jobs: args.jobs,
                    detect_missing: true,
                    refresh: args.refresh || force_refresh_all,
                    verify: args.verify,
                };
                run_sync_filtered(
                    provider.as_ref(),
                    &runtime_target,
                    root,
                    &cache_path,
                    options,
                )
                .await
                .or_else(|err| map_azdo_error(&runtime_target, err))?
            };

            print_summary(&target, summary);
            let details = serde_json::json!({
                "cloned": summary.cloned,
                "fast_forwarded": summary.fast_forwarded,
                "up_to_date": summary.up_to_date,
                "dirty": summary.dirty,
                "diverged": summary.diverged,
                "failed": summary.failed,
                "missing_archived": summary.missing_archived,
                "missing_removed": summary.missing_removed,
                "missing_skipped": summary.missing_skipped,
            });
            let context = AuditContext {
                provider: Some(target.provider.as_prefix().to_string()),
                scope: Some(target.scope.segments().join("/")),
                repo_id: Some(target.id.clone()),
                path: None,
            };
            let audit_id = audit.record_with_context(
                "sync.target",
                AuditStatus::Ok,
                Some("sync"),
                context,
                Some(details),
                None,
            )?;
            println!("Audit ID: {audit_id}");
            accumulate_summary(&mut total, summary);
        }

        if target_count > 1 {
            println!(
                "Total: cloned={} fast_forwarded={} up_to_date={} dirty={} diverged={} failed={} missing_archived={} missing_removed={} missing_skipped={}",
                total.cloned,
                total.fast_forwarded,
                total.up_to_date,
                total.dirty,
                total.diverged,
                total.failed,
                total.missing_archived,
                total.missing_removed,
                total.missing_skipped
            );
        }

        let totals = serde_json::json!({
            "targets": target_count,
            "cloned": total.cloned,
            "fast_forwarded": total.fast_forwarded,
            "up_to_date": total.up_to_date,
            "dirty": total.dirty,
            "diverged": total.diverged,
            "failed": total.failed,
            "missing_archived": total.missing_archived,
            "missing_removed": total.missing_removed,
            "missing_skipped": total.missing_skipped,
            "force_refresh_all": force_refresh_all,
        });
        let audit_id = audit.record(
            "sync.run",
            AuditStatus::Ok,
            Some("sync"),
            Some(totals),
            None,
        )?;
        println!("Audit ID: {audit_id}");

        Ok(())
    }
    .await;

    if let Err(err) = &result {
        let _ = audit.record(
            "sync.run",
            AuditStatus::Failed,
            Some("sync"),
            None,
            Some(&err.to_string()),
        );
    }
    result
}

use super::*;
pub(in crate::cli) async fn run_sync_job(
    config_path: &Path,
    cache_path: &Path,
    policy: MissingRemotePolicy,
    audit: &AuditLogger,
    audit_repo: bool,
    jobs: usize,
) -> anyhow::Result<()> {
    let (config, migrated) = load_or_migrate(config_path)?;
    if migrated {
        config.save(config_path)?;
    }
    let root = config
        .root
        .as_ref()
        .context("config missing root; run config init")?;
    let registry = ProviderRegistry::new();
    let day_bucket = current_day_bucket();
    let now = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .unwrap_or_default()
        .as_secs();
    let cache_snapshot = mirror_core::cache::RepoCache::load(cache_path).unwrap_or_default();
    let mut had_failure = false;
    for target in config.targets {
        let target_key = target_id(
            target.provider.clone(),
            target.host.as_deref(),
            &target.scope,
        );
        if let Some(until) = backoff_until(&cache_snapshot, &target_key)
            && until > now
        {
            warn!(
                provider = %target.provider,
                scope = ?target.scope,
                until = until,
                "skipping target due to backoff"
            );
            let _ = audit.record_with_context(
                "daemon.sync.target",
                AuditStatus::Skipped,
                Some("daemon.sync"),
                AuditContext {
                    provider: Some(target.provider.as_prefix().to_string()),
                    scope: Some(target.scope.segments().join("/")),
                    repo_id: Some(target.id.clone()),
                    path: None,
                },
                Some(serde_json::json!({"reason": "backoff", "until": until})),
                Some("skipping target due to backoff"),
            );
            continue;
        }
        let provider_kind = target.provider.clone();
        let provider = registry.provider(provider_kind.clone())?;
        let runtime_target = ProviderTarget {
            provider: provider_kind,
            scope: target.scope.clone(),
            host: target.host.clone(),
        };
        let progress_fn = |progress: SyncProgress| {
            if audit_repo {
                audit_repo_progress(
                    audit,
                    "daemon.sync",
                    "daemon.sync.repo",
                    &runtime_target,
                    &progress,
                );
            }
        };
        let bucketed = move |repo: &mirror_core::model::RemoteRepo| {
            !repo.archived && bucket_for_repo_id(&repo.id) == day_bucket
        };
        let options = RunSyncOptions {
            missing_policy: policy,
            missing_decider: None,
            repo_filter: Some(&bucketed),
            progress: if audit_repo { Some(&progress_fn) } else { None },
            jobs,
            detect_missing: true,
            refresh: false,
            verify: false,
        };
        let result = run_sync_filtered(
            provider.as_ref(),
            &runtime_target,
            root,
            cache_path,
            options,
        )
        .await
        .or_else(|err| map_azdo_error(&runtime_target, err));
        match result {
            Ok(summary) => {
                let _ = update_target_success(cache_path, &target_key, now);
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
                let _ = audit.record_with_context(
                    "daemon.sync.target",
                    AuditStatus::Ok,
                    Some("daemon.sync"),
                    AuditContext {
                        provider: Some(target.provider.as_prefix().to_string()),
                        scope: Some(target.scope.segments().join("/")),
                        repo_id: Some(target.id.clone()),
                        path: None,
                    },
                    Some(details),
                    None,
                );
            }
            Err(err) => {
                had_failure = true;
                let _ = update_target_failure(cache_path, &target_key, now);
                warn!(
                    provider = %runtime_target.provider,
                    scope = ?runtime_target.scope,
                    error = %err,
                    "target sync failed"
                );
                let _ = audit.record_with_context(
                    "daemon.sync.target",
                    AuditStatus::Failed,
                    Some("daemon.sync"),
                    AuditContext {
                        provider: Some(runtime_target.provider.as_prefix().to_string()),
                        scope: Some(runtime_target.scope.segments().join("/")),
                        repo_id: Some(target.id.clone()),
                        path: None,
                    },
                    None,
                    Some(&err.to_string()),
                );
            }
        }
    }
    if had_failure {
        anyhow::bail!("one or more targets failed");
    }
    Ok(())
}

pub(in crate::cli) fn select_targets(
    config: &AppConfigV2,
    target_id: Option<&str>,
    provider: Option<ProviderKindValue>,
    scope: &[String],
) -> anyhow::Result<Vec<TargetConfig>> {
    let mut targets = config.targets.clone();

    if let Some(target_id) = target_id {
        targets.retain(|target| target.id == target_id);
        return Ok(targets);
    }

    let provider_kind = provider.map(ProviderKind::from);

    if let Some(provider_kind) = provider_kind.as_ref() {
        targets.retain(|target| target.provider == *provider_kind);
    } else if !scope.is_empty() {
        anyhow::bail!("--scope requires --provider when selecting targets");
    }

    if !scope.is_empty() {
        let provider_kind = provider_kind.clone().context("provider required")?;
        let spec = spec_for(provider_kind);
        let scope = spec.parse_scope(scope.to_vec())?;
        targets.retain(|target| target.scope == scope);
    }

    Ok(targets)
}

pub(in crate::cli) fn prompt_action(entry: &RepoCacheEntry) -> anyhow::Result<DeletedRepoAction> {
    println!(
        "Remote repo missing: {} (path: {}). Choose [a]rchive / [r]emove / [s]kip:",
        entry.name, entry.path
    );
    loop {
        print!("> ");
        io::stdout().flush().ok();
        let mut input = String::new();
        io::stdin().read_line(&mut input)?;
        match input.trim().to_lowercase().as_str() {
            "a" | "archive" => return Ok(DeletedRepoAction::Archive),
            "r" | "remove" => return Ok(DeletedRepoAction::Remove),
            "s" | "skip" => return Ok(DeletedRepoAction::Skip),
            _ => println!("Please enter a, r, or s."),
        }
    }
}

pub(in crate::cli) fn print_summary(target: &TargetConfig, summary: SyncSummary) {
    println!(
        "Target {}: cloned={} fast_forwarded={} up_to_date={} dirty={} diverged={} failed={} missing_archived={} missing_removed={} missing_skipped={}",
        target.id,
        summary.cloned,
        summary.fast_forwarded,
        summary.up_to_date,
        summary.dirty,
        summary.diverged,
        summary.failed,
        summary.missing_archived,
        summary.missing_removed,
        summary.missing_skipped
    );
}

pub(in crate::cli) fn accumulate_summary(total: &mut SyncSummary, summary: SyncSummary) {
    total.cloned += summary.cloned;
    total.fast_forwarded += summary.fast_forwarded;
    total.up_to_date += summary.up_to_date;
    total.dirty += summary.dirty;
    total.diverged += summary.diverged;
    total.failed += summary.failed;
    total.missing_archived += summary.missing_archived;
    total.missing_removed += summary.missing_removed;
    total.missing_skipped += summary.missing_skipped;
}

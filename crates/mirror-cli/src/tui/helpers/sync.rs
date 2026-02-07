use super::*;

pub(in crate::tui) fn run_tui_sync(
    targets: &[TargetConfig],
    root: &std::path::Path,
    cache_path: &std::path::Path,
    audit: &AuditLogger,
    force_refresh_all: bool,
) -> anyhow::Result<SyncSummary> {
    let audit_id = audit.record(
        "tui.sync.start",
        AuditStatus::Ok,
        Some("tui"),
        Some(serde_json::json!({ "force_refresh_all": force_refresh_all })),
        None,
    )?;
    let _ = audit_id;
    let registry = ProviderRegistry::new();
    let mut total = SyncSummary::default();

    for target in targets {
        let provider_kind = target.provider.clone();
        let provider = registry.provider(provider_kind.clone())?;
        let runtime_target = ProviderTarget {
            provider: provider_kind,
            scope: target.scope.clone(),
            host: target.host.clone(),
        };
        let filter = |repo: &RemoteRepo| !repo.archived;
        let options = RunSyncOptions {
            missing_policy: MissingRemotePolicy::Skip,
            missing_decider: None,
            repo_filter: Some(&filter),
            progress: None,
            jobs: 1,
            detect_missing: true,
            refresh: force_refresh_all,
            verify: false,
        };
        let summary = match mirror_core::provider::block_on(run_sync_filtered(
            provider.as_ref(),
            &runtime_target,
            root,
            cache_path,
            options,
        )) {
            Ok(summary) => summary,
            Err(err) => {
                let error_text = format!("{err:#}");
                let context = AuditContext {
                    provider: Some(target.provider.as_prefix().to_string()),
                    scope: Some(target.scope.segments().join("/")),
                    repo_id: Some(target.id.clone()),
                    path: None,
                };
                let _ = audit.record_with_context(
                    "tui.sync.target",
                    AuditStatus::Failed,
                    Some("tui"),
                    context,
                    None,
                    Some(&error_text),
                );
                return Err(err.context(format!(
                    "target {}:{} sync failed",
                    target.provider.as_prefix(),
                    target.scope.segments().join("/")
                )));
            }
        };
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
        let _ = audit.record_with_context(
            "tui.sync.target",
            AuditStatus::Ok,
            Some("tui"),
            context,
            Some(details),
            None,
        );
        accumulate_summary(&mut total, summary);
    }

    let details = serde_json::json!({
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
    let _ = audit.record(
        "tui.sync.finish",
        AuditStatus::Ok,
        Some("tui"),
        Some(details),
        None,
    );
    Ok(total)
}

pub(in crate::tui) fn accumulate_summary(total: &mut SyncSummary, next: SyncSummary) {
    total.cloned += next.cloned;
    total.fast_forwarded += next.fast_forwarded;
    total.up_to_date += next.up_to_date;
    total.dirty += next.dirty;
    total.diverged += next.diverged;
    total.failed += next.failed;
    total.missing_archived += next.missing_archived;
    total.missing_removed += next.missing_removed;
    total.missing_skipped += next.missing_skipped;
}

pub(in crate::tui) fn last_sync_error(
    audit: &AuditLogger,
    target_id: &str,
) -> anyhow::Result<String> {
    let base_dir = audit.base_dir();
    let date = ::time::OffsetDateTime::now_utc()
        .format(&::time::format_description::parse("[year][month][day]").unwrap())
        .unwrap();
    let path = base_dir.join(format!("audit-{date}.jsonl"));
    if !path.exists() {
        return Ok(String::new());
    }
    let contents = std::fs::read_to_string(&path)?;
    for line in contents.lines().rev().take(200) {
        if !line.contains("\"status\":\"failed\"") {
            continue;
        }
        if !line.contains("\"event\":\"sync.target\"")
            && !line.contains("\"event\":\"daemon.sync.target\"")
        {
            continue;
        }
        if !line.contains(&format!("\"repo_id\":\"{target_id}\"")) {
            continue;
        }
        if let Ok(value) = serde_json::from_str::<serde_json::Value>(line)
            && let Some(error) = value.get("error").and_then(|v| v.as_str())
        {
            return Ok(error.to_string());
        }
    }
    Ok(String::new())
}

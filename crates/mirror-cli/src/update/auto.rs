use super::*;

pub fn check_and_maybe_apply(options: AutoUpdateOptions<'_>) -> anyhow::Result<bool> {
    let now = current_epoch_seconds();
    let mut cache = RepoCache::load(options.cache_path).unwrap_or_default();
    if !options.force && !update_check_due(&cache, now, options.interval_secs) {
        return Ok(false);
    }

    let check = match check_for_update(options.override_repo) {
        Ok(value) => value,
        Err(err) if is_network_error(&err) => {
            record_update_check(
                &mut cache,
                now,
                "skipped:network".to_string(),
                None,
                options.source,
            );
            let _ = cache.save(options.cache_path);
            let _ = options.audit.record(
                "update.check",
                AuditStatus::Skipped,
                Some("update"),
                Some(json!({"reason": "network", "source": options.source})),
                Some(&err.to_string()),
            );
            return Ok(false);
        }
        Err(err) => {
            let _ = options.audit.record(
                "update.check",
                AuditStatus::Failed,
                Some("update"),
                Some(json!({"source": options.source})),
                Some(&err.to_string()),
            );
            return Err(err);
        }
    };

    let latest = check.latest.to_string();
    let result = if check.is_newer {
        "update_available"
    } else {
        "up_to_date"
    };
    record_update_check(
        &mut cache,
        now,
        result.to_string(),
        Some(latest.clone()),
        options.source,
    );
    let _ = cache.save(options.cache_path);

    let _ = options.audit.record(
        "update.check",
        AuditStatus::Ok,
        Some("update"),
        Some(json!({
            "current": check.current.to_string(),
            "latest": latest,
            "is_newer": check.is_newer,
            "source": options.source
        })),
        None,
    );

    let mut applied = false;
    if check.is_newer && options.auto_apply {
        if !crate::install::is_installed()? {
            let _ = options.audit.record(
                "update.apply",
                AuditStatus::Skipped,
                Some("update"),
                Some(json!({"reason": "not_installed", "source": options.source})),
                None,
            );
            return Ok(false);
        }
        match apply_update(&check) {
            Ok(report) => {
                applied = true;
                let _ = options.audit.record(
                    "update.apply",
                    AuditStatus::Ok,
                    Some("update"),
                    Some(json!({
                        "source": options.source,
                        "install": report.install,
                        "service": report.service,
                        "path": report.path
                    })),
                    None,
                );
            }
            Err(err) if is_permission_error(&err) && options.interactive => {
                let _ = options.audit.record(
                    "update.apply",
                    AuditStatus::Failed,
                    Some("update"),
                    Some(json!({"source": options.source, "reason": "permission"})),
                    Some(&err.to_string()),
                );
                return Err(err);
            }
            Err(err) => {
                let _ = options.audit.record(
                    "update.apply",
                    AuditStatus::Failed,
                    Some("update"),
                    Some(json!({"source": options.source})),
                    Some(&err.to_string()),
                );
                return Err(err);
            }
        }
    }

    Ok(applied)
}

fn current_epoch_seconds() -> u64 {
    use std::time::{SystemTime, UNIX_EPOCH};
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap_or_default()
        .as_secs()
}

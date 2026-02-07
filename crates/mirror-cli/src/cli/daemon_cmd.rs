use super::shared::{run_sync_job, run_token_validity_checks};
use super::*;
pub(super) fn handle_daemon(args: DaemonArgs, audit: &AuditLogger) -> anyhow::Result<()> {
    let result: anyhow::Result<()> = (|| {
        let interval = std::time::Duration::from_secs(args.interval_seconds);
        let lock_path = match args.lock {
            Some(path) => path,
            None => default_lock_path()?,
        };
        if args.missing_remote == MissingRemotePolicyValue::Prompt {
            anyhow::bail!("daemon mode requires --missing-remote policy");
        }
        let config_path = args.config.unwrap_or(default_config_path()?);
        let cache_path = args.cache.unwrap_or(default_cache_path()?);
        let policy: MissingRemotePolicy = args.missing_remote.into();
        let update_applied = update::check_and_maybe_apply(update::AutoUpdateOptions {
            cache_path: &cache_path,
            interval_secs: 86_400,
            auto_apply: true,
            audit,
            force: true,
            interactive: false,
            source: "daemon",
            override_repo: None,
        })?;
        if update_applied {
            update::restart_current_process().context("restart after update apply")?;
            return Ok(());
        }
        let _ = run_token_validity_checks(&config_path, &cache_path, audit, "daemon", true);
        let audit_id = audit.record("daemon.start", AuditStatus::Ok, Some("daemon"), None, None)?;
        println!("Audit ID: {audit_id}");
        let job = || {
            run_sync_job(
                &config_path,
                &cache_path,
                policy,
                audit,
                args.audit_repo,
                args.jobs,
            )
        };
        if args.run_once {
            let ran = mirror_core::daemon::run_once_with_lock(&lock_path, job)?;
            if ran {
                let update_applied = update::check_and_maybe_apply(update::AutoUpdateOptions {
                    cache_path: &cache_path,
                    interval_secs: 86_400,
                    auto_apply: true,
                    audit,
                    force: false,
                    interactive: false,
                    source: "daemon",
                    override_repo: None,
                })?;
                if update_applied {
                    update::restart_current_process().context("restart after update apply")?;
                    return Ok(());
                }
                let _ =
                    run_token_validity_checks(&config_path, &cache_path, audit, "daemon", false);
            }
            let audit_id = audit.record(
                "daemon.run_once",
                AuditStatus::Ok,
                Some("daemon"),
                None,
                None,
            )?;
            println!("Audit ID: {audit_id}");
            return Ok(());
        }
        let mut failure_count: u32 = 0;
        loop {
            let ran = match mirror_core::daemon::run_once_with_lock(&lock_path, &job) {
                Ok(ran_flag) => {
                    if ran_flag {
                        info!("run completed");
                    }
                    failure_count = 0;
                    ran_flag
                }
                Err(err) => {
                    failure_count = failure_count.saturating_add(1);
                    warn!(error = %err, failures = failure_count, "run failed");
                    false
                }
            };
            if ran {
                let update_applied = update::check_and_maybe_apply(update::AutoUpdateOptions {
                    cache_path: &cache_path,
                    interval_secs: 86_400,
                    auto_apply: true,
                    audit,
                    force: false,
                    interactive: false,
                    source: "daemon",
                    override_repo: None,
                })?;
                if update_applied {
                    update::restart_current_process().context("restart after update apply")?;
                    return Ok(());
                }
                let _ =
                    run_token_validity_checks(&config_path, &cache_path, audit, "daemon", false);
            }
            std::thread::sleep(mirror_core::daemon::daemon_backoff_delay(
                interval,
                failure_count,
            ));
        }
    })();

    if let Err(err) = &result {
        let _ = audit.record(
            "daemon.start",
            AuditStatus::Failed,
            Some("daemon"),
            None,
            Some(&err.to_string()),
        );
    }
    result
}

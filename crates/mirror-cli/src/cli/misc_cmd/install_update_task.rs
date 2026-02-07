use super::*;
pub(in crate::cli) fn handle_install(
    args: InstallArgs,
    audit: &AuditLogger,
    log_buffer: logging::LogBuffer,
) -> anyhow::Result<()> {
    let result: anyhow::Result<()> = (|| {
        if args.status {
            let status = install::install_status()?;
            let service_label = service_label();
            println!("Installed: {}", if status.installed { "yes" } else { "no" });
            println!(
                "Installed path: {}",
                status
                    .installed_path
                    .as_ref()
                    .map(|p| p.display().to_string())
                    .unwrap_or_else(|| "(unknown)".to_string())
            );
            println!(
                "Manifest present: {}",
                if status.manifest_present { "yes" } else { "no" }
            );
            if let Some(value) = status.installed_version.as_deref() {
                println!("Installed version: {value}");
            }
            if let Some(value) = status.installed_at {
                println!("Installed at: {}", epoch_to_label(value));
            }
            println!(
                "Startup delay: {}",
                format_delayed_start(status.delayed_start)
            );
            println!(
                "{} installed: {}",
                service_label,
                if status.service_installed {
                    "yes"
                } else {
                    "no"
                }
            );
            println!(
                "{} running: {}",
                service_label,
                if status.service_running { "yes" } else { "no" }
            );
            if let Some(value) = status.service_last_run.as_deref() {
                println!("Last run: {value}");
            }
            if let Some(value) = status.service_next_run.as_deref() {
                println!("Next run: {value}");
            }
            if let Some(value) = status.service_last_result.as_deref() {
                println!("Last result: {value}");
            }
            if cfg!(target_os = "windows") {
                if let Some(value) = status.service_task_state.as_deref() {
                    println!("Task state: {value}");
                }
                if let Some(value) = status.service_schedule_type.as_deref() {
                    println!("Schedule type: {value}");
                }
                if let Some(value) = status.service_start_date.as_deref() {
                    println!("Start date: {value}");
                }
                if let Some(value) = status.service_start_time.as_deref() {
                    println!("Start time: {value}");
                }
                if let Some(value) = status.service_run_as.as_deref() {
                    println!("Run as: {value}");
                }
                if let Some(value) = status.service_task_to_run.as_deref() {
                    println!("Task command: {value}");
                }
                println!("Task name: git-project-sync");
            }
            println!(
                "PATH contains install dir (current shell): {}",
                if status.path_in_env { "yes" } else { "no" }
            );
            return Ok(());
        }
        if args.update {
            let status = install::install_status()?;
            if !status.installed {
                anyhow::bail!(
                    "update requested but no existing install was found (run `mirror-cli install` first)"
                );
            }
        }
        if args.tui {
            return tui::run_tui(audit, log_buffer, tui::StartView::Install);
        }
        let _guard = install::acquire_install_lock()?;
        println!("Starting installer (non-interactive).");
        let delayed_start = if args.non_interactive {
            args.delayed_start
        } else if let Some(value) = args.delayed_start {
            Some(value)
        } else {
            prompt_delay_seconds()?
        };
        let path_choice = match args.path {
            Some(choice) => choice.into(),
            None => {
                if args.non_interactive {
                    PathChoice::Skip
                } else {
                    prompt_path_choice()?
                }
            }
        };
        let exec = std::env::current_exe().context("resolve current executable")?;
        let last_len = std::cell::Cell::new(0usize);
        let report = install::perform_install_with_progress(
            &exec,
            InstallOptions {
                delayed_start,
                path_choice,
            },
            Some(&|progress| {
                let bar = render_progress_bar(progress.step, progress.total, 20);
                let line = format!(
                    "Step {}/{} {} {}",
                    progress.step, progress.total, bar, progress.message
                );
                let prev_len = last_len.get();
                if line.len() < prev_len {
                    print!("\r{line}{}", " ".repeat(prev_len - line.len()));
                } else {
                    print!("\r{line}");
                }
                last_len.set(line.len());
                let _ = io::stdout().flush();
            }),
            None,
        )
        .map_err(|err| {
            if update::is_permission_error(&err)
                && maybe_escalate_and_reexec("install").unwrap_or(false)
            {
                return anyhow::anyhow!("install escalated");
            }
            err
        })?;
        if last_len.get() > 0 {
            println!();
        }
        println!("{}", report.install);
        println!("{}", report.service);
        println!("{}", report.path);
        if args.start {
            mirror_core::service::start_service_now()
                .context("start service/task after install")?;
            println!("{} started.", service_label());
        }
        let audit_id = audit.record("install.run", AuditStatus::Ok, Some("install"), None, None)?;
        println!("Audit ID: {audit_id}");
        Ok(())
    })();

    if let Err(err) = &result {
        if err.to_string() == "install escalated" {
            return Ok(());
        }
        let _ = audit.record(
            "install.run",
            AuditStatus::Failed,
            Some("install"),
            None,
            Some(&err.to_string()),
        );
    }
    result
}

pub(in crate::cli) fn run_update_check(audit: &AuditLogger) -> anyhow::Result<()> {
    let check = update::check_for_update(None)?;
    let current = check.current.to_string();
    let latest = check.latest.to_string();
    if !check.is_newer {
        println!("Up to date ({current}).");
        let _ = audit.record("update.check", AuditStatus::Ok, Some("update"), None, None);
        return Ok(());
    }

    println!("Update available: {current} -> {latest}");
    if let Some(url) = check.release_url.as_deref() {
        println!("Release: {url}");
    }
    if check.asset.is_none() {
        println!("Update available but no release asset found for this platform.");
    }
    let _ = audit.record(
        "update.check",
        AuditStatus::Ok,
        Some("update"),
        Some(serde_json::json!({"current": current, "latest": latest, "available": true})),
        None,
    );
    Ok(())
}

pub(in crate::cli) fn handle_task(args: TaskArgs, audit: &AuditLogger) -> anyhow::Result<()> {
    let result: anyhow::Result<()> = (|| {
        if !cfg!(target_os = "windows") {
            println!("Task Scheduler is only supported on Windows.");
            let _ = audit.record(
                "task",
                AuditStatus::Skipped,
                Some("task"),
                None,
                Some("task scheduler unsupported"),
            );
            return Ok(());
        }
        match args.command {
            TaskCommands::Status => {
                let status = mirror_core::service::service_status()?;
                println!("Installed: {}", if status.installed { "yes" } else { "no" });
                println!("Running: {}", if status.running { "yes" } else { "no" });
                if let Some(value) = status.last_run_time.as_deref() {
                    println!("Last run: {value}");
                }
                if let Some(value) = status.next_run_time.as_deref() {
                    println!("Next run: {value}");
                }
                if let Some(value) = status.last_result.as_deref() {
                    println!("Last result: {value}");
                }
                println!("Task name: git-project-sync");
                let _ = audit.record("task.status", AuditStatus::Ok, Some("task"), None, None);
            }
            TaskCommands::Run => {
                mirror_core::service::start_service_now()?;
                println!("Task started.");
                let _ = audit.record("task.run", AuditStatus::Ok, Some("task"), None, None);
            }
            TaskCommands::Remove => {
                mirror_core::service::uninstall_service()?;
                println!("Task removed.");
                let _ = audit.record("task.remove", AuditStatus::Ok, Some("task"), None, None);
            }
        }
        Ok(())
    })();

    if let Err(err) = &result {
        let _ = audit.record(
            "task",
            AuditStatus::Failed,
            Some("task"),
            None,
            Some(&err.to_string()),
        );
    }
    result
}

pub(in crate::cli) fn handle_update(args: UpdateArgs, audit: &AuditLogger) -> anyhow::Result<()> {
    let result: anyhow::Result<()> = (|| {
        let check = update::check_for_update(args.repo.as_deref())?;
        let current = check.current.to_string();
        let latest = check.latest.to_string();
        if !check.is_newer {
            println!("Up to date ({current}).");
            let _ = audit.record("update.check", AuditStatus::Ok, Some("update"), None, None)?;
            return Ok(());
        }

        println!("Update available: {current} -> {latest}");
        if let Some(url) = check.release_url.as_deref() {
            println!("Release: {url}");
        }

        if check.asset.is_none() {
            anyhow::bail!("update available but no release asset found for this platform");
        }

        if args.check_only && !args.apply {
            let _ = audit.record(
                "update.check",
                AuditStatus::Ok,
                Some("update"),
                Some(serde_json::json!({"current": current, "latest": latest, "available": true})),
                None,
            )?;
            std::process::exit(2);
        }

        let should_apply = if args.yes {
            true
        } else {
            prompt_update_confirm(&latest)?
        };

        if !should_apply {
            println!("Update canceled.");
            let _ = audit.record(
                "update.apply",
                AuditStatus::Skipped,
                Some("update"),
                None,
                Some("user canceled"),
            );
            return Ok(());
        }

        let report = update::apply_update(&check).map_err(|err| {
            if update::is_permission_error(&err)
                && maybe_escalate_and_reexec("update").unwrap_or(false)
            {
                return anyhow::anyhow!("update escalated");
            }
            err
        })?;
        println!("{}", report.install);
        println!("{}", report.service);
        println!("{}", report.path);
        let _ = audit.record(
            "update.apply",
            AuditStatus::Ok,
            Some("update"),
            Some(serde_json::json!({"current": current, "latest": latest})),
            None,
        )?;
        update::restart_current_process().context("restart after update apply")?;
        Ok(())
    })();

    if let Err(err) = &result {
        if err.to_string() == "update escalated" {
            return Ok(());
        }
        let _ = audit.record(
            "update.apply",
            AuditStatus::Failed,
            Some("update"),
            None,
            Some(&err.to_string()),
        );
    }
    result
}

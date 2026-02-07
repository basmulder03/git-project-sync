use super::*;
pub(in crate::cli) fn admin_privileges_prompt_label() -> &'static str {
    if cfg!(target_os = "windows") {
        "User Account Control (UAC) prompt"
    } else if cfg!(target_os = "macos") {
        "macOS administrator privileges prompt"
    } else {
        "sudo password prompt"
    }
}

pub(in crate::cli) fn maybe_escalate_and_reexec(reason: &str) -> anyhow::Result<bool> {
    if !stdin_is_tty() || !stdout_is_tty() {
        return Ok(false);
    }
    let prompt_label = admin_privileges_prompt_label();
    println!(
        "Permission denied while attempting to {reason}. Re-run with elevated permissions? You will see the {prompt_label}. (y/n):"
    );
    let mut input = String::new();
    std::io::stdin().read_line(&mut input)?;
    if input.trim().to_lowercase() != "y" {
        return Ok(false);
    }

    let exe = std::env::current_exe().context("resolve current executable")?;
    let args: Vec<String> = std::env::args().skip(1).collect();

    if cfg!(target_os = "windows") {
        let exe_str = exe.to_string_lossy().replace('\'', "''");
        let arg_list = args
            .iter()
            .map(|arg| arg.replace('\'', "''"))
            .collect::<Vec<_>>()
            .join(" ");
        let command = format!(
            "Start-Process -Verb RunAs -FilePath '{}' -ArgumentList '{}'",
            exe_str, arg_list
        );
        Command::new("powershell")
            .args(["-Command", &command])
            .spawn()
            .context("launch elevated process")?;
        return Ok(true);
    }

    Command::new("sudo")
        .arg(exe)
        .args(args)
        .spawn()
        .context("launch elevated process")?;
    Ok(true)
}

pub(in crate::cli) fn audit_repo_progress(
    audit: &AuditLogger,
    category: &str,
    event: &str,
    runtime_target: &ProviderTarget,
    progress: &SyncProgress,
) {
    if !should_audit_action(progress.action) {
        return;
    }
    let repo_id = progress
        .repo_id
        .clone()
        .or_else(|| progress.repo_name.clone());
    let context = AuditContext {
        provider: Some(runtime_target.provider.as_prefix().to_string()),
        scope: Some(runtime_target.scope.segments().join("/")),
        repo_id,
        path: None,
    };
    let details = serde_json::json!({
        "action": progress.action.as_str(),
        "repo_name": progress.repo_name.clone(),
        "repo_id": progress.repo_id.clone(),
        "processed": progress.processed_repos,
        "total": progress.total_repos,
        "summary": {
            "cloned": progress.summary.cloned,
            "fast_forwarded": progress.summary.fast_forwarded,
            "up_to_date": progress.summary.up_to_date,
            "dirty": progress.summary.dirty,
            "diverged": progress.summary.diverged,
            "failed": progress.summary.failed,
            "missing_archived": progress.summary.missing_archived,
            "missing_removed": progress.summary.missing_removed,
            "missing_skipped": progress.summary.missing_skipped,
        }
    });
    let status = audit_status_for_action(progress.action);
    let _ = audit.record_with_context(event, status, Some(category), context, Some(details), None);
}

pub(in crate::cli) fn should_audit_action(action: SyncAction) -> bool {
    matches!(
        action,
        SyncAction::Cloned
            | SyncAction::FastForwarded
            | SyncAction::UpToDate
            | SyncAction::Dirty
            | SyncAction::Diverged
            | SyncAction::Failed
            | SyncAction::MissingArchived
            | SyncAction::MissingRemoved
            | SyncAction::MissingSkipped
    )
}

pub(in crate::cli) fn audit_status_for_action(action: SyncAction) -> AuditStatus {
    match action {
        SyncAction::Failed => AuditStatus::Failed,
        SyncAction::Dirty | SyncAction::Diverged | SyncAction::MissingSkipped => {
            AuditStatus::Skipped
        }
        _ => AuditStatus::Ok,
    }
}

pub(in crate::cli) fn prompt_delay_seconds() -> anyhow::Result<Option<u64>> {
    println!("Delayed start on boot? Enter seconds or leave empty:");
    let mut input = String::new();
    io::stdin().read_line(&mut input)?;
    let value = input.trim();
    if value.is_empty() {
        return Ok(None);
    }
    let seconds: u64 = value.parse().context("invalid delay seconds")?;
    Ok(Some(seconds))
}

pub(in crate::cli) fn prompt_path_choice() -> anyhow::Result<PathChoice> {
    println!("Add mirror-cli to PATH? (y/n):");
    let mut input = String::new();
    io::stdin().read_line(&mut input)?;
    let choice = input.trim().to_lowercase();
    Ok(if choice == "y" || choice == "yes" {
        PathChoice::Add
    } else {
        PathChoice::Skip
    })
}

pub(in crate::cli) fn prompt_update_confirm(latest: &str) -> anyhow::Result<bool> {
    println!("Apply update to {latest}? (y/n):");
    let mut input = String::new();
    io::stdin().read_line(&mut input)?;
    let choice = input.trim().to_lowercase();
    Ok(choice == "y" || choice == "yes")
}

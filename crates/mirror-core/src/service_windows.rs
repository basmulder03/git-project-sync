#[cfg(target_os = "windows")]
use anyhow::Context;
#[cfg(target_os = "windows")]
use std::path::Path;

#[cfg(target_os = "windows")]
use crate::service::{ServiceStatusInfo, default_service_status};
#[cfg(target_os = "windows")]
use crate::service_common::{run_schtasks, run_schtasks_output, schtasks_delay};

#[cfg(target_os = "windows")]
const SERVICE_NAME: &str = "git-project-sync";

#[cfg(target_os = "windows")]
pub(crate) fn install_windows(exec_path: &Path, delay_seconds: Option<u64>) -> anyhow::Result<()> {
    let task_name = SERVICE_NAME;
    let exec = exec_path.to_string_lossy();
    let task = format!("\"{}\" daemon --missing-remote skip", exec);
    let mut args = vec![
        "/Create".to_string(),
        "/TN".to_string(),
        task_name.to_string(),
        "/TR".to_string(),
        task,
        "/SC".to_string(),
        "ONSTART".to_string(),
        "/RL".to_string(),
        "HIGHEST".to_string(),
        "/RU".to_string(),
        "SYSTEM".to_string(),
        "/F".to_string(),
    ];
    if let Some(delay) = delay_seconds.filter(|value| *value > 0) {
        let delay_value = schtasks_delay(delay);
        args.push("/DELAY".to_string());
        args.push(delay_value);
    }
    run_schtasks(&args).with_context(|| {
        "install scheduled task failed. \
Run this command from an Administrator PowerShell and try again."
    })?;
    Ok(())
}

#[cfg(target_os = "windows")]
pub(crate) fn uninstall_windows() -> anyhow::Result<()> {
    let _ = run_schtasks(&[
        "/End".to_string(),
        "/TN".to_string(),
        SERVICE_NAME.to_string(),
    ]);
    let _ = run_schtasks(&[
        "/Delete".to_string(),
        "/TN".to_string(),
        SERVICE_NAME.to_string(),
        "/F".to_string(),
    ]);
    Ok(())
}

#[cfg(target_os = "windows")]
pub(crate) fn service_exists() -> anyhow::Result<bool> {
    Ok(run_schtasks(&[
        "/Query".to_string(),
        "/TN".to_string(),
        SERVICE_NAME.to_string(),
    ])
    .is_ok())
}

#[cfg(target_os = "windows")]
pub(crate) fn service_running() -> anyhow::Result<bool> {
    let output = run_schtasks_output(&[
        "/Query".to_string(),
        "/TN".to_string(),
        SERVICE_NAME.to_string(),
        "/FO".to_string(),
        "LIST".to_string(),
    ])?;
    Ok(output
        .lines()
        .any(|line| line.trim_start().starts_with("Status:") && line.contains("Running")))
}

#[cfg(target_os = "windows")]
pub(crate) fn start_service_now() -> anyhow::Result<()> {
    run_schtasks(&[
        "/Run".to_string(),
        "/TN".to_string(),
        SERVICE_NAME.to_string(),
    ])?;
    Ok(())
}

#[cfg(target_os = "windows")]
pub(crate) fn service_status() -> anyhow::Result<ServiceStatusInfo> {
    let output = match run_schtasks_output(&[
        "/Query".to_string(),
        "/TN".to_string(),
        SERVICE_NAME.to_string(),
        "/FO".to_string(),
        "LIST".to_string(),
        "/V".to_string(),
    ]) {
        Ok(output) => output,
        Err(_) => return Ok(default_service_status(false, false)),
    };

    let mut status = None;
    let mut last_run_time = None;
    let mut last_result = None;
    let mut next_run_time = None;
    let mut task_state = None;
    let mut schedule_type = None;
    let mut start_time = None;
    let mut start_date = None;
    let mut run_as_user = None;
    let mut task_to_run = None;

    for line in output.lines() {
        let line = line.trim();
        if let Some(value) = line.strip_prefix("Status:") {
            status = Some(value.trim().to_string());
        } else if let Some(value) = line.strip_prefix("Last Run Time:") {
            let value = value.trim();
            if !value.is_empty() && value != "N/A" {
                last_run_time = Some(value.to_string());
            }
        } else if let Some(value) = line.strip_prefix("Last Result:") {
            let value = value.trim();
            if !value.is_empty() && value != "N/A" {
                last_result = Some(value.to_string());
            }
        } else if let Some(value) = line.strip_prefix("Next Run Time:") {
            let value = value.trim();
            if !value.is_empty() && value != "N/A" {
                next_run_time = Some(value.to_string());
            }
        } else if let Some(value) = line.strip_prefix("Scheduled Task State:") {
            let value = value.trim();
            if !value.is_empty() && value != "N/A" {
                task_state = Some(value.to_string());
            }
        } else if let Some(value) = line.strip_prefix("Schedule Type:") {
            let value = value.trim();
            if !value.is_empty() && value != "N/A" {
                schedule_type = Some(value.to_string());
            }
        } else if let Some(value) = line.strip_prefix("Start Time:") {
            let value = value.trim();
            if !value.is_empty() && value != "N/A" {
                start_time = Some(value.to_string());
            }
        } else if let Some(value) = line.strip_prefix("Start Date:") {
            let value = value.trim();
            if !value.is_empty() && value != "N/A" {
                start_date = Some(value.to_string());
            }
        } else if let Some(value) = line.strip_prefix("Run As User:") {
            let value = value.trim();
            if !value.is_empty() && value != "N/A" {
                run_as_user = Some(value.to_string());
            }
        } else if let Some(value) = line.strip_prefix("Task To Run:") {
            let value = value.trim();
            if !value.is_empty() && value != "N/A" {
                task_to_run = Some(value.to_string());
            }
        }
    }

    let running = status
        .as_deref()
        .map(|value| value.eq_ignore_ascii_case("Running"))
        .unwrap_or(false);
    Ok(ServiceStatusInfo {
        installed: true,
        running,
        last_run_time,
        last_result,
        next_run_time,
        task_state,
        schedule_type,
        start_time,
        start_date,
        run_as_user,
        task_to_run,
    })
}

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
    let row = task_query_row()?;
    let status = row_field(&row, SCHTASKS_STATUS_INDEX);
    Ok(matches!(status.as_deref(), Some(value) if value.eq_ignore_ascii_case("Running")))
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
    let row = match task_query_row() {
        Ok(row) => row,
        Err(_) => return Ok(default_service_status(false, false)),
    };

    let status = row_field(&row, SCHTASKS_STATUS_INDEX);
    let last_run_time = row_field(&row, SCHTASKS_LAST_RUN_TIME_INDEX);
    let last_result = row_field(&row, SCHTASKS_LAST_RESULT_INDEX);
    let next_run_time = row_field(&row, SCHTASKS_NEXT_RUN_TIME_INDEX);
    let task_state = row_field(&row, SCHTASKS_TASK_STATE_INDEX);
    let schedule_type = row_field(&row, SCHTASKS_SCHEDULE_TYPE_INDEX);
    let start_time = row_field(&row, SCHTASKS_START_TIME_INDEX);
    let start_date = row_field(&row, SCHTASKS_START_DATE_INDEX);
    let run_as_user = row_field(&row, SCHTASKS_RUN_AS_USER_INDEX);
    let task_to_run = row_field(&row, SCHTASKS_TASK_TO_RUN_INDEX);

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

#[cfg(target_os = "windows")]
const SCHTASKS_NEXT_RUN_TIME_INDEX: usize = 2;
#[cfg(target_os = "windows")]
const SCHTASKS_STATUS_INDEX: usize = 3;
#[cfg(target_os = "windows")]
const SCHTASKS_LAST_RUN_TIME_INDEX: usize = 5;
#[cfg(target_os = "windows")]
const SCHTASKS_LAST_RESULT_INDEX: usize = 6;
#[cfg(target_os = "windows")]
const SCHTASKS_TASK_TO_RUN_INDEX: usize = 8;
#[cfg(target_os = "windows")]
const SCHTASKS_TASK_STATE_INDEX: usize = 11;
#[cfg(target_os = "windows")]
const SCHTASKS_RUN_AS_USER_INDEX: usize = 14;
#[cfg(target_os = "windows")]
const SCHTASKS_SCHEDULE_TYPE_INDEX: usize = 18;
#[cfg(target_os = "windows")]
const SCHTASKS_START_TIME_INDEX: usize = 19;
#[cfg(target_os = "windows")]
const SCHTASKS_START_DATE_INDEX: usize = 20;

#[cfg(target_os = "windows")]
fn task_query_row() -> anyhow::Result<Vec<String>> {
    let output = run_schtasks_output(&[
        "/Query".to_string(),
        "/TN".to_string(),
        SERVICE_NAME.to_string(),
        "/FO".to_string(),
        "CSV".to_string(),
        "/V".to_string(),
        "/NH".to_string(),
    ])?;
    output
        .lines()
        .find_map(|line| {
            let line = line.trim();
            if line.is_empty() {
                return None;
            }
            Some(parse_csv_row(line))
        })
        .ok_or_else(|| anyhow::anyhow!("unexpected schtasks output format"))
}

#[cfg(target_os = "windows")]
fn row_field(row: &[String], index: usize) -> Option<String> {
    row.get(index).and_then(|value| {
        let trimmed = value.trim();
        if trimmed.is_empty() || trimmed.eq_ignore_ascii_case("N/A") {
            None
        } else {
            Some(trimmed.to_string())
        }
    })
}

#[cfg(target_os = "windows")]
fn parse_csv_row(line: &str) -> Vec<String> {
    let mut values = Vec::new();
    let mut current = String::new();
    let mut in_quotes = false;
    let mut chars = line.chars().peekable();

    while let Some(ch) = chars.next() {
        match ch {
            '"' => {
                if in_quotes && matches!(chars.peek(), Some('"')) {
                    current.push('"');
                    let _ = chars.next();
                } else {
                    in_quotes = !in_quotes;
                }
            }
            ',' if !in_quotes => {
                values.push(current.trim().to_string());
                current.clear();
            }
            _ => current.push(ch),
        }
    }
    values.push(current.trim().to_string());
    values
}

#[cfg(all(test, target_os = "windows"))]
mod tests {
    use super::parse_csv_row;

    #[test]
    fn parse_csv_row_handles_escaped_quotes() {
        let row = parse_csv_row("\"A\",\"B, C\",\"it\"\"s\"");
        assert_eq!(row, vec!["A", "B, C", "it\"s"]);
    }
}

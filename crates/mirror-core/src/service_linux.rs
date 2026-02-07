#[cfg(target_os = "linux")]
use anyhow::Context;
#[cfg(target_os = "linux")]
use directories::BaseDirs;
#[cfg(target_os = "linux")]
use std::fs;
#[cfg(target_os = "linux")]
use std::path::{Path, PathBuf};

#[cfg(target_os = "linux")]
use crate::service_common::{command_success, ensure_dir_writable, run_command};

#[cfg(target_os = "linux")]
const SYSTEMD_UNIT_NAME: &str = "git-project-sync.service";
#[cfg(target_os = "linux")]
const SYSTEMD_TIMER_NAME: &str = "git-project-sync.timer";

#[cfg(target_os = "linux")]
pub(crate) fn install_systemd(exec_path: &Path, delay_seconds: Option<u64>) -> anyhow::Result<()> {
    let unit_path = systemd_unit_path()?;
    ensure_dir_writable(unit_path.parent(), "systemd user dir")?;
    let unit = systemd_unit_contents(exec_path);
    if let Some(parent) = unit_path.parent() {
        fs::create_dir_all(parent).context("create systemd user dir")?;
    }
    fs::write(&unit_path, unit).context("write systemd unit")?;
    run_command(
        "systemctl",
        &["--user", "daemon-reload"],
        "reload systemd user units",
    )?;
    if let Some(delay) = delay_seconds.filter(|value| *value > 0) {
        let timer_path = systemd_timer_path()?;
        ensure_dir_writable(timer_path.parent(), "systemd user dir")?;
        let timer = systemd_timer_contents(delay);
        fs::write(&timer_path, timer).context("write systemd timer")?;
        run_command(
            "systemctl",
            &["--user", "daemon-reload"],
            "reload systemd user units",
        )?;
        run_command(
            "systemctl",
            &["--user", "enable", "--now", SYSTEMD_TIMER_NAME],
            "enable systemd user timer",
        )?;
    } else {
        run_command(
            "systemctl",
            &["--user", "enable", "--now", SYSTEMD_UNIT_NAME],
            "enable systemd user service",
        )?;
    }
    Ok(())
}

#[cfg(target_os = "linux")]
pub(crate) fn uninstall_systemd() -> anyhow::Result<()> {
    let unit_path = systemd_unit_path()?;
    let timer_path = systemd_timer_path()?;
    run_command(
        "systemctl",
        &["--user", "disable", "--now", SYSTEMD_UNIT_NAME],
        "disable systemd user service",
    )
    .ok();
    run_command(
        "systemctl",
        &["--user", "disable", "--now", SYSTEMD_TIMER_NAME],
        "disable systemd user timer",
    )
    .ok();
    if unit_path.exists() {
        fs::remove_file(&unit_path).context("remove systemd unit")?;
    }
    if timer_path.exists() {
        fs::remove_file(&timer_path).context("remove systemd timer")?;
    }
    run_command(
        "systemctl",
        &["--user", "daemon-reload"],
        "reload systemd user units",
    )?;
    Ok(())
}

#[cfg(target_os = "linux")]
pub(crate) fn service_exists() -> anyhow::Result<bool> {
    Ok(systemd_unit_path()?.exists() || systemd_timer_path()?.exists())
}

#[cfg(target_os = "linux")]
pub(crate) fn start_service_now() -> anyhow::Result<()> {
    if systemd_timer_path()?.exists() {
        run_command(
            "systemctl",
            &["--user", "start", SYSTEMD_TIMER_NAME],
            "start systemd user timer",
        )?;
    } else {
        run_command(
            "systemctl",
            &["--user", "start", SYSTEMD_UNIT_NAME],
            "start systemd user service",
        )?;
    }
    Ok(())
}

#[cfg(target_os = "linux")]
pub(crate) fn service_running() -> anyhow::Result<bool> {
    if systemd_timer_path()?.exists() {
        return Ok(command_success(
            "systemctl",
            &["--user", "is-active", SYSTEMD_TIMER_NAME],
        ));
    }
    Ok(command_success(
        "systemctl",
        &["--user", "is-active", SYSTEMD_UNIT_NAME],
    ))
}

#[cfg(target_os = "linux")]
fn systemd_unit_path() -> anyhow::Result<PathBuf> {
    let base = BaseDirs::new().context("resolve base dirs")?;
    Ok(base
        .config_dir()
        .join("systemd")
        .join("user")
        .join(SYSTEMD_UNIT_NAME))
}

#[cfg(target_os = "linux")]
fn systemd_timer_path() -> anyhow::Result<PathBuf> {
    let base = BaseDirs::new().context("resolve base dirs")?;
    Ok(base
        .config_dir()
        .join("systemd")
        .join("user")
        .join(SYSTEMD_TIMER_NAME))
}

#[cfg(target_os = "linux")]
fn systemd_unit_contents(exec_path: &Path) -> String {
    let exec = exec_path.to_string_lossy();
    format!(
        "[Unit]\n\
Description=git-project-sync daemon\n\
After=network-online.target\n\n\
[Service]\n\
Type=simple\n\
ExecStart={exec} daemon --missing-remote skip\n\
Restart=on-failure\n\
RestartSec=10\n\n\
[Install]\n\
WantedBy=default.target\n"
    )
}

#[cfg(target_os = "linux")]
fn systemd_timer_contents(delay_seconds: u64) -> String {
    format!(
        "[Unit]\n\
Description=git-project-sync daemon timer\n\n\
[Timer]\n\
OnBootSec={delay_seconds}\n\
Unit={SYSTEMD_UNIT_NAME}\n\n\
[Install]\n\
WantedBy=timers.target\n"
    )
}

#[cfg(test)]
mod tests {
    #[cfg(target_os = "linux")]
    use super::{systemd_timer_contents, systemd_unit_contents};
    #[cfg(target_os = "linux")]
    use std::path::Path;

    #[cfg(target_os = "linux")]
    #[test]
    fn systemd_unit_includes_daemon_args() {
        let unit = systemd_unit_contents(Path::new("/usr/bin/mirror-cli"));
        assert!(unit.contains("daemon --missing-remote skip"));
        assert!(!unit.contains("git-project-sync.service"));
    }

    #[cfg(target_os = "linux")]
    #[test]
    fn systemd_timer_includes_delay() {
        let timer = systemd_timer_contents(120);
        assert!(timer.contains("OnBootSec=120"));
        assert!(timer.contains("git-project-sync.service"));
    }
}

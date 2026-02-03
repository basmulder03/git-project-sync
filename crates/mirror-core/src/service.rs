use anyhow::{Context, bail};
use directories::BaseDirs;
#[cfg(target_os = "macos")]
use directories::ProjectDirs;
use std::fs;
use std::path::{Path, PathBuf};
use std::process::Command;

const SERVICE_NAME: &str = "git-project-sync";
const SYSTEMD_UNIT_NAME: &str = "git-project-sync.service";
const LAUNCHD_LABEL: &str = "com.git-project-sync.daemon";

pub fn install_service(exec_path: &Path) -> anyhow::Result<()> {
    #[cfg(target_os = "linux")]
    {
        return install_systemd(exec_path);
    }
    #[cfg(target_os = "macos")]
    {
        return install_launchd(exec_path);
    }
    #[cfg(target_os = "windows")]
    {
        return install_windows(exec_path);
    }
    #[cfg(not(any(target_os = "linux", target_os = "macos", target_os = "windows")))]
    {
        bail!("service install not supported on this OS");
    }
}

pub fn uninstall_service() -> anyhow::Result<()> {
    #[cfg(target_os = "linux")]
    {
        return uninstall_systemd();
    }
    #[cfg(target_os = "macos")]
    {
        return uninstall_launchd();
    }
    #[cfg(target_os = "windows")]
    {
        return uninstall_windows();
    }
    #[cfg(not(any(target_os = "linux", target_os = "macos", target_os = "windows")))]
    {
        bail!("service uninstall not supported on this OS");
    }
}

#[cfg(target_os = "linux")]
fn install_systemd(exec_path: &Path) -> anyhow::Result<()> {
    let unit_path = systemd_unit_path()?;
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
    run_command(
        "systemctl",
        &["--user", "enable", "--now", SYSTEMD_UNIT_NAME],
        "enable systemd user service",
    )?;
    Ok(())
}

#[cfg(target_os = "linux")]
fn uninstall_systemd() -> anyhow::Result<()> {
    let unit_path = systemd_unit_path()?;
    run_command(
        "systemctl",
        &["--user", "disable", "--now", SYSTEMD_UNIT_NAME],
        "disable systemd user service",
    )
    .ok();
    if unit_path.exists() {
        fs::remove_file(&unit_path).context("remove systemd unit")?;
    }
    run_command(
        "systemctl",
        &["--user", "daemon-reload"],
        "reload systemd user units",
    )?;
    Ok(())
}

#[cfg(target_os = "linux")]
fn systemd_unit_path() -> anyhow::Result<PathBuf> {
    let base = BaseDirs::new().context("resolve base dirs")?;
    Ok(base.config_dir().join("systemd").join("user").join(SYSTEMD_UNIT_NAME))
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

#[cfg(target_os = "macos")]
fn install_launchd(exec_path: &Path) -> anyhow::Result<()> {
    let plist_path = launchd_plist_path()?;
    let log_dir = launchd_log_dir()?;
    let plist = launchd_plist_contents(exec_path)?;
    if let Some(parent) = plist_path.parent() {
        fs::create_dir_all(parent).context("create launchd agents dir")?;
    }
    fs::create_dir_all(&log_dir).context("create launchd log dir")?;
    fs::write(&plist_path, plist).context("write launchd plist")?;
    run_command(
        "launchctl",
        &["load", "-w", plist_path.to_string_lossy().as_ref()],
        "load launchd agent",
    )?;
    Ok(())
}

#[cfg(target_os = "macos")]
fn uninstall_launchd() -> anyhow::Result<()> {
    let plist_path = launchd_plist_path()?;
    run_command(
        "launchctl",
        &["unload", "-w", plist_path.to_string_lossy().as_ref()],
        "unload launchd agent",
    )
    .ok();
    if plist_path.exists() {
        fs::remove_file(&plist_path).context("remove launchd plist")?;
    }
    Ok(())
}

#[cfg(target_os = "macos")]
fn launchd_plist_path() -> anyhow::Result<PathBuf> {
    let base = BaseDirs::new().context("resolve base dirs")?;
    Ok(base.home_dir().join("Library").join("LaunchAgents").join(format!("{LAUNCHD_LABEL}.plist")))
}

#[cfg(target_os = "macos")]
fn launchd_plist_contents(exec_path: &Path) -> anyhow::Result<String> {
    let exec = exec_path.to_string_lossy();
    let log_dir = launchd_log_dir()?;
    let stdout = log_dir.join("daemon.out.log");
    let stderr = log_dir.join("daemon.err.log");
    Ok(format!(
        "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n\
<!DOCTYPE plist PUBLIC \"-//Apple//DTD PLIST 1.0//EN\" \"http://www.apple.com/DTDs/PropertyList-1.0.dtd\">\n\
<plist version=\"1.0\">\n\
<dict>\n\
  <key>Label</key>\n\
  <string>{label}</string>\n\
  <key>ProgramArguments</key>\n\
  <array>\n\
    <string>{exec}</string>\n\
    <string>daemon</string>\n\
    <string>--missing-remote</string>\n\
    <string>skip</string>\n\
  </array>\n\
  <key>RunAtLoad</key>\n\
  <true/>\n\
  <key>KeepAlive</key>\n\
  <true/>\n\
  <key>StandardOutPath</key>\n\
  <string>{stdout}</string>\n\
  <key>StandardErrorPath</key>\n\
  <string>{stderr}</string>\n\
</dict>\n\
</plist>\n",
        label = LAUNCHD_LABEL,
        exec = exec,
        stdout = stdout.to_string_lossy(),
        stderr = stderr.to_string_lossy(),
    ))
}

#[cfg(target_os = "macos")]
fn launchd_log_dir() -> anyhow::Result<PathBuf> {
    let project = ProjectDirs::from("com", SERVICE_NAME, SERVICE_NAME)
        .context("resolve project dirs")?;
    Ok(project.data_local_dir().join("logs"))
}

#[cfg(target_os = "windows")]
fn install_windows(exec_path: &Path) -> anyhow::Result<()> {
    let exec = exec_path.to_string_lossy();
    let bin_path = format!("\"{exec}\" daemon --missing-remote skip");
    run_command(
        "sc.exe",
        &["create", SERVICE_NAME, &format!("binPath= {bin_path}"), "start= auto"],
        "create windows service",
    )?;
    run_command(
        "sc.exe",
        &["start", SERVICE_NAME],
        "start windows service",
    )
    .ok();
    Ok(())
}

#[cfg(target_os = "windows")]
fn uninstall_windows() -> anyhow::Result<()> {
    run_command(
        "sc.exe",
        &["stop", SERVICE_NAME],
        "stop windows service",
    )
    .ok();
    run_command(
        "sc.exe",
        &["delete", SERVICE_NAME],
        "delete windows service",
    )?;
    Ok(())
}

fn run_command(binary: &str, args: &[&str], context_label: &str) -> anyhow::Result<()> {
    let status = Command::new(binary)
        .args(args)
        .status()
        .with_context(|| format!("run {binary} for {context_label}"))?;
    if !status.success() {
        bail!("{context_label} failed with status {status}");
    }
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[cfg(target_os = "linux")]
    #[test]
    fn systemd_unit_includes_daemon_args() {
        let unit = systemd_unit_contents(Path::new("/usr/bin/mirror-cli"));
        assert!(unit.contains("daemon --missing-remote skip"));
        assert!(!unit.contains("git-project-sync.service"));
    }

    #[cfg(target_os = "macos")]
    #[test]
    fn launchd_plist_includes_daemon_args() {
        let plist = launchd_plist_contents(Path::new("/usr/bin/mirror-cli")).unwrap();
        assert!(plist.contains("<string>daemon</string>"));
        assert!(plist.contains("<string>skip</string>"));
    }
}

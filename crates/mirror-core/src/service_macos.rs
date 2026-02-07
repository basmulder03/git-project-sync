#[cfg(target_os = "macos")]
use anyhow::Context;
#[cfg(target_os = "macos")]
use directories::{BaseDirs, ProjectDirs};
#[cfg(target_os = "macos")]
use std::fs;
#[cfg(target_os = "macos")]
use std::path::{Path, PathBuf};

#[cfg(target_os = "macos")]
use crate::service_common::{command_success, ensure_dir_writable, run_command};

#[cfg(target_os = "macos")]
const SERVICE_NAME: &str = "git-project-sync";
#[cfg(target_os = "macos")]
const LAUNCHD_LABEL: &str = "com.git-project-sync.daemon";

#[cfg(target_os = "macos")]
pub(crate) fn install_launchd(exec_path: &Path, delay_seconds: Option<u64>) -> anyhow::Result<()> {
    let plist_path = launchd_plist_path()?;
    let log_dir = launchd_log_dir()?;
    ensure_dir_writable(plist_path.parent(), "launchd agents dir")?;
    ensure_dir_writable(Some(&log_dir), "launchd log dir")?;
    let plist = launchd_plist_contents(exec_path, delay_seconds)?;
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
pub(crate) fn uninstall_launchd() -> anyhow::Result<()> {
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
pub(crate) fn service_exists() -> anyhow::Result<bool> {
    Ok(launchd_plist_path()?.exists())
}

#[cfg(target_os = "macos")]
pub(crate) fn start_service_now() -> anyhow::Result<()> {
    run_command(
        "launchctl",
        &["start", LAUNCHD_LABEL],
        "start launchd agent",
    )?;
    Ok(())
}

#[cfg(target_os = "macos")]
pub(crate) fn service_running() -> anyhow::Result<bool> {
    Ok(command_success("launchctl", &["list", LAUNCHD_LABEL]))
}

#[cfg(target_os = "macos")]
fn launchd_plist_path() -> anyhow::Result<PathBuf> {
    let base = BaseDirs::new().context("resolve base dirs")?;
    Ok(base
        .home_dir()
        .join("Library")
        .join("LaunchAgents")
        .join(format!("{LAUNCHD_LABEL}.plist")))
}

#[cfg(target_os = "macos")]
fn launchd_plist_contents(exec_path: &Path, delay_seconds: Option<u64>) -> anyhow::Result<String> {
    let exec = exec_path.to_string_lossy();
    let log_dir = launchd_log_dir()?;
    let stdout = log_dir.join("daemon.out.log");
    let stderr = log_dir.join("daemon.err.log");
    let start_interval = delay_seconds.filter(|value| *value > 0);
    let start_interval_block = start_interval.map(|value| {
        format!(
            "  <key>StartInterval</key>\n\
  <integer>{value}</integer>\n"
        )
    });
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
{start_interval}\
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
        start_interval = start_interval_block.unwrap_or_default(),
    ))
}

#[cfg(target_os = "macos")]
fn launchd_log_dir() -> anyhow::Result<PathBuf> {
    let project =
        ProjectDirs::from("com", SERVICE_NAME, SERVICE_NAME).context("resolve project dirs")?;
    Ok(project.data_local_dir().join("logs"))
}

#[cfg(test)]
mod tests {
    #[cfg(target_os = "macos")]
    use super::launchd_plist_contents;
    #[cfg(target_os = "macos")]
    use std::path::Path;

    #[cfg(target_os = "macos")]
    #[test]
    fn launchd_plist_includes_daemon_args() {
        let plist = launchd_plist_contents(Path::new("/usr/bin/mirror-cli"), None).unwrap();
        assert!(plist.contains("<string>daemon</string>"));
        assert!(plist.contains("<string>skip</string>"));
    }

    #[cfg(target_os = "macos")]
    #[test]
    fn launchd_plist_includes_start_interval() {
        let plist = launchd_plist_contents(Path::new("/usr/bin/mirror-cli"), Some(60)).unwrap();
        assert!(plist.contains("<key>StartInterval</key>"));
        assert!(plist.contains("<integer>60</integer>"));
    }
}

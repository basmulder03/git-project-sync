use anyhow::Context;
#[cfg(any(target_os = "linux", target_os = "macos"))]
use anyhow::bail;
#[cfg(any(target_os = "linux", target_os = "macos"))]
use std::fs;
#[cfg(any(target_os = "linux", target_os = "macos"))]
use std::io::ErrorKind;
#[cfg(any(target_os = "linux", target_os = "macos"))]
use std::path::Path;
use std::process::{Command, Stdio};

#[cfg(any(target_os = "linux", target_os = "macos"))]
pub(crate) fn command_success(binary: &str, args: &[&str]) -> bool {
    Command::new(binary)
        .args(args)
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .status()
        .map(|status| status.success())
        .unwrap_or(false)
}

#[cfg(any(target_os = "linux", target_os = "macos"))]
pub(crate) fn run_command(binary: &str, args: &[&str], context_label: &str) -> anyhow::Result<()> {
    let status = Command::new(binary)
        .args(args)
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .status()
        .with_context(|| format!("run {binary} for {context_label}"))?;
    if !status.success() {
        bail!("{context_label} failed with status {status}");
    }
    Ok(())
}

#[cfg(any(target_os = "linux", target_os = "macos"))]
pub(crate) fn ensure_dir_writable(path: Option<&Path>, label: &str) -> anyhow::Result<()> {
    let Some(path) = path else {
        return Ok(());
    };
    fs::create_dir_all(path).with_context(|| format!("create {label} at {}", path.display()))?;
    let probe = path.join(".mirror_cli_perm_check");
    match fs::write(&probe, b"") {
        Ok(()) => {
            let _ = fs::remove_file(&probe);
            Ok(())
        }
        Err(err) if err.kind() == ErrorKind::PermissionDenied => {
            bail!(
                "{label} is not writable at {}. \
If you intended a system-wide install, run with sudo (not yet supported). \
Otherwise, check your home directory permissions.",
                path.display()
            )
        }
        Err(err) => Err(err).with_context(|| format!("write permission check for {label}")),
    }
}

#[cfg(target_os = "windows")]
pub(crate) fn run_schtasks(args: &[String]) -> anyhow::Result<()> {
    let output = Command::new("schtasks")
        .args(args)
        .stdout(Stdio::piped())
        .stderr(Stdio::piped())
        .output()
        .context("run schtasks")?;
    if !output.status.success() {
        let stderr = String::from_utf8_lossy(&output.stderr).trim().to_string();
        let stdout = String::from_utf8_lossy(&output.stdout).trim().to_string();
        let message = if !stderr.is_empty() { &stderr } else { &stdout };
        if is_schtasks_access_denied(message) {
            return Err(std::io::Error::new(
                std::io::ErrorKind::PermissionDenied,
                format!("schtasks access denied: {message}"),
            ))
            .context("run schtasks");
        }
        anyhow::bail!("schtasks failed with status {}: {message}", output.status);
    }
    Ok(())
}

#[cfg(target_os = "windows")]
pub(crate) fn run_schtasks_output(args: &[String]) -> anyhow::Result<String> {
    let output = Command::new("schtasks")
        .args(args)
        .stdout(Stdio::piped())
        .stderr(Stdio::piped())
        .output()
        .context("run schtasks")?;
    if !output.status.success() {
        let stderr = String::from_utf8_lossy(&output.stderr).trim().to_string();
        let stdout = String::from_utf8_lossy(&output.stdout).trim().to_string();
        let message = if !stderr.is_empty() { &stderr } else { &stdout };
        if is_schtasks_access_denied(message) {
            return Err(std::io::Error::new(
                std::io::ErrorKind::PermissionDenied,
                format!("schtasks access denied: {message}"),
            ))
            .context("run schtasks");
        }
        anyhow::bail!("schtasks failed with status {}: {message}", output.status);
    }
    Ok(String::from_utf8_lossy(&output.stdout).to_string())
}

#[cfg(target_os = "windows")]
fn is_schtasks_access_denied(message: &str) -> bool {
    let normalized = message.to_ascii_lowercase();
    normalized.contains("access is denied") || normalized.contains("0x80070005")
}

#[cfg(target_os = "windows")]
pub(crate) fn schtasks_delay(delay_seconds: u64) -> String {
    let minutes = delay_seconds.div_ceil(60);
    let hours = minutes / 60;
    let mins = minutes % 60;
    format!("{:04}:{:02}", hours, mins)
}

use anyhow::Context;
#[cfg(unix)]
use directories::BaseDirs;
use std::path::Path;
use std::process::{Command, Stdio};
#[cfg(windows)]
use tracing::debug;

pub fn register_path(exec_path: &Path) -> anyhow::Result<String> {
    let dir = exec_path
        .parent()
        .ok_or_else(|| anyhow::anyhow!("executable path has no parent"))?;
    if cfg!(target_os = "windows") {
        return add_path_windows(dir);
    }
    add_path_unix(dir)
}

#[cfg(unix)]
fn add_path_unix(dir: &Path) -> anyhow::Result<String> {
    let base = BaseDirs::new().context("resolve base dirs")?;
    let user_bin = base.home_dir().join(".local").join("bin");
    std::fs::create_dir_all(&user_bin).context("create user bin dir")?;
    let target = user_bin.join("mirror-cli");
    if target.exists() {
        return Ok(format!("PATH entry already exists at {}", target.display()));
    }
    std::os::unix::fs::symlink(dir.join("mirror-cli"), &target)
        .context("create symlink for mirror-cli")?;
    Ok(format!("Symlinked mirror-cli to {}", target.display()))
}

#[cfg(not(unix))]
fn add_path_unix(_dir: &Path) -> anyhow::Result<String> {
    anyhow::bail!("PATH install is only supported on Unix-like systems")
}

fn add_path_windows(dir: &Path) -> anyhow::Result<String> {
    let current = std::env::var("PATH").unwrap_or_default();
    let dir_str = dir.to_string_lossy().to_string();
    if current.split(';').any(|p| p.eq_ignore_ascii_case(&dir_str)) {
        return Ok("PATH already contains mirror-cli directory".to_string());
    }
    let updated = build_path_update(&current, &dir_str);
    Command::new("setx")
        .args(["PATH", &updated])
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .status()
        .context("update PATH with setx")?;
    Ok("Updated user PATH (restart shell to apply)".to_string())
}

pub(in crate::install) fn build_path_update(current: &str, add: &str) -> String {
    if current.trim().is_empty() {
        return add.to_string();
    }
    format!("{current};{add}")
}

pub(in crate::install) fn path_contains_dir(dir: &Path) -> bool {
    let current = std::env::var_os("PATH").unwrap_or_default();
    #[cfg(windows)]
    let dir = resolve_windows_path(dir);
    std::env::split_paths(&current).any(|path| {
        #[cfg(windows)]
        {
            let path = resolve_windows_path(&path);
            eq_ignore_ascii_case_wide(&path, &dir)
        }
        #[cfg(not(windows))]
        {
            path == dir
        }
    })
}

#[cfg(windows)]
pub(in crate::install) fn resolve_windows_path(path: &Path) -> std::path::PathBuf {
    path.canonicalize().unwrap_or_else(|err| {
        debug!(
            path = %path.display(),
            error = %err,
            "Could not resolve absolute path for PATH entry (path may not exist yet), using original path"
        );
        path.to_path_buf()
    })
}

#[cfg(windows)]
pub(in crate::install) fn eq_ignore_ascii_case_wide(left: &Path, right: &Path) -> bool {
    use std::os::windows::ffi::OsStrExt;

    let left = left.as_os_str().encode_wide();
    let right = right.as_os_str().encode_wide();
    left.map(ascii_lowercase_wide)
        .eq(right.map(ascii_lowercase_wide))
}

#[cfg(windows)]
pub(in crate::install) fn ascii_lowercase_wide(value: u16) -> u16 {
    const ASCII_UPPERCASE_A: u16 = b'A' as u16;
    const ASCII_UPPERCASE_Z: u16 = b'Z' as u16;
    const ASCII_CASE_OFFSET: u16 = 32;
    if (ASCII_UPPERCASE_A..=ASCII_UPPERCASE_Z).contains(&value) {
        value + ASCII_CASE_OFFSET
    } else {
        value
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn build_path_update_appends() {
        let updated = build_path_update("C:\\bin", "C:\\new");
        assert_eq!(updated, "C:\\bin;C:\\new");
    }

    #[cfg(windows)]
    #[test]
    fn ascii_lowercase_wide_handles_ascii() {
        assert_eq!(ascii_lowercase_wide(b'A' as u16), b'a' as u16);
        assert_eq!(ascii_lowercase_wide(b'Z' as u16), b'z' as u16);
        assert_eq!(ascii_lowercase_wide(b'a' as u16), b'a' as u16);
        assert_eq!(ascii_lowercase_wide(0x00DF), 0x00DF);
    }

    #[cfg(windows)]
    #[test]
    fn eq_ignore_ascii_case_wide_matches_paths() {
        let left = Path::new("C:\\Test\\Path");
        let right = Path::new("c:\\test\\path");
        assert!(eq_ignore_ascii_case_wide(left, right));
    }

    #[cfg(windows)]
    #[test]
    fn resolve_windows_path_uses_canonical_or_original() {
        let temp = tempfile::TempDir::new().unwrap();
        let existing = temp.path().to_path_buf();
        let canonical = existing.canonicalize().unwrap();
        assert_eq!(resolve_windows_path(&existing), canonical);

        let missing = existing.join("missing");
        assert_eq!(resolve_windows_path(&missing), missing);
    }
}

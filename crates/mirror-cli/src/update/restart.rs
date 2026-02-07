use super::*;

struct RestartCommand {
    exec: PathBuf,
    args: Vec<String>,
}

pub(in crate::update) fn choose_restart_path(
    installed_path: Option<PathBuf>,
    current_exe: PathBuf,
) -> PathBuf {
    if let Some(path) = installed_path
        && path.exists()
    {
        return path;
    }
    current_exe
}

fn restart_command() -> anyhow::Result<RestartCommand> {
    let current_exe = std::env::current_exe().context("resolve current executable")?;
    let installed_path = crate::install::install_status()
        .ok()
        .and_then(|status| status.installed_path);
    Ok(RestartCommand {
        exec: choose_restart_path(installed_path, current_exe),
        args: std::env::args().skip(1).collect(),
    })
}

#[cfg(unix)]
pub fn restart_current_process() -> anyhow::Result<()> {
    use std::os::unix::process::CommandExt;

    let command = restart_command()?;
    let err = Command::new(command.exec).args(command.args).exec();
    Err(err).context("re-exec mirror-cli")
}

#[cfg(not(unix))]
pub fn restart_current_process() -> anyhow::Result<()> {
    let command = restart_command()?;
    Command::new(command.exec)
        .args(command.args)
        .spawn()
        .context("restart mirror-cli")?;
    std::process::exit(0);
}

use super::*;

pub(in crate::tui) fn validation_timestamp() -> String {
    ::time::OffsetDateTime::now_utc()
        .format(
            &::time::format_description::parse("[year]-[month]-[day] [hour]:[minute]:[second]")
                .unwrap(),
        )
        .unwrap_or_else(|_| "unknown".to_string())
}

pub(in crate::tui) fn epoch_to_label(epoch: u64) -> String {
    let ts = ::time::OffsetDateTime::from_unix_timestamp(epoch as i64)
        .unwrap_or_else(|_| ::time::OffsetDateTime::now_utc());
    ts.format(&::time::format_description::parse("[year]-[month]-[day] [hour]:[minute]").unwrap())
        .unwrap_or_else(|_| "unknown".to_string())
}

pub(in crate::tui) fn format_delayed_start(delay: Option<u64>) -> String {
    match delay.filter(|value| *value > 0) {
        Some(value) => format!("{value}s"),
        None => "none".to_string(),
    }
}

pub(in crate::tui) fn install_action_from_status(
    status: Option<&crate::install::InstallStatus>,
) -> InstallAction {
    let Some(status) = status else {
        return InstallAction::Install;
    };
    let running_from_install = status
        .installed_path
        .as_ref()
        .and_then(|path| std::env::current_exe().ok().map(|exe| exe == *path))
        .unwrap_or(false);
    install_action_for_versions(
        status.installed,
        status.installed_version.as_deref(),
        env!("CARGO_PKG_VERSION"),
        running_from_install,
    )
}

pub(in crate::tui) fn install_action_for_versions(
    installed: bool,
    installed_version: Option<&str>,
    current_version: &str,
    running_from_install: bool,
) -> InstallAction {
    if !installed {
        return InstallAction::Install;
    }
    if !running_from_install
        && let (Ok(current), Some(installed)) = (
            Version::parse(current_version),
            installed_version.and_then(|value| Version::parse(value).ok()),
        )
        && current > installed
    {
        return InstallAction::Update;
    }
    InstallAction::Reinstall
}

pub(in crate::tui) fn install_state_from_status(
    status: Option<&crate::install::InstallStatus>,
    action: InstallAction,
) -> InstallState {
    let Some(status) = status else {
        return InstallState::Unknown;
    };
    if !status.installed {
        return InstallState::NotInstalled;
    }
    if matches!(action, InstallAction::Update) {
        return InstallState::UpdateReady;
    }
    InstallState::Installed
}

pub(in crate::tui) fn current_epoch_seconds() -> u64 {
    use std::time::{SystemTime, UNIX_EPOCH};
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap_or_default()
        .as_secs()
}

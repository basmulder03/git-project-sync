use super::{InstallStatus, metadata, path_env};

pub fn install_status() -> anyhow::Result<InstallStatus> {
    let manifest = metadata::read_manifest()?;
    let installed_path = manifest.as_ref().map(|m| m.installed_path.clone());
    let manifest_present = manifest.is_some();
    let installed_version = manifest.as_ref().map(|m| m.installed_version.clone());
    let installed_at = manifest.as_ref().map(|m| m.installed_at);
    let delayed_start = manifest.as_ref().and_then(|m| m.delayed_start);
    let marker_present = metadata::marker_exists()?;
    let installed = installed_path
        .as_ref()
        .map(|path| path.exists())
        .unwrap_or(marker_present);
    let service_installed = mirror_core::service::service_exists().unwrap_or(false);
    let service_running = mirror_core::service::service_running().unwrap_or(false);
    let service_status = mirror_core::service::service_status().ok();
    let path_dir = installed_path
        .as_ref()
        .and_then(|path| path.parent().map(|p| p.to_path_buf()));
    let install_dir_for_path_check = if installed_path.is_none() {
        metadata::default_install_dir().ok()
    } else {
        path_dir
    };
    let path_in_env = install_dir_for_path_check
        .as_ref()
        .map(|dir| path_env::path_contains_dir(dir))
        .unwrap_or(false);
    Ok(InstallStatus {
        installed,
        installed_path,
        manifest_present,
        installed_version,
        installed_at,
        delayed_start,
        service_installed,
        service_running,
        service_last_run: service_status
            .as_ref()
            .and_then(|s| s.last_run_time.clone()),
        service_last_result: service_status.as_ref().and_then(|s| s.last_result.clone()),
        service_next_run: service_status
            .as_ref()
            .and_then(|s| s.next_run_time.clone()),
        service_task_state: service_status.as_ref().and_then(|s| s.task_state.clone()),
        service_schedule_type: service_status
            .as_ref()
            .and_then(|s| s.schedule_type.clone()),
        service_start_time: service_status.as_ref().and_then(|s| s.start_time.clone()),
        service_start_date: service_status.as_ref().and_then(|s| s.start_date.clone()),
        service_run_as: service_status.as_ref().and_then(|s| s.run_as_user.clone()),
        service_task_to_run: service_status.as_ref().and_then(|s| s.task_to_run.clone()),
        path_in_env,
    })
}

use std::path::Path;

#[derive(Debug, Clone)]
pub struct ServiceStatusInfo {
    pub installed: bool,
    pub running: bool,
    pub last_run_time: Option<String>,
    pub last_result: Option<String>,
    pub next_run_time: Option<String>,
    pub task_state: Option<String>,
    pub schedule_type: Option<String>,
    pub start_time: Option<String>,
    pub start_date: Option<String>,
    pub run_as_user: Option<String>,
    pub task_to_run: Option<String>,
}

pub(crate) fn default_service_status(installed: bool, running: bool) -> ServiceStatusInfo {
    ServiceStatusInfo {
        installed,
        running,
        last_run_time: None,
        last_result: None,
        next_run_time: None,
        task_state: None,
        schedule_type: None,
        start_time: None,
        start_date: None,
        run_as_user: None,
        task_to_run: None,
    }
}

pub fn install_service(exec_path: &Path) -> anyhow::Result<()> {
    install_service_with_delay(exec_path, None)
}

pub fn install_service_with_delay(
    exec_path: &Path,
    delay_seconds: Option<u64>,
) -> anyhow::Result<()> {
    #[cfg(target_os = "linux")]
    {
        crate::service_linux::install_systemd(exec_path, delay_seconds)
    }
    #[cfg(target_os = "macos")]
    {
        return crate::service_macos::install_launchd(exec_path, delay_seconds);
    }
    #[cfg(target_os = "windows")]
    {
        crate::service_windows::install_windows(exec_path, delay_seconds)
    }
    #[cfg(not(any(target_os = "linux", target_os = "macos", target_os = "windows")))]
    {
        anyhow::bail!("service install not supported on this OS");
    }
}

pub fn uninstall_service() -> anyhow::Result<()> {
    #[cfg(target_os = "linux")]
    {
        crate::service_linux::uninstall_systemd()
    }
    #[cfg(target_os = "macos")]
    {
        return crate::service_macos::uninstall_launchd();
    }
    #[cfg(target_os = "windows")]
    {
        crate::service_windows::uninstall_windows()
    }
    #[cfg(not(any(target_os = "linux", target_os = "macos", target_os = "windows")))]
    {
        anyhow::bail!("service uninstall not supported on this OS");
    }
}

pub fn service_exists() -> anyhow::Result<bool> {
    #[cfg(target_os = "windows")]
    {
        crate::service_windows::service_exists()
    }
    #[cfg(target_os = "linux")]
    {
        crate::service_linux::service_exists()
    }
    #[cfg(target_os = "macos")]
    {
        return crate::service_macos::service_exists();
    }
    #[cfg(not(any(target_os = "linux", target_os = "macos", target_os = "windows")))]
    {
        Ok(false)
    }
}

pub fn service_running() -> anyhow::Result<bool> {
    #[cfg(target_os = "windows")]
    {
        crate::service_windows::service_running()
    }
    #[cfg(target_os = "linux")]
    {
        crate::service_linux::service_running()
    }
    #[cfg(target_os = "macos")]
    {
        return crate::service_macos::service_running();
    }
    #[cfg(not(any(target_os = "linux", target_os = "macos", target_os = "windows")))]
    {
        Ok(false)
    }
}

pub fn service_status() -> anyhow::Result<ServiceStatusInfo> {
    #[cfg(target_os = "windows")]
    {
        crate::service_windows::service_status()
    }
    #[cfg(any(target_os = "linux", target_os = "macos"))]
    {
        let installed = service_exists().unwrap_or(false);
        let running = service_running().unwrap_or(false);
        Ok(default_service_status(installed, running))
    }
    #[cfg(not(any(target_os = "linux", target_os = "macos", target_os = "windows")))]
    {
        Ok(default_service_status(false, false))
    }
}

pub fn start_service_now() -> anyhow::Result<()> {
    #[cfg(target_os = "windows")]
    {
        crate::service_windows::start_service_now()
    }
    #[cfg(target_os = "linux")]
    {
        crate::service_linux::start_service_now()
    }
    #[cfg(target_os = "macos")]
    {
        return crate::service_macos::start_service_now();
    }
    #[cfg(not(any(target_os = "linux", target_os = "macos", target_os = "windows")))]
    {
        Ok(())
    }
}

pub fn uninstall_service_if_exists() -> anyhow::Result<()> {
    #[cfg(target_os = "windows")]
    {
        if !crate::service_windows::service_exists()? {
            return Ok(());
        }
        crate::service_windows::uninstall_windows()
    }
    #[cfg(target_os = "linux")]
    {
        crate::service_linux::uninstall_systemd()
    }
    #[cfg(target_os = "macos")]
    {
        return crate::service_macos::uninstall_launchd();
    }
    #[cfg(not(any(target_os = "linux", target_os = "macos", target_os = "windows")))]
    {
        Ok(())
    }
}

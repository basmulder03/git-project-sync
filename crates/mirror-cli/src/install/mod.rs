use mirror_core::lockfile::LockFile;
use std::path::{Path, PathBuf};
use std::sync::{Arc, Mutex, OnceLock};
use tracing::info;

mod lock;
mod metadata;
mod path_env;
mod status;

pub use lock::acquire_install_lock;
pub use metadata::{is_installed, remove_manifest, remove_marker, write_marker};
pub use path_env::register_path;
pub use status::install_status;

pub struct InstallProgress {
    pub step: usize,
    pub total: usize,
    pub message: String,
}

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum PathChoice {
    Add,
    Skip,
}

#[derive(Clone, Copy, Debug)]
pub struct InstallOptions {
    pub delayed_start: Option<u64>,
    pub path_choice: PathChoice,
}

#[derive(Clone, Debug)]
pub struct InstallReport {
    pub install: String,
    pub service: String,
    pub path: String,
}

#[derive(Clone, Debug)]
pub struct InstallStatus {
    pub installed: bool,
    pub installed_path: Option<PathBuf>,
    pub manifest_present: bool,
    pub installed_version: Option<String>,
    pub installed_at: Option<u64>,
    pub delayed_start: Option<u64>,
    pub service_installed: bool,
    pub service_running: bool,
    pub service_last_run: Option<String>,
    pub service_last_result: Option<String>,
    pub service_next_run: Option<String>,
    pub service_task_state: Option<String>,
    pub service_schedule_type: Option<String>,
    pub service_start_time: Option<String>,
    pub service_start_date: Option<String>,
    pub service_run_as: Option<String>,
    pub service_task_to_run: Option<String>,
    pub path_in_env: bool,
}

pub struct InstallGuard {
    lock: Arc<LockFile>,
}

static INSTALL_LOCK: OnceLock<Mutex<Option<Arc<LockFile>>>> = OnceLock::new();

impl Drop for InstallGuard {
    fn drop(&mut self) {
        let cell = INSTALL_LOCK.get_or_init(|| Mutex::new(None));
        let mut guard = cell.lock().expect("install lock mutex");
        if Arc::strong_count(&self.lock) == 2 {
            *guard = None;
        }
    }
}

pub fn perform_install_with_progress(
    exec_path: &Path,
    options: InstallOptions,
    progress: Option<&dyn Fn(InstallProgress)>,
    installed_version: Option<&str>,
) -> anyhow::Result<InstallReport> {
    let status = install_status().ok();
    let is_update = status.as_ref().map(|s| s.installed).unwrap_or(false);
    let install_path = metadata::resolve_install_path(
        exec_path,
        status
            .as_ref()
            .and_then(|value| value.installed_path.as_deref()),
    )?;
    let path_in_env = install_path
        .parent()
        .map(path_env::path_contains_dir)
        .unwrap_or(false);
    let update_path = matches!(options.path_choice, PathChoice::Add) && !path_in_env;
    let delayed_start = options.delayed_start.or_else(|| {
        if is_update {
            status.as_ref().and_then(|value| value.delayed_start)
        } else {
            None
        }
    });
    let total = if update_path { 5 } else { 4 };
    let mut step = 0;
    step += 1;
    report_progress(
        progress,
        step,
        total,
        if is_update {
            "Preparing update"
        } else {
            "Preparing install"
        },
    );
    mirror_core::service::uninstall_service_if_exists().ok();
    step += 1;
    report_progress(
        progress,
        step,
        total,
        &format!(
            "{} binary to {}",
            if is_update { "Updating" } else { "Installing" },
            install_path.display()
        ),
    );
    let install_message = metadata::install_binary(exec_path, &install_path, is_update)?;
    step += 1;
    report_progress(
        progress,
        step,
        total,
        if is_update {
            "Updating service"
        } else {
            "Installing service"
        },
    );
    mirror_core::service::install_service_with_delay(&install_path, delayed_start)?;
    let service_action = if is_update { "updated" } else { "installed" };
    let service = match delayed_start {
        Some(delay) if delay > 0 => {
            format!(
                "{} {service_action} with delayed start ({delay}s)",
                service_label()
            )
        }
        _ => format!("{} {service_action}", service_label()),
    };
    let path = if update_path {
        step += 1;
        report_progress(progress, step, total, "Registering PATH entry");
        register_path(&install_path)?
    } else if matches!(options.path_choice, PathChoice::Add) {
        "PATH already contains install directory".to_string()
    } else {
        "PATH update skipped".to_string()
    };
    step += 1;
    report_progress(progress, step, total, "Writing install metadata");
    write_marker()?;
    metadata::write_manifest(&install_path, installed_version, delayed_start)?;
    Ok(InstallReport {
        install: install_message,
        service,
        path,
    })
}

fn service_label() -> &'static str {
    if cfg!(target_os = "windows") {
        "Scheduled task"
    } else {
        "Service"
    }
}

fn report_progress(
    progress: Option<&dyn Fn(InstallProgress)>,
    step: usize,
    total: usize,
    message: &str,
) {
    if progress.is_none() {
        info!(step = message, "installer progress");
    }
    if let Some(callback) = progress {
        callback(InstallProgress {
            step,
            total,
            message: message.to_string(),
        });
    }
}

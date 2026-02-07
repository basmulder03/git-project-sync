use super::helpers::*;
use super::*;

impl TuiApp {
    pub(super) fn load(
        audit: AuditLogger,
        log_buffer: LogBuffer,
        start_view: StartView,
    ) -> anyhow::Result<Self> {
        let config_path = default_config_path()?;
        let (config, migrated) = load_or_migrate(&config_path)?;
        if migrated {
            config.save(&config_path)?;
        }
        let view = match start_view {
            StartView::Dashboard => View::Dashboard,
            StartView::Install => View::Install,
            StartView::Main => View::Main,
        };
        let mut app = Self {
            config_path,
            config,
            view,
            menu_index: 0,
            message: String::new(),
            input_index: 0,
            input_fields: Vec::new(),
            provider_index: 0,
            token_menu_index: 0,
            token_validation: HashMap::new(),
            audit,
            log_buffer,
            audit_filter: AuditFilter::All,
            validation_message: None,
            show_target_stats: false,
            repo_status: HashMap::new(),
            repo_status_last_refresh: None,
            repo_status_refreshing: false,
            repo_status_rx: None,
            repo_overview_message: None,
            repo_overview_selected: 0,
            repo_overview_scroll: 0,
            repo_overview_collapsed: HashSet::new(),
            repo_overview_compact: false,
            sync_running: false,
            sync_rx: None,
            install_guard: None,
            install_rx: None,
            install_progress: None,
            install_status: None,
            install_scroll: 0,
            update_rx: None,
            update_progress: None,
            update_prompt: None,
            update_return_view: View::Main,
            restart_requested: false,
            message_return_view: View::Main,
            audit_scroll: 0,
            audit_search: String::new(),
            audit_search_active: false,
        };
        if app.view == View::Install {
            app.enter_install_view()?;
        }
        Ok(app)
    }

    pub(super) fn prepare_install_form(&mut self) {
        let status = crate::install::install_status().ok();
        let mut delayed_start = InputField::new("Delayed start seconds (optional)");
        if let Some(value) = status
            .as_ref()
            .and_then(|state| state.delayed_start)
            .filter(|value| *value > 0)
        {
            delayed_start.value = value.to_string();
        }
        let mut path = InputField::new("Add CLI to PATH? (y/n)");
        if status
            .as_ref()
            .map(|state| state.path_in_env)
            .unwrap_or(false)
        {
            path.value = "y".to_string();
        } else {
            path.value = "n".to_string();
        }
        self.input_fields = vec![delayed_start, path];
        self.input_index = 0;
    }

    pub(super) fn ensure_install_guard(&mut self) -> anyhow::Result<()> {
        if self.install_guard.is_none() {
            self.install_guard = Some(crate::install::acquire_install_lock()?);
        }
        Ok(())
    }

    pub(super) fn release_install_guard(&mut self) {
        self.install_guard = None;
    }

    pub(super) fn enter_install_view(&mut self) -> anyhow::Result<()> {
        self.ensure_install_guard()?;
        info!("Entered install view");
        self.view = View::Install;
        self.install_scroll = 0;
        self.prepare_install_form();
        self.drain_input_events()?;
        Ok(())
    }

    pub(super) fn drain_input_events(&self) -> anyhow::Result<()> {
        while event::poll(Duration::from_millis(0))? {
            let _ = event::read()?;
        }
        Ok(())
    }
}

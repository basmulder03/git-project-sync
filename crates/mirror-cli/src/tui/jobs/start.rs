use super::*;

impl TuiApp {
    pub(in crate::tui) fn start_update_check(&mut self, return_view: View) -> anyhow::Result<()> {
        info!(return_view = ?return_view, "Starting update check");
        self.update_return_view = return_view;
        self.update_progress = Some(UpdateProgressState::new());
        self.view = View::UpdateProgress;
        let (tx, rx) = mpsc::channel::<UpdateEvent>();
        self.update_rx = Some(rx);
        thread::spawn(move || {
            let result = update::check_for_update(None).map_err(|err| err.to_string());
            let _ = tx.send(UpdateEvent::Checked(result));
        });
        Ok(())
    }

    pub(in crate::tui) fn start_update_apply(
        &mut self,
        check: update::UpdateCheck,
    ) -> anyhow::Result<()> {
        info!(current = %check.current, latest = %check.latest, "Applying update");
        self.update_progress = Some(UpdateProgressState {
            messages: vec!["Starting update...".to_string()],
        });
        self.view = View::UpdateProgress;
        let (tx, rx) = mpsc::channel::<UpdateEvent>();
        self.update_rx = Some(rx);
        thread::spawn(move || {
            let result = update::apply_update_with_progress(
                &check,
                Some(&|message| {
                    let _ = tx.send(UpdateEvent::Progress(message.to_string()));
                }),
            )
            .map_err(|err| err.to_string());
            let _ = tx.send(UpdateEvent::Done(result));
        });
        Ok(())
    }

    pub(in crate::tui) fn start_sync_run(&mut self, force_refresh_all: bool) -> anyhow::Result<()> {
        if self.sync_running {
            warn!("Sync already running");
            self.message = "Sync already running.".to_string();
            self.view = View::Message;
            return Ok(());
        }
        let root = match self.config.root.clone() {
            Some(root) => root,
            None => {
                warn!("Sync requested without configured root");
                self.message = "Config missing root; run config init.".to_string();
                self.view = View::Message;
                return Ok(());
            }
        };
        if self.config.targets.is_empty() {
            warn!("Sync requested without configured targets");
            self.message = "No targets configured.".to_string();
            self.view = View::Message;
            return Ok(());
        }

        let lock_path = default_lock_path()?;
        let lock = match LockFile::try_acquire(&lock_path)? {
            Some(lock) => lock,
            None => {
                warn!(path = %lock_path.display(), "Sync lock already held");
                self.message = "Sync already running (lock held).".to_string();
                self.view = View::Message;
                return Ok(());
            }
        };

        let targets = self.config.targets.clone();
        let audit = self.audit.clone();
        let cache_path = default_cache_path()?;
        let (tx, rx) = mpsc::channel::<Result<SyncSummary, String>>();
        self.sync_rx = Some(rx);
        self.sync_running = true;
        self.view = View::SyncStatus;
        info!(
            targets = targets.len(),
            root = %root.display(),
            force_refresh_all,
            "Starting sync"
        );

        thread::spawn(move || {
            let _lock = lock;
            let result = run_tui_sync(&targets, &root, &cache_path, &audit, force_refresh_all);
            if let Err(err) = &result {
                let error_text = format!("{err:#}");
                let _ = audit.record(
                    "tui.sync.finish",
                    AuditStatus::Failed,
                    Some("tui"),
                    None,
                    Some(&error_text),
                );
            }
            let _ = tx.send(result.map_err(|err| format!("{err:#}")));
        });

        Ok(())
    }
}

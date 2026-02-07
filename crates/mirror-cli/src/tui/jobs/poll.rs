use super::*;

impl TuiApp {
    pub(in crate::tui) fn poll_install_events(&mut self) -> anyhow::Result<()> {
        let Some(rx) = self.install_rx.take() else {
            return Ok(());
        };
        let mut done = false;
        while let Ok(event) = rx.try_recv() {
            match event {
                InstallEvent::Progress(progress) => {
                    let state = self
                        .install_progress
                        .get_or_insert_with(|| InstallProgressState::new(progress.total));
                    state.update(progress);
                }
                InstallEvent::Done(result) => {
                    self.release_install_guard();
                    match result {
                        Ok(report) => {
                            info!("Install completed");
                            let audit_id = self.audit.record(
                                "tui.install",
                                AuditStatus::Ok,
                                Some("tui"),
                                None,
                                None,
                            )?;
                            self.message = format!(
                                "{}\n{}\n{}\nAudit ID: {audit_id}",
                                report.install, report.service, report.path
                            );
                        }
                        Err(err) => {
                            error!(error = %err, "Install failed");
                            let _ = self.audit.record(
                                "tui.install",
                                AuditStatus::Failed,
                                Some("tui"),
                                None,
                                Some(&err),
                            );
                            self.message = format!("Install failed: {err}");
                        }
                    }
                    self.navigate_to(View::Message);
                    done = true;
                }
            }
        }
        if !done {
            self.install_rx = Some(rx);
        }
        Ok(())
    }

    pub(in crate::tui) fn poll_repo_status_events(&mut self) -> anyhow::Result<()> {
        let Some(rx) = self.repo_status_rx.take() else {
            return Ok(());
        };
        let mut done = false;
        while let Ok(result) = rx.try_recv() {
            self.repo_status_refreshing = false;
            match result {
                Ok(statuses) => {
                    info!(count = statuses.len(), "Repo status refreshed");
                    self.update_repo_status_cache(statuses);
                    let refreshed = self
                        .repo_status_last_refresh
                        .map(epoch_to_label)
                        .unwrap_or_else(|| "unknown".to_string());
                    self.repo_overview_message =
                        Some(format!("Repo status refreshed at {refreshed}"));
                }
                Err(err) => {
                    warn!(error = %err, "Repo status refresh failed");
                    self.repo_overview_message = Some(format!("Repo status refresh failed: {err}"));
                }
            }
            done = true;
        }
        if !done {
            self.repo_status_rx = Some(rx);
        }
        Ok(())
    }

    pub(in crate::tui) fn poll_sync_events(&mut self) -> anyhow::Result<()> {
        let Some(rx) = self.sync_rx.take() else {
            return Ok(());
        };
        let mut done = false;
        while let Ok(result) = rx.try_recv() {
            self.sync_running = false;
            match result {
                Ok(summary) => {
                    info!(
                        cloned = summary.cloned,
                        fast_forwarded = summary.fast_forwarded,
                        up_to_date = summary.up_to_date,
                        dirty = summary.dirty,
                        diverged = summary.diverged,
                        failed = summary.failed,
                        missing_archived = summary.missing_archived,
                        missing_removed = summary.missing_removed,
                        missing_skipped = summary.missing_skipped,
                        "Sync completed"
                    );
                    self.message = format!(
                        "Sync completed. cloned={} ff={} up={} dirty={} div={} fail={} missA={} missR={} missS={}",
                        summary.cloned,
                        summary.fast_forwarded,
                        summary.up_to_date,
                        summary.dirty,
                        summary.diverged,
                        summary.failed,
                        summary.missing_archived,
                        summary.missing_removed,
                        summary.missing_skipped
                    );
                }
                Err(err) => {
                    let error_text = format!("{err:#}");
                    error!(error = %error_text, "Sync failed");
                    self.message = format!("Sync failed:\n{error_text}");
                }
            }
            self.navigate_to(View::Message);
            done = true;
        }
        if !done {
            self.sync_rx = Some(rx);
        }
        Ok(())
    }

    pub(in crate::tui) fn poll_update_events(&mut self) -> anyhow::Result<()> {
        let Some(rx) = self.update_rx.take() else {
            return Ok(());
        };
        let mut done = false;
        while let Ok(event) = rx.try_recv() {
            match event {
                UpdateEvent::Progress(message) => {
                    let state = self
                        .update_progress
                        .get_or_insert_with(UpdateProgressState::new);
                    state.messages.push(message);
                }
                UpdateEvent::Checked(result) => {
                    match result {
                        Ok(check) => {
                            info!(
                                current = %check.current,
                                latest = %check.latest,
                                is_newer = check.is_newer,
                                "Update check completed"
                            );
                            if !check.is_newer {
                                self.message = format!("Up to date ({})", check.current);
                                self.message_return_view = self.update_return_view;
                                self.navigate_to(View::Message);
                            } else if check.asset.is_none() {
                                self.message =
                                    "Update available but no asset found for this platform."
                                        .to_string();
                                self.message_return_view = self.update_return_view;
                                self.navigate_to(View::Message);
                            } else {
                                self.message = format!(
                                    "Update available: {} -> {}\nApply update? (y/n)",
                                    check.current, check.latest
                                );
                                self.update_prompt = Some(check);
                                self.navigate_to(View::UpdatePrompt);
                            }
                        }
                        Err(err) => {
                            warn!(error = %err, "Update check failed");
                            self.message = format!("Update check failed: {err}");
                            self.message_return_view = self.update_return_view;
                            self.navigate_to(View::Message);
                        }
                    }
                    done = true;
                }
                UpdateEvent::Done(result) => {
                    match result {
                        Ok(report) => {
                            info!("Update applied");
                            self.message =
                                format!("{}\n{}\n{}", report.install, report.service, report.path);
                            self.restart_requested = true;
                        }
                        Err(err) => {
                            error!(error = %err, "Update apply failed");
                            self.message = format!("Update failed: {err}");
                        }
                    }
                    self.message_return_view = self.update_return_view;
                    self.navigate_to(View::Message);
                    done = true;
                }
            }
        }
        if !done {
            self.update_rx = Some(rx);
        }
        Ok(())
    }
}

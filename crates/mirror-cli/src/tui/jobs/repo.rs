use super::*;

impl TuiApp {
    pub(in crate::tui) fn enter_repo_overview(&mut self) -> anyhow::Result<()> {
        info!("Entered repo overview view");
        self.view = View::RepoOverview;
        self.repo_overview_message = None;
        self.repo_overview_selected = 0;
        self.repo_overview_scroll = 0;
        self.load_repo_status_from_cache();
        if self.repo_status_is_stale() {
            self.start_repo_status_refresh()?;
        }
        Ok(())
    }

    pub(in crate::tui) fn load_repo_status_from_cache(&mut self) {
        if let Ok(cache_path) = default_cache_path()
            && let Ok(cache) = RepoCache::load(&cache_path)
        {
            self.update_repo_status_cache(cache.repo_status);
        }
    }

    pub(in crate::tui) fn update_repo_status_cache(
        &mut self,
        statuses: HashMap<String, RepoLocalStatus>,
    ) {
        self.repo_status_last_refresh = statuses.values().map(|s| s.checked_at).max();
        self.repo_status = statuses;
    }

    pub(in crate::tui) fn current_overview_rows(&self) -> Vec<repo_overview::OverviewRow> {
        let cache_path = default_cache_path().ok();
        let cache = cache_path
            .as_ref()
            .and_then(|path| RepoCache::load(path).ok())
            .unwrap_or_default();
        let root = self.config.root.as_deref();
        if cache.repos.is_empty() {
            vec![repo_overview::OverviewRow {
                id: "empty".to_string(),
                depth: 0,
                name: "No repos in cache yet.".to_string(),
                branch: None,
                pulled: None,
                ahead_behind: None,
                touched: None,
                is_leaf: true,
            }]
        } else {
            let tree = repo_overview::build_repo_tree(cache.repos.iter(), root);
            repo_overview::render_repo_tree_rows(&tree, &cache, &self.repo_status)
        }
    }

    pub(in crate::tui) fn visible_overview_rows(
        &self,
        rows: &[repo_overview::OverviewRow],
    ) -> Vec<repo_overview::OverviewRow> {
        let mut visible = Vec::new();
        let mut stack: Vec<(usize, bool)> = Vec::new();
        for row in rows {
            while let Some((depth, _)) = stack.last() {
                if row.depth <= *depth {
                    stack.pop();
                } else {
                    break;
                }
            }
            let hidden = stack.iter().any(|(_, collapsed)| *collapsed);
            if !hidden {
                visible.push(row.clone());
            }
            if !row.is_leaf {
                let collapsed = self.repo_overview_collapsed.contains(&row.id);
                if !hidden {
                    stack.push((row.depth, collapsed));
                } else if collapsed {
                    stack.push((row.depth, true));
                }
            }
        }
        visible
    }

    pub(in crate::tui) fn repo_status_is_stale(&self) -> bool {
        match self.repo_status_last_refresh {
            Some(timestamp) => {
                current_epoch_seconds().saturating_sub(timestamp) > REPO_STATUS_TTL_SECS
            }
            None => true,
        }
    }

    pub(in crate::tui) fn start_repo_status_refresh(&mut self) -> anyhow::Result<()> {
        if self.repo_status_refreshing {
            debug!("Repo status refresh already running");
            return Ok(());
        }
        let cache_path = default_cache_path()?;
        info!(path = %cache_path.display(), "Starting repo status refresh");
        let (tx, rx) = mpsc::channel::<Result<HashMap<String, RepoLocalStatus>, String>>();
        self.repo_status_rx = Some(rx);
        self.repo_status_refreshing = true;
        self.repo_overview_message = Some("Refreshing repo status...".to_string());
        thread::spawn(move || {
            let result =
                repo_overview::refresh_repo_status(&cache_path).map_err(|err| err.to_string());
            let _ = tx.send(result);
        });
        Ok(())
    }
}

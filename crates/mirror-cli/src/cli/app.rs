use super::*;
pub async fn run() -> anyhow::Result<()> {
    let log_buffer = logging::LogBuffer::new(200);
    let filter = EnvFilter::from_default_env();
    tracing_subscriber::registry()
        .with(filter)
        .with(tracing_subscriber::fmt::layer())
        .with(logging::LogLayer::new(log_buffer.clone()))
        .init();

    let audit = AuditLogger::new()?;
    let _ = audit.record("app.start", AuditStatus::Ok, None, None, None)?;

    let args: Vec<String> = std::env::args().collect();
    if args.len() == 1 {
        info!(mode = "tui", "Launching default TUI");
        if install::is_installed().unwrap_or(false) {
            let mut cmd = Cli::command();
            cmd.print_help()?;
            println!();
            return Ok(());
        }
        return tui::run_tui(&audit, log_buffer.clone(), tui::StartView::Install);
    }

    let cli = Cli::parse();
    let is_interactive = stdin_is_tty() && stdout_is_tty();
    info!(command = command_label(&cli.command), "Running command");

    if cli.check_updates
        && !matches!(cli.command, Commands::Update(_))
        && let Err(err) = run_update_check(&audit)
    {
        warn!(error = %err, "Update check failed");
        let _ = audit.record(
            "update.check",
            AuditStatus::Failed,
            Some("update"),
            None,
            Some(&err.to_string()),
        );
        eprintln!("Update check failed: {err}");
    }

    if should_run_cli_update_check(&cli.command, cli.check_updates) {
        let cache_path = default_cache_path()?;
        let now = current_epoch_seconds();
        let should_check = match RepoCache::load(&cache_path) {
            Ok(cache) => update_check_due(&cache, now, 86_400),
            Err(_) => true,
        };
        if should_check {
            match update::check_and_maybe_apply(update::AutoUpdateOptions {
                cache_path: &cache_path,
                interval_secs: 86_400,
                auto_apply: true,
                audit: &audit,
                force: true,
                interactive: is_interactive,
                source: "cli",
                override_repo: None,
            }) {
                Ok(applied) => {
                    if applied {
                        update::restart_current_process().context("restart after update apply")?;
                        return Ok(());
                    }
                }
                Err(err) => {
                    if update::is_permission_error(&err)
                        && maybe_escalate_and_reexec("update").context("escalate update")?
                    {
                        return Ok(());
                    }
                    let _ = audit.record(
                        "update.check",
                        AuditStatus::Skipped,
                        Some("update"),
                        Some(serde_json::json!({"reason": "error", "source": "cli"})),
                        Some(&err.to_string()),
                    );
                }
            }
        }
    }

    if should_run_cli_token_check(&cli.command) {
        let cache_path = default_cache_path()?;
        let config_path = default_config_path()?;
        let should_check = match RepoCache::load(&cache_path) {
            Ok(cache) => {
                cache.token_last_source.as_deref() != Some("daemon")
                    && cache.token_last_check.is_none()
            }
            Err(_) => true,
        };
        if should_check {
            let _ = run_token_validity_checks(&config_path, &cache_path, &audit, "cli", true).await;
        }
    }

    let result = match cli.command {
        Commands::Config(args) => handle_config(args, &audit),
        Commands::Target(args) => handle_target(args, &audit),
        Commands::Token(args) => handle_token(args, &audit).await,
        Commands::Sync(args) => handle_sync(args, &audit).await,
        Commands::Daemon(args) => handle_daemon(args, &audit).await,
        Commands::Service(args) => handle_service(args, &audit),
        Commands::Health(args) => handle_health(args, &audit).await,
        Commands::Webhook(args) => handle_webhook(args, &audit).await,
        Commands::Cache(args) => handle_cache(args, &audit),
        Commands::Tui(args) => {
            let start_view = if args.install {
                tui::StartView::Install
            } else if args.dashboard {
                tui::StartView::Dashboard
            } else {
                tui::StartView::Main
            };
            tui::run_tui(&audit, log_buffer.clone(), start_view)
        }
        Commands::Install(args) => handle_install(args, &audit, log_buffer.clone()),
        Commands::Task(args) => handle_task(args, &audit),
        Commands::Update(args) => handle_update(args, &audit),
    };

    if let Err(err) = &result {
        let _ = audit.record(
            "app.error",
            AuditStatus::Failed,
            None,
            None,
            Some(&err.to_string()),
        );
    }

    result
}
fn command_label(command: &Commands) -> &'static str {
    match command {
        Commands::Config(_) => "config",
        Commands::Target(_) => "target",
        Commands::Token(_) => "token",
        Commands::Sync(_) => "sync",
        Commands::Daemon(_) => "daemon",
        Commands::Service(_) => "service",
        Commands::Health(_) => "health",
        Commands::Webhook(_) => "webhook",
        Commands::Cache(_) => "cache",
        Commands::Tui(_) => "tui",
        Commands::Install(_) => "install",
        Commands::Task(_) => "task",
        Commands::Update(_) => "update",
    }
}

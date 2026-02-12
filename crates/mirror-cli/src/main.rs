mod cli;
mod i18n;
mod install;
mod logging;
mod repo_overview;
mod token_check;
mod tui;
mod update;

fn main() -> anyhow::Result<()> {
    // Check if we're running TUI - if so, run it synchronously to avoid runtime nesting
    if cli::should_run_tui_sync() {
        cli::run_sync()
    } else {
        let runtime = tokio::runtime::Builder::new_current_thread()
            .enable_time()
            .build()
            .map_err(|err| anyhow::anyhow!("create cli runtime: {err}"))?;
        runtime.block_on(cli::run())
    }
}

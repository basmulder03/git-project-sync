mod cli;
mod install;
mod logging;
mod repo_overview;
mod token_check;
mod tui;
mod update;

fn main() -> anyhow::Result<()> {
    let runtime = tokio::runtime::Builder::new_current_thread()
        .enable_time()
        .build()
        .map_err(|err| anyhow::anyhow!("create cli runtime: {err}"))?;
    runtime.block_on(cli::run())
}

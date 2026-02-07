mod cli;
mod install;
mod logging;
mod repo_overview;
mod token_check;
mod tui;
mod update;

fn main() -> anyhow::Result<()> {
    cli::run()
}

use super::*;
pub(in crate::cli) fn service_label() -> &'static str {
    if cfg!(target_os = "windows") {
        "Scheduled task"
    } else {
        "Service"
    }
}

pub(in crate::cli) fn render_progress_bar(step: usize, total: usize, width: usize) -> String {
    if total == 0 || width == 0 {
        return "[]".to_string();
    }
    let filled = ((step as f32 / total as f32) * width as f32).round() as usize;
    let filled = filled.min(width);
    let empty = width.saturating_sub(filled);
    format!("[{}{}]", "#".repeat(filled), "-".repeat(empty))
}

pub(in crate::cli) fn render_sync_progress(
    target_label: &str,
    last_len: &Cell<usize>,
    progress: &SyncProgress,
) {
    let total = progress.total_repos;
    let processed = progress.processed_repos.min(total);
    let bar = render_progress_bar(processed, total, 20);
    let repo = progress
        .repo_name
        .as_deref()
        .or(progress.repo_id.as_deref())
        .unwrap_or("-");
    let line = format!(
        "{} {}/{} {} action={} repo={}",
        target_label,
        processed,
        total,
        bar,
        progress.action.as_str(),
        repo
    );
    let prev_len = last_len.get();
    if line.len() < prev_len {
        print!("\r{line}{}", " ".repeat(prev_len - line.len()));
    } else {
        print!("\r{line}");
    }
    let _ = io::stdout().flush();
    last_len.set(line.len());
    if !progress.in_progress || matches!(progress.action, SyncAction::Done) {
        println!();
        last_len.set(0);
    }
}

pub(in crate::cli) fn print_sync_status(
    cache_path: &Path,
    targets: &[TargetConfig],
) -> anyhow::Result<()> {
    let cache = mirror_core::cache::RepoCache::load(cache_path).unwrap_or_default();
    for target in targets {
        let label = format!(
            "{} | {}",
            target.provider.as_prefix(),
            target.scope.segments().join("/")
        );
        let status = cache.target_sync_status.get(&target.id);
        let state = status
            .map(|s| if s.in_progress { "running" } else { "idle" })
            .unwrap_or("idle");
        let action = status
            .and_then(|s| s.last_action.as_deref())
            .unwrap_or("unknown");
        let repo = status.and_then(|s| s.last_repo.as_deref()).unwrap_or("-");
        let updated = status
            .map(|s| epoch_to_label(s.last_updated))
            .unwrap_or_else(|| "unknown".to_string());
        let empty_summary = mirror_core::cache::SyncSummarySnapshot::default();
        let summary = status.map(|s| &s.summary).unwrap_or(&empty_summary);
        let total = status.map(|s| s.total_repos).unwrap_or(0);
        let processed = status.map(|s| s.processed_repos).unwrap_or(0);

        println!("{label} | {state} | {action} | {repo} | {updated}");
        println!(
            "progress: {}/{} {}",
            processed,
            total,
            render_progress_bar(processed.min(total), total, 20)
        );
        println!(
            "counts: cl={} ff={} up={} dirty={} div={} fail={} missA={} missR={} missS={}",
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
        println!();
    }
    Ok(())
}

pub(in crate::cli) fn epoch_to_label(epoch: u64) -> String {
    let ts = time::OffsetDateTime::from_unix_timestamp(epoch as i64)
        .unwrap_or_else(|_| time::OffsetDateTime::now_utc());
    ts.format(&time::format_description::parse("[year]-[month]-[day] [hour]:[minute]").unwrap())
        .unwrap_or_else(|_| "unknown".to_string())
}

pub(in crate::cli) fn format_delayed_start(delay: Option<u64>) -> String {
    match delay.filter(|value| *value > 0) {
        Some(value) => format!("{value}s"),
        None => "none".to_string(),
    }
}

use super::*;

pub(in crate::tui) enum InstallEvent {
    Progress(crate::install::InstallProgress),
    Done(Result<crate::install::InstallReport, String>),
}

pub(in crate::tui) enum UpdateEvent {
    Progress(String),
    Checked(Result<update::UpdateCheck, String>),
    Done(Result<crate::install::InstallReport, String>),
}

pub(in crate::tui) struct InstallProgressState {
    pub(in crate::tui) current: usize,
    pub(in crate::tui) total: usize,
    pub(in crate::tui) messages: Vec<String>,
}

impl InstallProgressState {
    pub(in crate::tui) fn new(total: usize) -> Self {
        Self {
            current: 0,
            total,
            messages: Vec::new(),
        }
    }

    pub(in crate::tui) fn update(&mut self, progress: crate::install::InstallProgress) {
        self.current = progress.step;
        self.total = progress.total.max(1);
        self.messages.push(progress.message);
    }
}

pub(in crate::tui) struct UpdateProgressState {
    pub(in crate::tui) messages: Vec<String>,
}

impl UpdateProgressState {
    pub(in crate::tui) fn new() -> Self {
        Self {
            messages: vec!["Checking for updates...".to_string()],
        }
    }
}

pub(in crate::tui) fn progress_bar(step: usize, total: usize, width: usize) -> String {
    if total == 0 || width == 0 {
        return "[]".to_string();
    }
    let filled = ((step as f32 / total as f32) * width as f32).round() as usize;
    let filled = filled.min(width);
    let empty = width.saturating_sub(filled);
    format!("[{}{}]", "#".repeat(filled), "-".repeat(empty))
}

pub(in crate::tui) fn dashboard_footer_text() -> &'static str {
    "t: toggle targets | s: sync status | r: sync now | f: force refresh all | u: check updates | Esc: back | PgUp/PgDn/Home/End: scroll"
}

pub(in crate::tui) fn read_audit_lines(
    path: &std::path::Path,
    filter: AuditFilter,
) -> anyhow::Result<Vec<String>> {
    if !path.exists() {
        return Ok(vec!["No audit log found for today.".to_string()]);
    }
    let contents = std::fs::read_to_string(path)?;
    let mut lines = Vec::new();
    for line in contents.lines().rev().take(100) {
        if filter == AuditFilter::Failures && !line.contains("\"status\":\"failed\"") {
            continue;
        }
        lines.push(line.to_string());
    }
    Ok(lines)
}

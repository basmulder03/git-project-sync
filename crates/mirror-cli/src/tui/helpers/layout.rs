use super::*;

pub(in crate::tui) fn provider_kind(index: usize) -> ProviderKind {
    match index {
        1 => ProviderKind::GitHub,
        2 => ProviderKind::GitLab,
        _ => ProviderKind::AzureDevOps,
    }
}

pub(in crate::tui) fn provider_label(index: usize) -> &'static str {
    match index {
        1 => "github",
        2 => "gitlab",
        _ => "azure-devops",
    }
}

pub(in crate::tui) fn provider_selector_line(index: usize) -> String {
    let labels = ["azure-devops", "github", "gitlab"];
    let mut parts = Vec::with_capacity(labels.len());
    for (idx, label) in labels.iter().enumerate() {
        if idx == index {
            parts.push(format!("[{label}]"));
        } else {
            parts.push(label.to_string());
        }
    }
    format!("Provider: {}", parts.join(" "))
}

pub(in crate::tui) fn provider_scope_hint(provider: ProviderKind) -> &'static str {
    match provider {
        ProviderKind::AzureDevOps => "Scope uses space-separated segments (org project)",
        ProviderKind::GitHub => "Scope uses a single segment (org or user)",
        ProviderKind::GitLab => "Scope uses space-separated group/subgroup segments",
    }
}

pub(in crate::tui) fn provider_scope_hint_with_host(provider: ProviderKind) -> &'static str {
    match provider {
        ProviderKind::AzureDevOps => {
            "Scope uses space-separated segments (org project). Host optional."
        }
        ProviderKind::GitHub => "Scope uses a single segment (org or user). Host optional.",
        ProviderKind::GitLab => {
            "Scope uses space-separated group/subgroup segments. Host optional."
        }
    }
}

pub(in crate::tui) fn slice_with_scroll(
    lines: &[String],
    scroll: usize,
    height: usize,
) -> Vec<String> {
    if lines.is_empty() || height == 0 {
        return Vec::new();
    }
    let start = scroll.min(lines.len());
    let end = (start + height).min(lines.len());
    lines[start..end].to_vec()
}

pub(in crate::tui) fn max_scroll_for_lines(content_len: usize, area_height: u16) -> usize {
    let body_height = area_height.saturating_sub(2) as usize;
    content_len.saturating_sub(body_height)
}

pub(in crate::tui) fn clamp_index(index: usize, len: usize) -> usize {
    if len == 0 { 0 } else { index.min(len - 1) }
}

pub(in crate::tui) fn adjust_scroll(
    selected: usize,
    scroll: usize,
    height: usize,
    len: usize,
) -> usize {
    if len == 0 || height == 0 {
        return 0;
    }
    if selected < scroll {
        return selected;
    }
    let last_visible = scroll.saturating_add(height).saturating_sub(1);
    if selected > last_visible {
        let new_scroll = selected.saturating_sub(height - 1);
        return new_scroll.min(len.saturating_sub(1));
    }
    scroll
}

pub(in crate::tui) fn name_column_width(total_width: usize, compact: bool) -> usize {
    let fixed = if compact {
        2 + 12 + 2 + 10 // separators + columns
    } else {
        2 + 12 + 2 + 16 + 2 + 10 + 2 + 16
    };
    let available = total_width.saturating_sub(fixed);
    available.max(20)
}

pub(in crate::tui) fn format_overview_header(name_width: usize, compact: bool) -> String {
    if compact {
        format!(
            "{:<name_width$} | {:<12} | {:<10}",
            "name",
            "branch",
            "ahead/behind",
            name_width = name_width
        )
    } else {
        format!(
            "{:<name_width$} | {:<12} | {:<16} | {:<10} | {:<16}",
            "name",
            "branch",
            "pulled",
            "ahead/behind",
            "touched",
            name_width = name_width
        )
    }
}

pub(in crate::tui) fn format_overview_row(
    row: &repo_overview::OverviewRow,
    name_width: usize,
    compact: bool,
) -> String {
    let name = truncate_with_ellipsis(&row.name, name_width);
    let branch = row.branch.as_deref().unwrap_or("-");
    let ahead = row.ahead_behind.as_deref().unwrap_or("-");
    if compact {
        format!(
            "{:<name_width$} | {:<12} | {:<10}",
            name,
            branch,
            ahead,
            name_width = name_width
        )
    } else {
        let pulled = row.pulled.as_deref().unwrap_or("-");
        let touched = row.touched.as_deref().unwrap_or("-");
        format!(
            "{:<name_width$} | {:<12} | {:<16} | {:<10} | {:<16}",
            name,
            branch,
            pulled,
            ahead,
            touched,
            name_width = name_width
        )
    }
}

pub(in crate::tui) fn truncate_with_ellipsis(value: &str, max: usize) -> String {
    if max == 0 {
        return String::new();
    }
    if value.len() <= max {
        return value.to_string();
    }
    if max <= 1 {
        return "…".to_string();
    }
    let mut truncated = value.chars().take(max - 1).collect::<String>();
    truncated.push('…');
    truncated
}

pub(in crate::tui) fn optional_text(value: &str) -> Option<String> {
    let trimmed = value.trim();
    if trimmed.is_empty() {
        None
    } else {
        Some(trimmed.to_string())
    }
}

pub(in crate::tui) fn split_labels(value: &str) -> Vec<String> {
    value
        .split(',')
        .map(|label| label.trim())
        .filter(|label| !label.is_empty())
        .map(|label| label.to_string())
        .collect()
}

use mirror_core::cache::{RepoCache, RepoCacheEntry};
use mirror_core::repo_status::{RepoLocalStatus, compute_repo_status};
use std::collections::{BTreeMap, HashMap};
use std::path::{Component, Path};

#[derive(Default)]
pub struct RepoTreeNode {
    pub children: BTreeMap<String, RepoTreeNode>,
    pub leaf: Option<RepoTreeLeaf>,
}

pub struct RepoTreeLeaf {
    pub id: String,
}

#[derive(Clone)]
pub struct OverviewRow {
    pub id: String,
    pub depth: usize,
    pub name: String,
    pub branch: Option<String>,
    pub pulled: Option<String>,
    pub ahead_behind: Option<String>,
    pub touched: Option<String>,
    pub is_leaf: bool,
}

pub fn refresh_repo_status(cache_path: &Path) -> anyhow::Result<HashMap<String, RepoLocalStatus>> {
    let mut cache = RepoCache::load(cache_path).unwrap_or_default();
    let mut statuses = HashMap::new();
    for (repo_id, entry) in &cache.repos {
        let status = compute_repo_status(Path::new(&entry.path))?;
        statuses.insert(repo_id.clone(), status);
    }
    cache.repo_status = statuses.clone();
    cache.save(cache_path)?;
    Ok(statuses)
}

pub fn build_repo_tree<'a, I>(entries: I, root: Option<&Path>) -> RepoTreeNode
where
    I: IntoIterator<Item = (&'a String, &'a RepoCacheEntry)>,
{
    let mut tree = RepoTreeNode::default();
    for (repo_id, entry) in entries {
        let path = Path::new(&entry.path);
        let rel = root
            .and_then(|root| path.strip_prefix(root).ok())
            .unwrap_or(path);
        let components: Vec<String> = rel
            .components()
            .filter_map(|component| match component {
                Component::Normal(os) => Some(os.to_string_lossy().to_string()),
                _ => None,
            })
            .collect();
        if components.is_empty() {
            continue;
        }
        let mut node = &mut tree;
        for (idx, component) in components.iter().enumerate() {
            node = node
                .children
                .entry(component.clone())
                .or_insert_with(RepoTreeNode::default);
            if idx == components.len() - 1 {
                node.leaf = Some(RepoTreeLeaf {
                    id: repo_id.clone(),
                });
            }
        }
    }
    tree
}

pub fn render_repo_tree_lines(
    node: &RepoTreeNode,
    cache: &RepoCache,
    status: &HashMap<String, RepoLocalStatus>,
) -> Vec<String> {
    let mut lines = Vec::new();
    render_repo_tree(node, 0, &mut lines, cache, status);
    lines
}

pub fn render_repo_tree_rows(
    node: &RepoTreeNode,
    cache: &RepoCache,
    status: &HashMap<String, RepoLocalStatus>,
) -> Vec<OverviewRow> {
    let mut rows = Vec::new();
    render_repo_tree_with_rows(node, 0, "", &mut rows, cache, status);
    rows
}

fn render_repo_tree(
    node: &RepoTreeNode,
    depth: usize,
    lines: &mut Vec<String>,
    cache: &RepoCache,
    status: &HashMap<String, RepoLocalStatus>,
) {
    for (name, child) in &node.children {
        let indent = "  ".repeat(depth);
        if let Some(leaf) = &child.leaf {
            let status = status.get(&leaf.id);
            let branch = status
                .and_then(|value| value.head_branch.as_deref())
                .unwrap_or("unknown");
            let pulled = cache
                .last_sync
                .get(&leaf.id)
                .and_then(|value| parse_epoch_string(value))
                .map(format_epoch_label)
                .unwrap_or_else(|| "never".to_string());
            let ahead_behind = status
                .and_then(|value| value.ahead.zip(value.behind))
                .map(|(ahead, behind)| format!("+{ahead}/-{behind}"))
                .unwrap_or_else(|| "unknown".to_string());
            let touched = status
                .and_then(|value| value.head_commit_time)
                .map(format_epoch_label)
                .unwrap_or_else(|| "unknown".to_string());
            let line = format!(
                "{indent}{name}  branch={branch}  pulled={pulled}  ahead/behind={ahead_behind}  touched={touched}"
            );
            lines.push(line);
        } else {
            lines.push(format!("{indent}{name}/"));
            render_repo_tree(child, depth + 1, lines, cache, status);
        }
    }
}

fn render_repo_tree_with_rows(
    node: &RepoTreeNode,
    depth: usize,
    prefix: &str,
    rows: &mut Vec<OverviewRow>,
    cache: &RepoCache,
    status: &HashMap<String, RepoLocalStatus>,
) {
    for (name, child) in &node.children {
        let indent = "  ".repeat(depth);
        let id = if prefix.is_empty() {
            name.to_string()
        } else {
            format!("{prefix}/{name}")
        };
        if let Some(leaf) = &child.leaf {
            let status = status.get(&leaf.id);
            let branch = status
                .and_then(|value| value.head_branch.as_deref())
                .unwrap_or("unknown");
            let pulled = cache
                .last_sync
                .get(&leaf.id)
                .and_then(|value| parse_epoch_string(value))
                .map(format_epoch_label)
                .unwrap_or_else(|| "never".to_string());
            let ahead_behind = status
                .and_then(|value| value.ahead.zip(value.behind))
                .map(|(ahead, behind)| format!("+{ahead}/-{behind}"))
                .unwrap_or_else(|| "unknown".to_string());
            let touched = status
                .and_then(|value| value.head_commit_time)
                .map(format_epoch_label)
                .unwrap_or_else(|| "unknown".to_string());
            rows.push(OverviewRow {
                id,
                depth,
                name: format!("{indent}{name}"),
                branch: Some(branch.to_string()),
                pulled: Some(pulled),
                ahead_behind: Some(ahead_behind),
                touched: Some(touched),
                is_leaf: true,
            });
        } else {
            rows.push(OverviewRow {
                id: id.clone(),
                depth,
                name: format!("{indent}{name}/"),
                branch: None,
                pulled: None,
                ahead_behind: None,
                touched: None,
                is_leaf: false,
            });
            render_repo_tree_with_rows(child, depth + 1, &id, rows, cache, status);
        }
    }
}

pub fn parse_epoch_string(value: &str) -> Option<u64> {
    value.parse::<u64>().ok()
}

pub fn format_epoch_label(epoch: u64) -> String {
    let ts = time::OffsetDateTime::from_unix_timestamp(epoch as i64)
        .unwrap_or_else(|_| time::OffsetDateTime::now_utc());
    ts.format(&time::format_description::parse("[year]-[month]-[day] [hour]:[minute]").unwrap())
        .unwrap_or_else(|_| "unknown".to_string())
}

#[cfg(test)]
mod tests {
    use super::*;
    use mirror_core::model::{ProviderKind, ProviderScope};

    #[test]
    fn parse_epoch_string_handles_invalid() {
        assert_eq!(parse_epoch_string("123"), Some(123));
        assert_eq!(parse_epoch_string("nope"), None);
    }

    #[test]
    fn repo_tree_renders_hierarchy() {
        let scope = ProviderScope::new(vec!["org".into(), "proj".into()]).unwrap();
        let mut cache = RepoCache::new();
        cache.repos.insert(
            "repo-1".into(),
            RepoCacheEntry {
                name: "repo-a".into(),
                provider: ProviderKind::AzureDevOps,
                scope: scope.clone(),
                path: "/tmp/root/azure-devops/org/proj/repo-a".into(),
            },
        );
        cache.repos.insert(
            "repo-2".into(),
            RepoCacheEntry {
                name: "repo-b".into(),
                provider: ProviderKind::GitHub,
                scope,
                path: "/tmp/root/github/org/repo-b".into(),
            },
        );
        let tree = build_repo_tree(cache.repos.iter(), Some(Path::new("/tmp/root")));
        let lines = render_repo_tree_lines(&tree, &cache, &HashMap::new());
        assert!(lines.iter().any(|line| line.contains("azure-devops/")));
        assert!(lines.iter().any(|line| line.contains("repo-a")));
        assert!(lines.iter().any(|line| line.contains("github/")));
        assert!(lines.iter().any(|line| line.contains("repo-b")));
    }
}

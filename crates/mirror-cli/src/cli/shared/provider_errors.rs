use super::*;
pub(in crate::cli) fn map_azdo_error(
    target: &ProviderTarget,
    err: anyhow::Error,
) -> anyhow::Result<SyncSummary> {
    if target.provider == ProviderKind::AzureDevOps
        && let Some(reqwest_err) = err.downcast_ref::<reqwest::Error>()
        && let Some(status) = reqwest_err.status()
        && let Some(message) = azdo_message_for_status(target, status)
    {
        return Err(anyhow::anyhow!(message));
    }
    Err(err)
}

pub(in crate::cli) fn map_provider_error(
    target: &ProviderTarget,
    err: anyhow::Error,
) -> anyhow::Result<()> {
    if let Some(reqwest_err) = err.downcast_ref::<reqwest::Error>()
        && let Some(status) = reqwest_err.status()
    {
        let scope = target.scope.segments().join("/");
        let message = match target.provider {
            ProviderKind::AzureDevOps => azdo_status_message(&scope, status),
            ProviderKind::GitHub => github_status_message(&scope, status),
            ProviderKind::GitLab => gitlab_status_message(&scope, status),
        };
        if let Some(message) = message {
            return Err(anyhow::anyhow!(message));
        }
    }
    Err(err)
}

pub(in crate::cli) fn azdo_message_for_status(
    target: &ProviderTarget,
    status: StatusCode,
) -> Option<String> {
    let scope = target.scope.segments().join("/");
    azdo_status_message(&scope, status)
}

pub(in crate::cli) fn azdo_status_message(scope: &str, status: StatusCode) -> Option<String> {
    match status {
        StatusCode::UNAUTHORIZED | StatusCode::FORBIDDEN => Some(format!(
            "Azure DevOps authentication failed for scope {scope} (HTTP {status}). Check your PAT.",
        )),
        StatusCode::NOT_FOUND => Some(format!(
            "Azure DevOps scope not found: {scope} (HTTP {status}). Check org/project.",
        )),
        _ => None,
    }
}

pub(in crate::cli) fn github_status_message(scope: &str, status: StatusCode) -> Option<String> {
    match status {
        StatusCode::UNAUTHORIZED | StatusCode::FORBIDDEN => Some(format!(
            "GitHub authentication failed for scope {scope} (HTTP {status}). Check your token and org access.",
        )),
        StatusCode::NOT_FOUND => Some(format!(
            "GitHub scope not found: {scope} (HTTP {status}). Check org/user.",
        )),
        _ => None,
    }
}

pub(in crate::cli) fn gitlab_status_message(scope: &str, status: StatusCode) -> Option<String> {
    match status {
        StatusCode::UNAUTHORIZED | StatusCode::FORBIDDEN => Some(format!(
            "GitLab authentication failed for scope {scope} (HTTP {status}). Check your token and group access.",
        )),
        StatusCode::NOT_FOUND => Some(format!(
            "GitLab scope not found: {scope} (HTTP {status}). Check group path.",
        )),
        _ => None,
    }
}

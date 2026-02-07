use super::*;

#[derive(Deserialize)]
struct ReleaseResponse {
    tag_name: String,
    html_url: Option<String>,
    assets: Vec<ReleaseAssetResponse>,
}

#[derive(Deserialize)]
struct ReleaseAssetResponse {
    name: String,
    browser_download_url: String,
}

pub fn resolve_repo(override_repo: Option<&str>) -> String {
    if let Some(value) = override_repo {
        return value.to_string();
    }
    if let Ok(value) = std::env::var(UPDATE_REPO_ENV) {
        let trimmed = value.trim();
        if !trimmed.is_empty() {
            return trimmed.to_string();
        }
    }
    DEFAULT_UPDATE_REPO.to_string()
}

pub fn expected_asset_name() -> &'static str {
    if cfg!(target_os = "windows") {
        "mirror-cli-windows-x86_64.exe"
    } else if cfg!(target_os = "macos") {
        "mirror-cli-macos-x86_64"
    } else {
        "mirror-cli-linux-x86_64"
    }
}

pub fn check_for_update(override_repo: Option<&str>) -> anyhow::Result<UpdateCheck> {
    let repo = resolve_repo(override_repo);
    let url = format!("https://api.github.com/repos/{repo}/releases/latest");
    let client = Client::new();
    let response = client
        .get(url)
        .header("User-Agent", "git-project-sync")
        .header("Accept", "application/vnd.github+json")
        .send()
        .context("fetch latest release")?
        .error_for_status()
        .context("latest release request failed")?;
    let release: ReleaseResponse = response.json().context("parse release response")?;

    let current = Version::parse(env!("CARGO_PKG_VERSION")).context("parse current version")?;
    let latest = parse_version(&release.tag_name)?;
    let is_newer = latest > current;

    let asset = if is_newer {
        let target = expected_asset_name();
        release
            .assets
            .iter()
            .find(|asset| asset.name == target)
            .map(|asset| ReleaseAsset {
                name: asset.name.clone(),
                url: asset.browser_download_url.clone(),
            })
    } else {
        None
    };

    Ok(UpdateCheck {
        current,
        latest,
        release_url: release.html_url,
        is_newer,
        asset,
    })
}

pub(in crate::update) fn parse_version(raw: &str) -> anyhow::Result<Version> {
    let trimmed = raw.trim();
    let normalized = trimmed.strip_prefix('v').unwrap_or(trimmed);
    Version::parse(normalized).context("parse release version")
}

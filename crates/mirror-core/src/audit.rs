use anyhow::Context;
use directories::ProjectDirs;
use serde::Serialize;
use serde_json::Value;
use std::fs::{self, OpenOptions};
use std::io::Write;
use std::path::{Path, PathBuf};
use time::format_description::well_known::Rfc3339;
use time::OffsetDateTime;
use uuid::Uuid;

const MAX_BYTES: u64 = 10 * 1024 * 1024;

#[derive(Clone)]
pub struct AuditLogger {
    session_id: String,
    base_dir: PathBuf,
    max_bytes: u64,
}

impl AuditLogger {
    pub fn new() -> anyhow::Result<Self> {
        let project = ProjectDirs::from("com", "git-project-sync", "git-project-sync")
            .context("resolve project dirs")?;
        let base_dir = project.data_local_dir().join("audit");
        fs::create_dir_all(&base_dir).context("create audit dir")?;
        Ok(Self {
            session_id: Uuid::new_v4().to_string(),
            base_dir,
            max_bytes: MAX_BYTES,
        })
    }

    pub fn new_with_dir(base_dir: PathBuf, max_bytes: u64) -> anyhow::Result<Self> {
        fs::create_dir_all(&base_dir).context("create audit dir")?;
        Ok(Self {
            session_id: Uuid::new_v4().to_string(),
            base_dir,
            max_bytes,
        })
    }

    pub fn base_dir(&self) -> &Path {
        &self.base_dir
    }

    pub fn record(
        &self,
        event: &str,
        status: AuditStatus,
        command: Option<&str>,
        details: Option<Value>,
        error: Option<&str>,
    ) -> anyhow::Result<String> {
        let ts = OffsetDateTime::now_utc()
            .format(&Rfc3339)
            .context("format timestamp")?;
        let audit_id = Uuid::new_v4().to_string();
        let entry = AuditEvent {
            ts,
            level: status.level(),
            event: event.to_string(),
            audit_id: audit_id.clone(),
            session_id: self.session_id.clone(),
            status: status.as_str(),
            command: command.map(|value| value.to_string()),
            provider: None,
            scope: None,
            repo_id: None,
            path: None,
            error: error.map(|value| value.to_string()),
            details,
        };
        self.write_entry(&entry)?;
        Ok(audit_id)
    }

    pub fn record_with_context(
        &self,
        event: &str,
        status: AuditStatus,
        command: Option<&str>,
        context: AuditContext,
        details: Option<Value>,
        error: Option<&str>,
    ) -> anyhow::Result<String> {
        let ts = OffsetDateTime::now_utc()
            .format(&Rfc3339)
            .context("format timestamp")?;
        let audit_id = Uuid::new_v4().to_string();
        let entry = AuditEvent {
            ts,
            level: status.level(),
            event: event.to_string(),
            audit_id: audit_id.clone(),
            session_id: self.session_id.clone(),
            status: status.as_str(),
            command: command.map(|value| value.to_string()),
            provider: context.provider,
            scope: context.scope,
            repo_id: context.repo_id,
            path: context.path,
            error: error.map(|value| value.to_string()),
            details,
        };
        self.write_entry(&entry)?;
        Ok(audit_id)
    }

    fn write_entry(&self, entry: &AuditEvent) -> anyhow::Result<()> {
        let date = OffsetDateTime::now_utc()
            .format(&time::format_description::parse("[year][month][day]")?)
            .context("format date")?;
        let path = next_audit_path(&self.base_dir, &date, self.max_bytes)?;
        let line = serde_json::to_string(entry).context("serialize audit entry")?;
        let mut file = OpenOptions::new()
            .create(true)
            .append(true)
            .open(&path)
            .with_context(|| format!("open audit log {}", path.display()))?;
        writeln!(file, "{line}").context("write audit entry")?;
        Ok(())
    }
}

#[derive(Debug, Clone, Copy)]
pub enum AuditStatus {
    Ok,
    Failed,
    Skipped,
}

impl AuditStatus {
    fn as_str(&self) -> &'static str {
        match self {
            AuditStatus::Ok => "ok",
            AuditStatus::Failed => "failed",
            AuditStatus::Skipped => "skipped",
        }
    }

    fn level(&self) -> &'static str {
        match self {
            AuditStatus::Ok => "INFO",
            AuditStatus::Failed => "ERROR",
            AuditStatus::Skipped => "WARN",
        }
    }
}

#[derive(Debug, Clone)]
pub struct AuditContext {
    pub provider: Option<String>,
    pub scope: Option<String>,
    pub repo_id: Option<String>,
    pub path: Option<String>,
}

impl AuditContext {
    pub fn empty() -> Self {
        Self {
            provider: None,
            scope: None,
            repo_id: None,
            path: None,
        }
    }
}

#[derive(Serialize)]
struct AuditEvent {
    ts: String,
    level: &'static str,
    event: String,
    audit_id: String,
    session_id: String,
    status: &'static str,
    command: Option<String>,
    provider: Option<String>,
    scope: Option<String>,
    repo_id: Option<String>,
    path: Option<String>,
    error: Option<String>,
    details: Option<Value>,
}

fn next_audit_path(base_dir: &Path, date: &str, max_bytes: u64) -> anyhow::Result<PathBuf> {
    let mut suffix = 0;
    loop {
        let name = if suffix == 0 {
            format!("audit-{date}.jsonl")
        } else {
            format!("audit-{date}-{suffix}.jsonl")
        };
        let path = base_dir.join(name);
        if let Ok(metadata) = fs::metadata(&path) {
            if metadata.len() >= max_bytes {
                suffix += 1;
                continue;
            }
        }
        return Ok(path);
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use tempfile::TempDir;

    #[test]
    fn audit_writes_jsonl() {
        let tmp = TempDir::new().unwrap();
        let logger = AuditLogger::new_with_dir(tmp.path().to_path_buf(), 1024).unwrap();
        logger
            .record("test.event", AuditStatus::Ok, Some("test"), None, None)
            .unwrap();
        let entries: Vec<_> = fs::read_dir(tmp.path()).unwrap().collect();
        assert!(!entries.is_empty());
        let path = entries[0].as_ref().unwrap().path();
        let contents = fs::read_to_string(path).unwrap();
        assert!(contents.contains("\"event\":\"test.event\""));
    }

    #[test]
    fn audit_rotates_when_max_reached() {
        let tmp = TempDir::new().unwrap();
        let logger = AuditLogger::new_with_dir(tmp.path().to_path_buf(), 1).unwrap();
        logger
            .record("test.event", AuditStatus::Ok, Some("test"), None, None)
            .unwrap();
        logger
            .record("test.event", AuditStatus::Ok, Some("test"), None, None)
            .unwrap();
        let entries: Vec<_> = fs::read_dir(tmp.path()).unwrap().collect();
        assert!(entries.len() >= 2);
    }
}

use std::collections::VecDeque;
use std::fmt::Debug;
use std::sync::{Arc, Mutex};

use time::OffsetDateTime;
use tracing::{Event, Level, Subscriber};
use tracing_subscriber::layer::{Context, Layer};

#[derive(Clone, Debug)]
pub struct LogEntry {
    pub timestamp: String,
    pub level: Level,
    pub target: String,
    pub fields: Vec<(String, String)>,
}

impl LogEntry {
    pub fn format_compact(&self) -> String {
        let message = self
            .fields
            .iter()
            .find(|(name, _)| name == "message")
            .map(|(_, value)| value.as_str())
            .unwrap_or("");
        let mut extras: Vec<String> = self
            .fields
            .iter()
            .filter(|(name, _)| name != "message")
            .map(|(name, value)| format!("{name}={value}"))
            .collect();
        extras.sort();
        if extras.is_empty() {
            format!(
                "{} {:<5} {} {}",
                self.timestamp, self.level, self.target, message
            )
        } else {
            format!(
                "{} {:<5} {} {} | {}",
                self.timestamp,
                self.level,
                self.target,
                message,
                extras.join(" ")
            )
        }
    }
}

#[derive(Clone)]
pub struct LogBuffer {
    entries: Arc<Mutex<VecDeque<LogEntry>>>,
    max_entries: usize,
}

impl LogBuffer {
    pub fn new(max_entries: usize) -> Self {
        Self {
            entries: Arc::new(Mutex::new(VecDeque::new())),
            max_entries,
        }
    }

    pub fn entries(&self) -> Vec<LogEntry> {
        self.entries
            .lock()
            .map(|entries| entries.iter().cloned().collect())
            .unwrap_or_default()
    }

    fn push(&self, entry: LogEntry) {
        if let Ok(mut entries) = self.entries.lock() {
            entries.push_back(entry);
            while entries.len() > self.max_entries {
                entries.pop_front();
            }
        }
    }
}

#[derive(Clone)]
pub struct LogLayer {
    buffer: LogBuffer,
}

impl LogLayer {
    pub fn new(buffer: LogBuffer) -> Self {
        Self { buffer }
    }
}

impl<S> Layer<S> for LogLayer
where
    S: Subscriber,
{
    fn on_event(&self, event: &Event<'_>, _ctx: Context<'_, S>) {
        let mut visitor = LogVisitor::default();
        event.record(&mut visitor);
        let metadata = event.metadata();
        let entry = LogEntry {
            timestamp: format_timestamp(OffsetDateTime::now_utc()),
            level: *metadata.level(),
            target: metadata.target().to_string(),
            fields: visitor.fields,
        };
        self.buffer.push(entry);
    }
}

#[derive(Default)]
struct LogVisitor {
    fields: Vec<(String, String)>,
}

impl LogVisitor {
    fn push(&mut self, field: &tracing::field::Field, value: String) {
        self.fields.push((field.name().to_string(), value));
    }
}

impl tracing::field::Visit for LogVisitor {
    fn record_bool(&mut self, field: &tracing::field::Field, value: bool) {
        self.push(field, value.to_string());
    }

    fn record_i64(&mut self, field: &tracing::field::Field, value: i64) {
        self.push(field, value.to_string());
    }

    fn record_u64(&mut self, field: &tracing::field::Field, value: u64) {
        self.push(field, value.to_string());
    }

    fn record_str(&mut self, field: &tracing::field::Field, value: &str) {
        self.push(field, value.to_string());
    }

    fn record_debug(&mut self, field: &tracing::field::Field, value: &dyn Debug) {
        self.push(field, format!("{value:?}"));
    }
}

fn format_timestamp(timestamp: OffsetDateTime) -> String {
    let format = time::format_description::parse("[hour repr:24]:[minute]:[second]")
        .unwrap_or_else(|_| time::format_description::parse("[second]").unwrap());
    timestamp
        .format(&format)
        .unwrap_or_else(|_| timestamp.unix_timestamp().to_string())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn format_compact_includes_fields() {
        let entry = LogEntry {
            timestamp: "12:34:56".to_string(),
            level: Level::INFO,
            target: "mirror_cli::tui".to_string(),
            fields: vec![
                ("message".to_string(), "hello".to_string()),
                ("view".to_string(), "dashboard".to_string()),
            ],
        };

        let formatted = entry.format_compact();
        assert!(formatted.contains("12:34:56"));
        assert!(formatted.contains("INFO"));
        assert!(formatted.contains("mirror_cli::tui"));
        assert!(formatted.contains("hello"));
        assert!(formatted.contains("view=dashboard"));
    }
}

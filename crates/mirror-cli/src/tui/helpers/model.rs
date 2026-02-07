use super::*;

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub(in crate::tui) enum AuditFilter {
    All,
    Failures,
}

#[derive(Clone)]
pub(in crate::tui) struct TokenEntry {
    pub(in crate::tui) account: String,
    pub(in crate::tui) provider: ProviderKind,
    pub(in crate::tui) scope: String,
    pub(in crate::tui) host: String,
    pub(in crate::tui) present: bool,
    pub(in crate::tui) validation: Option<TokenValidation>,
}

#[derive(Clone)]
pub(in crate::tui) struct TokenValidation {
    pub(in crate::tui) status: TokenValidationStatus,
    pub(in crate::tui) at: String,
}

impl TokenValidation {
    pub(in crate::tui) fn display(&self) -> String {
        match &self.status {
            TokenValidationStatus::Ok => format!("verified ok at {}", self.at),
            TokenValidationStatus::MissingScopes(scopes) => {
                format!("missing scopes ({}) at {}", scopes.join(", "), self.at)
            }
            TokenValidationStatus::Unsupported => format!(
                "token valid (scope validation not supported) at {}",
                self.at
            ),
        }
    }
}

#[derive(Clone, Debug)]
pub(in crate::tui) enum TokenValidationStatus {
    Ok,
    MissingScopes(Vec<String>),
    Unsupported,
}

pub(in crate::tui) struct DashboardStats {
    pub(in crate::tui) total_targets: usize,
    pub(in crate::tui) healthy_targets: usize,
    pub(in crate::tui) backoff_targets: usize,
    pub(in crate::tui) no_success_targets: usize,
    pub(in crate::tui) last_sync: Option<String>,
    pub(in crate::tui) audit_entries: usize,
    pub(in crate::tui) targets: Vec<DashboardTarget>,
}

pub(in crate::tui) struct DashboardTarget {
    pub(in crate::tui) id: String,
    pub(in crate::tui) status: String,
    pub(in crate::tui) last_success: String,
}

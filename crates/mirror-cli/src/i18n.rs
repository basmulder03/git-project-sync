use std::collections::HashMap;
use std::sync::{OnceLock, RwLock};

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum Locale {
    En001,
    EnUs,
    EnGb,
    Nl,
    Af,
}

impl Locale {
    pub fn as_bcp47(self) -> &'static str {
        match self {
            Locale::En001 => "en-001",
            Locale::EnUs => "en-US",
            Locale::EnGb => "en-GB",
            Locale::Nl => "nl",
            Locale::Af => "af",
        }
    }

    pub fn label(self) -> &'static str {
        match self {
            Locale::En001 => "English (International)",
            Locale::EnUs => "English (American)",
            Locale::EnGb => "English (British)",
            Locale::Nl => "Dutch",
            Locale::Af => "Afrikaans",
        }
    }
}

const SUPPORTED_LOCALES: [Locale; 5] = [
    Locale::En001,
    Locale::EnUs,
    Locale::EnGb,
    Locale::Nl,
    Locale::Af,
];

pub fn supported_locales() -> &'static [Locale] {
    &SUPPORTED_LOCALES
}

pub fn parse_locale(input: &str) -> Option<Locale> {
    let normalized = input.trim().replace('_', "-").to_ascii_lowercase();
    match normalized.as_str() {
        "en-001" | "en" => Some(Locale::En001),
        "en-us" => Some(Locale::EnUs),
        "en-gb" => Some(Locale::EnGb),
        "nl" | "nl-nl" => Some(Locale::Nl),
        "af" | "af-za" => Some(Locale::Af),
        _ => None,
    }
}

pub fn resolve_locale(
    cli_lang: Option<&str>,
    env_lang: Option<&str>,
    config_lang: Option<&str>,
) -> Locale {
    cli_lang
        .and_then(parse_locale)
        .or_else(|| env_lang.and_then(parse_locale))
        .or_else(|| config_lang.and_then(parse_locale))
        .unwrap_or(Locale::En001)
}

static ACTIVE_LOCALE: OnceLock<RwLock<Locale>> = OnceLock::new();

pub fn active_locale() -> Locale {
    let lock = ACTIVE_LOCALE.get_or_init(|| RwLock::new(Locale::En001));
    *lock.read().expect("read locale")
}

pub fn set_active_locale(locale: Locale) {
    let lock = ACTIVE_LOCALE.get_or_init(|| RwLock::new(Locale::En001));
    *lock.write().expect("write locale") = locale;
}

#[allow(dead_code)]
pub mod key {
    pub const AUDIT_ID: &str = "common.audit_id";
    pub const CONFIG_SAVED: &str = "config.saved";
    pub const CONFIG_MIGRATED_SAVED: &str = "config.migrated_saved";
    pub const LANGUAGE_SET: &str = "language.set";
    pub const LANGUAGE_CONFIGURED: &str = "language.configured";
    pub const LANGUAGE_EFFECTIVE: &str = "language.effective";
    pub const LANGUAGE_NONE_EFFECTIVE: &str = "language.none_effective";
    pub const LANGUAGE_LABEL: &str = "language.label";
    pub const LANGUAGE_CONTEXT: &str = "language.context";
    pub const LANGUAGE_SAVED_AUDIT: &str = "language.saved_audit";
    pub const WARNING_GENERIC: &str = "common.warning";
    pub const WARNING_TARGET_ID_PRECEDENCE: &str = "common.warning_target_id_precedence";
    pub const NO_MATCHING_TARGETS: &str = "common.no_matching_targets";
    pub const NO_TARGETS_CONFIGURED: &str = "common.no_targets_configured";
    pub const PROMPT_ENTER_ARS: &str = "sync.prompt_enter_ars";
    pub const REMOTE_MISSING_PROMPT: &str = "sync.remote_missing_prompt";
    pub const TARGET_EXISTS: &str = "target.exists";
    pub const TARGET_ADDED_TO_PATH: &str = "target.added_to_path";
    pub const TARGET_NOT_FOUND_ID: &str = "target.not_found_id";
    pub const TARGET_REMOVED_FROM_PATH: &str = "target.removed_from_path";
    pub const TOKEN_STORED_ACCOUNT: &str = "token.stored_account";
    pub const PROVIDER_LABEL: &str = "common.provider_label";
    pub const SCOPE_LABEL: &str = "common.scope_label";
    pub const CREATE_PAT_URL: &str = "token.create_pat_url";
    pub const REQUIRED_ACCESS: &str = "token.required_access";
    pub const TOKEN_SCOPES_VALID: &str = "token.scopes_valid";
    pub const TOKEN_SCOPES_MISSING: &str = "token.scopes_missing";
    pub const UPDATE_CHECK_FAILED: &str = "update.check_failed";
    pub const ROOT_PATH_EMPTY: &str = "config.root_empty";
    pub const ROOT_SAVED_AUDIT: &str = "config.root_saved_audit";
    pub const LABEL_SCOPE_SPACED: &str = "form.scope_spaced";
    pub const LABEL_HOST_OPTIONAL: &str = "form.host_optional";
    pub const LABEL_LABELS_COMMA: &str = "form.labels_comma";
    pub const LABEL_TARGET_ID: &str = "form.target_id";
    pub const LABEL_TOKEN: &str = "form.token";
    pub const LABEL_DELAY_SECONDS: &str = "form.delay_seconds";
    pub const LABEL_ADD_PATH: &str = "form.add_path";
    pub const MAIN_MENU: &str = "main.menu";
    pub const DASHBOARD: &str = "dashboard.title";
    pub const DASHBOARD_SYSTEM_STATUS: &str = "dashboard.system_status";
    pub const DASHBOARD_PER_TARGET: &str = "dashboard.per_target";
    pub const DASHBOARD_PRESS_T: &str = "dashboard.press_t";
    pub const SETUP_CONTEXT: &str = "setup.context";
    pub const TASK_NAME: &str = "setup.task_name";
    pub const STATUS_UNAVAILABLE: &str = "common.status_unavailable";
    pub const TIP_PRESS_U_UPDATE: &str = "common.tip_press_u_update";
    pub const VALIDATION_LABEL: &str = "common.validation";
    pub const SETUP_TITLE: &str = "setup.title";
    pub const TOKENS_TITLE: &str = "tokens.title";
    pub const TOKENS_CONTEXT_PER_TARGET: &str = "tokens.context_per_target";
    pub const TOKENS_NONE_CONFIGURED_YET: &str = "tokens.none_configured_yet";
    pub const TOKENS_CONTEXT_SET: &str = "tokens.context_set";
    pub const TOKENS_TIP_SCOPE: &str = "tokens.tip_scope";
    pub const TOKEN_SET_TITLE: &str = "tokens.set_title";
    pub const TOKENS_CONTEXT_VALIDATE: &str = "tokens.context_validate";
    pub const TOKENS_TIP_HOST: &str = "tokens.tip_host";
    pub const TOKEN_VALIDATE_TITLE: &str = "tokens.validate_title";
    pub const SERVICE_CONTEXT: &str = "service.context";
    pub const SERVICE_PRESS_INSTALL: &str = "service.press_install";
    pub const SERVICE_PRESS_UNINSTALL: &str = "service.press_uninstall";
    pub const SERVICE_TITLE: &str = "service.title";
    pub const CONFIG_ROOT_CONTEXT: &str = "config_root.context";
    pub const CONFIG_ROOT_TIP_PATH: &str = "config_root.tip_path";
    pub const CONFIG_ROOT_NEW_ROOT: &str = "config_root.new_root";
    pub const CONFIG_ROOT_TITLE: &str = "config_root.title";
    pub const CONFIG_ROOT_LABEL: &str = "config_root.label";
    pub const SETUP_UNAVAILABLE: &str = "setup.unavailable";
    pub const REPO_OVERVIEW_UNAVAILABLE: &str = "repo_overview.unavailable";
    pub const DELAY_MUST_BE_NUMBER: &str = "setup.delay_must_be_number";
    pub const TOKEN_EMPTY: &str = "token.empty";
    pub const TOKEN_STORED_AUDIT: &str = "token.stored_audit";
    pub const STATUS_AUDIT: &str = "common.status_audit";
    pub const VALIDATION_FAILED: &str = "common.validation_failed";
}

pub fn tr(key: &str) -> String {
    let locale = active_locale();
    if let Some(value) = catalog(locale).get(key) {
        return value.clone();
    }
    if let Some(value) = catalog(Locale::En001).get(key) {
        return value.clone();
    }
    key.to_string()
}

pub fn tf(template: &str, values: &[(&str, String)]) -> String {
    let mut out = tr(template);
    for (name, value) in values {
        let needle = format!("{{{name}}}");
        out = out.replace(&needle, value);
    }
    out
}

fn catalog(locale: Locale) -> &'static HashMap<String, String> {
    match locale {
        Locale::En001 => {
            CATALOG_EN001.get_or_init(|| parse_catalog(include_str!("i18n/en-001.json")))
        }
        Locale::EnUs => CATALOG_ENUS.get_or_init(|| parse_catalog(include_str!("i18n/en-US.json"))),
        Locale::EnGb => CATALOG_ENGB.get_or_init(|| parse_catalog(include_str!("i18n/en-GB.json"))),
        Locale::Nl => CATALOG_NL.get_or_init(|| parse_catalog(include_str!("i18n/nl.json"))),
        Locale::Af => CATALOG_AF.get_or_init(|| parse_catalog(include_str!("i18n/af.json"))),
    }
}

fn parse_catalog(raw: &str) -> HashMap<String, String> {
    serde_json::from_str(raw).expect("parse i18n catalog")
}

static CATALOG_EN001: OnceLock<HashMap<String, String>> = OnceLock::new();
static CATALOG_ENUS: OnceLock<HashMap<String, String>> = OnceLock::new();
static CATALOG_ENGB: OnceLock<HashMap<String, String>> = OnceLock::new();
static CATALOG_NL: OnceLock<HashMap<String, String>> = OnceLock::new();
static CATALOG_AF: OnceLock<HashMap<String, String>> = OnceLock::new();

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn locale_precedence_cli_env_config() {
        assert_eq!(
            resolve_locale(Some("nl"), Some("af"), Some("en-GB")),
            Locale::Nl
        );
        assert_eq!(resolve_locale(None, Some("af"), Some("en-GB")), Locale::Af);
        assert_eq!(resolve_locale(None, None, Some("en-GB")), Locale::EnGb);
        assert_eq!(resolve_locale(None, None, None), Locale::En001);
    }

    #[test]
    fn locale_parsing_supports_requested_tags() {
        assert_eq!(parse_locale("en-001"), Some(Locale::En001));
        assert_eq!(parse_locale("en-US"), Some(Locale::EnUs));
        assert_eq!(parse_locale("en-GB"), Some(Locale::EnGb));
        assert_eq!(parse_locale("nl"), Some(Locale::Nl));
        assert_eq!(parse_locale("af"), Some(Locale::Af));
    }

    #[test]
    fn non_default_locales_can_fallback_to_en001() {
        set_active_locale(Locale::Nl);
        let text = tr(key::MAIN_MENU);
        assert!(!text.is_empty());
    }
}

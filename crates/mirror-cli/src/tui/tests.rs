use super::*;
use tempfile::TempDir;

#[test]
fn optional_text_handles_empty() {
    assert_eq!(optional_text(""), None);
    assert_eq!(optional_text("  "), None);
    assert_eq!(optional_text("hi"), Some("hi".to_string()));
}

#[test]
fn split_labels_parses_list() {
    let labels = split_labels("a, b, ,c");
    assert_eq!(
        labels,
        vec!["a".to_string(), "b".to_string(), "c".to_string()]
    );
}

#[test]
fn format_delayed_start_reports_none() {
    assert_eq!(format_delayed_start(None), "none");
    assert_eq!(format_delayed_start(Some(0)), "none");
    assert_eq!(format_delayed_start(Some(15)), "15s");
}

#[test]
fn menu_index_wraps_in_main_menu() {
    let tmp = TempDir::new().unwrap();
    let mut app = TuiApp {
        config_path: std::path::PathBuf::from("/tmp/config.json"),
        config: AppConfigV2::default(),
        view: View::Main,
        menu_index: 10,
        message: String::new(),
        input_index: 0,
        input_fields: Vec::new(),
        provider_index: 0,
        language_index: 0,
        token_menu_index: 0,
        token_validation: HashMap::new(),
        audit: AuditLogger::new_with_dir(tmp.path().to_path_buf(), 1024).unwrap(),
        log_buffer: LogBuffer::new(50),
        audit_filter: AuditFilter::All,
        validation_message: None,
        show_target_stats: false,
        repo_status: HashMap::new(),
        repo_status_last_refresh: None,
        repo_status_refreshing: false,
        repo_status_rx: None,
        repo_overview_message: None,
        sync_running: false,
        sync_rx: None,
        install_guard: None,
        install_rx: None,
        install_progress: None,
        install_status: None,
        update_rx: None,
        update_progress: None,
        update_prompt: None,
        update_return_view: View::Main,
        restart_requested: false,
        message_return_view: View::Main,
        audit_search: String::new(),
        audit_search_active: false,
        view_stack: Vec::new(),
        scroll_offsets: HashMap::new(),
        repo_overview_selected: 0,
        repo_overview_scroll: 0,
        repo_overview_collapsed: HashSet::new(),
        repo_overview_compact: false,
    };
    let key = KeyEvent::new(KeyCode::Down, KeyModifiers::empty());
    app.handle_main(key).unwrap();
    assert_eq!(app.menu_index, 0);
}

#[test]
fn read_audit_lines_handles_missing_file() {
    let tmp = TempDir::new().unwrap();
    let path = tmp.path().join("missing.jsonl");
    let lines = read_audit_lines(&path, AuditFilter::All).unwrap();
    assert_eq!(lines[0], "No audit log found for today.");
}

#[test]
fn read_audit_lines_filters_failures() {
    let tmp = TempDir::new().unwrap();
    let path = tmp.path().join("audit.jsonl");
    std::fs::write(
        &path,
        r#"{"status":"ok","event":"a"}
{"status":"failed","event":"b"}
"#,
    )
    .unwrap();
    let lines = read_audit_lines(&path, AuditFilter::Failures).unwrap();
    assert_eq!(lines.len(), 1);
    assert!(lines[0].contains("\"status\":\"failed\""));
}

#[test]
fn token_menu_enter_moves_to_set_view() {
    let tmp = TempDir::new().unwrap();
    let mut app = TuiApp {
        config_path: std::path::PathBuf::from("/tmp/config.json"),
        config: AppConfigV2::default(),
        view: View::TokenMenu,
        menu_index: 0,
        message: String::new(),
        input_index: 0,
        input_fields: Vec::new(),
        provider_index: 0,
        language_index: 0,
        token_menu_index: 1,
        token_validation: HashMap::new(),
        audit: AuditLogger::new_with_dir(tmp.path().to_path_buf(), 1024).unwrap(),
        log_buffer: LogBuffer::new(50),
        audit_filter: AuditFilter::All,
        validation_message: None,
        show_target_stats: false,
        repo_status: HashMap::new(),
        repo_status_last_refresh: None,
        repo_status_refreshing: false,
        repo_status_rx: None,
        repo_overview_message: None,
        sync_running: false,
        sync_rx: None,
        install_guard: None,
        install_rx: None,
        install_progress: None,
        install_status: None,
        update_rx: None,
        update_progress: None,
        update_prompt: None,
        update_return_view: View::Main,
        restart_requested: false,
        message_return_view: View::Main,
        audit_search: String::new(),
        audit_search_active: false,
        view_stack: Vec::new(),
        scroll_offsets: HashMap::new(),
        repo_overview_selected: 0,
        repo_overview_scroll: 0,
        repo_overview_collapsed: HashSet::new(),
        repo_overview_compact: false,
    };
    let key = KeyEvent::new(KeyCode::Enter, KeyModifiers::empty());
    app.handle_token_menu(key).unwrap();
    assert_eq!(app.view, View::TokenSet);
}

#[test]
fn token_validation_display_reports_missing_scopes() {
    let validation = TokenValidation {
        status: TokenValidationStatus::MissingScopes(vec![
            "repo".to_string(),
            "read:org".to_string(),
        ]),
        at: "2026-02-04 12:00:00".to_string(),
    };
    let message = validation.display();
    assert!(message.contains("missing scopes"));
    assert!(message.contains("repo"));
}

#[test]
fn token_validation_display_reports_unsupported_scopes() {
    let validation = TokenValidation {
        status: TokenValidationStatus::Unsupported,
        at: "2026-02-04 12:00:00".to_string(),
    };
    let message = validation.display();
    assert_eq!(
        message,
        "token valid (scope validation not supported) at 2026-02-04 12:00:00"
    );
}

#[test]
fn form_hint_is_present_for_target_add() {
    let tmp = TempDir::new().unwrap();
    let app = TuiApp {
        config_path: std::path::PathBuf::from("/tmp/config.json"),
        config: AppConfigV2::default(),
        view: View::TargetAdd,
        menu_index: 0,
        message: String::new(),
        input_index: 0,
        input_fields: Vec::new(),
        provider_index: 0,
        language_index: 0,
        token_menu_index: 0,
        token_validation: HashMap::new(),
        audit: AuditLogger::new_with_dir(tmp.path().to_path_buf(), 1024).unwrap(),
        log_buffer: LogBuffer::new(50),
        audit_filter: AuditFilter::All,
        validation_message: None,
        show_target_stats: false,
        repo_status: HashMap::new(),
        repo_status_last_refresh: None,
        repo_status_refreshing: false,
        repo_status_rx: None,
        repo_overview_message: None,
        sync_running: false,
        sync_rx: None,
        install_guard: None,
        install_rx: None,
        install_progress: None,
        install_status: None,
        update_rx: None,
        update_progress: None,
        update_prompt: None,
        update_return_view: View::Main,
        restart_requested: false,
        message_return_view: View::Main,
        audit_search: String::new(),
        audit_search_active: false,
        view_stack: Vec::new(),
        scroll_offsets: HashMap::new(),
        repo_overview_selected: 0,
        repo_overview_scroll: 0,
        repo_overview_collapsed: HashSet::new(),
        repo_overview_compact: false,
    };
    assert!(app.form_hint().is_some());
}

#[test]
fn install_action_for_versions_detects_update() {
    let action = install_action_for_versions(true, Some("1.2.3"), "1.3.0", false);
    assert_eq!(action, InstallAction::Update);
}

#[test]
fn install_action_for_versions_install_when_missing() {
    let action = install_action_for_versions(false, None, "1.2.3", false);
    assert_eq!(action, InstallAction::Install);
}

#[test]
fn install_action_for_versions_reinstall_when_current() {
    let action = install_action_for_versions(true, Some("1.2.3"), "1.2.3", false);
    assert_eq!(action, InstallAction::Reinstall);
    let action = install_action_for_versions(true, Some("1.2.3"), "1.3.0", true);
    assert_eq!(action, InstallAction::Reinstall);
}

#[test]
fn dashboard_footer_includes_force_hotkey() {
    assert!(dashboard_footer_text().contains("f: force refresh all"));
}

#[test]
fn esc_navigation_returns_to_previous_view() {
    let tmp = TempDir::new().unwrap();
    let mut app = TuiApp {
        config_path: std::path::PathBuf::from("/tmp/config.json"),
        config: AppConfigV2::default(),
        view: View::Main,
        menu_index: 0,
        message: String::new(),
        input_index: 0,
        input_fields: Vec::new(),
        provider_index: 0,
        language_index: 0,
        token_menu_index: 0,
        token_validation: HashMap::new(),
        audit: AuditLogger::new_with_dir(tmp.path().to_path_buf(), 1024).unwrap(),
        log_buffer: LogBuffer::new(50),
        audit_filter: AuditFilter::All,
        validation_message: None,
        show_target_stats: false,
        repo_status: HashMap::new(),
        repo_status_last_refresh: None,
        repo_status_refreshing: false,
        repo_status_rx: None,
        repo_overview_message: None,
        sync_running: false,
        sync_rx: None,
        install_guard: None,
        install_rx: None,
        install_progress: None,
        install_status: None,
        update_rx: None,
        update_progress: None,
        update_prompt: None,
        update_return_view: View::Main,
        restart_requested: false,
        message_return_view: View::Main,
        audit_search: String::new(),
        audit_search_active: false,
        view_stack: Vec::new(),
        scroll_offsets: HashMap::new(),
        repo_overview_selected: 0,
        repo_overview_scroll: 0,
        repo_overview_collapsed: HashSet::new(),
        repo_overview_compact: false,
    };

    app.navigate_to(View::Targets);
    app.navigate_to(View::TargetAdd);
    assert_eq!(app.view, View::TargetAdd);
    app.handle_key(KeyEvent::new(KeyCode::Esc, KeyModifiers::empty()))
        .unwrap();
    assert_eq!(app.view, View::Targets);
}

#[test]
fn global_scroll_keys_update_view_scroll_offset() {
    let tmp = TempDir::new().unwrap();
    let mut app = TuiApp {
        config_path: std::path::PathBuf::from("/tmp/config.json"),
        config: AppConfigV2::default(),
        view: View::Dashboard,
        menu_index: 0,
        message: String::new(),
        input_index: 0,
        input_fields: Vec::new(),
        provider_index: 0,
        language_index: 0,
        token_menu_index: 0,
        token_validation: HashMap::new(),
        audit: AuditLogger::new_with_dir(tmp.path().to_path_buf(), 1024).unwrap(),
        log_buffer: LogBuffer::new(50),
        audit_filter: AuditFilter::All,
        validation_message: None,
        show_target_stats: false,
        repo_status: HashMap::new(),
        repo_status_last_refresh: None,
        repo_status_refreshing: false,
        repo_status_rx: None,
        repo_overview_message: None,
        sync_running: false,
        sync_rx: None,
        install_guard: None,
        install_rx: None,
        install_progress: None,
        install_status: None,
        update_rx: None,
        update_progress: None,
        update_prompt: None,
        update_return_view: View::Main,
        restart_requested: false,
        message_return_view: View::Main,
        audit_search: String::new(),
        audit_search_active: false,
        view_stack: Vec::new(),
        scroll_offsets: HashMap::new(),
        repo_overview_selected: 0,
        repo_overview_scroll: 0,
        repo_overview_collapsed: HashSet::new(),
        repo_overview_compact: false,
    };

    app.handle_key(KeyEvent::new(KeyCode::PageDown, KeyModifiers::empty()))
        .unwrap();
    assert!(app.scroll_offset(View::Dashboard) > 0);
    app.handle_key(KeyEvent::new(KeyCode::Home, KeyModifiers::empty()))
        .unwrap();
    assert_eq!(app.scroll_offset(View::Dashboard), 0);
}

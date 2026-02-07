use super::shared::{
    admin_privileges_prompt_label, azdo_message_for_status, github_status_message,
    gitlab_status_message,
};
use super::*;

#[test]
fn azdo_message_for_auth_errors() {
    let scope = ProviderScope::new(vec!["org".into()]).unwrap();
    let target = ProviderTarget {
        provider: ProviderKind::AzureDevOps,
        scope,
        host: None,
    };
    let message = azdo_message_for_status(&target, StatusCode::UNAUTHORIZED).unwrap();
    assert!(message.contains("authentication failed"));
    let message = azdo_message_for_status(&target, StatusCode::FORBIDDEN).unwrap();
    assert!(message.contains("authentication failed"));
}

#[test]
fn azdo_message_for_not_found() {
    let scope = ProviderScope::new(vec!["org".into(), "proj".into()]).unwrap();
    let target = ProviderTarget {
        provider: ProviderKind::AzureDevOps,
        scope,
        host: None,
    };
    let message = azdo_message_for_status(&target, StatusCode::NOT_FOUND).unwrap();
    assert!(message.contains("scope not found"));
    assert!(message.contains("org/proj"));
}

#[test]
fn github_status_messages() {
    let scope = "org";
    let message = github_status_message(scope, StatusCode::UNAUTHORIZED).unwrap();
    assert!(message.contains("GitHub authentication failed"));
    let message = github_status_message(scope, StatusCode::NOT_FOUND).unwrap();
    assert!(message.contains("scope not found"));
}

#[test]
fn gitlab_status_messages() {
    let scope = "group";
    let message = gitlab_status_message(scope, StatusCode::FORBIDDEN).unwrap();
    assert!(message.contains("GitLab authentication failed"));
    let message = gitlab_status_message(scope, StatusCode::NOT_FOUND).unwrap();
    assert!(message.contains("scope not found"));
}

#[test]
fn update_check_only_parses() {
    let cli = Cli::try_parse_from(["mirror-cli", "update", "--check-only"]).unwrap();
    match cli.command {
        Commands::Update(args) => {
            assert!(args.check_only);
        }
        _ => panic!("expected update command"),
    }
    let cli = Cli::try_parse_from(["mirror-cli", "update", "--check"]).unwrap();
    match cli.command {
        Commands::Update(args) => {
            assert!(args.check_only);
        }
        _ => panic!("expected update command"),
    }
}

#[test]
fn check_updates_flag_parses() {
    let cli = Cli::try_parse_from(["mirror-cli", "--check-updates", "sync"]).unwrap();
    assert!(cli.check_updates);
}

#[test]
fn sync_force_refresh_all_parses() {
    let cli = Cli::try_parse_from(["mirror-cli", "sync", "--force-refresh-all"]).unwrap();
    match cli.command {
        Commands::Sync(args) => assert!(args.force_refresh_all),
        _ => panic!("expected sync command"),
    }
}

#[test]
fn token_doctor_parses() {
    let cli = Cli::try_parse_from(["mirror-cli", "token", "doctor"]).unwrap();
    match cli.command {
        Commands::Token(TokenArgs {
            command: TokenCommands::Doctor(_),
        }) => {}
        _ => panic!("expected token doctor command"),
    }
}

#[test]
fn config_language_set_parses() {
    let cli =
        Cli::try_parse_from(["mirror-cli", "config", "language", "set", "--lang", "nl"]).unwrap();
    match cli.command {
        Commands::Config(ConfigArgs {
            command:
                ConfigCommands::Language(ConfigLanguageArgs {
                    command: ConfigLanguageCommands::Set(args),
                }),
        }) => assert_eq!(args.lang, "nl"),
        _ => panic!("expected config language set command"),
    }
}

#[test]
fn admin_prompt_label_matches_os() {
    let label = admin_privileges_prompt_label();
    if cfg!(target_os = "windows") {
        assert!(label.contains("User Account Control"));
    } else if cfg!(target_os = "macos") {
        assert!(label.contains("macOS"));
    } else {
        assert!(label.contains("sudo"));
    }
}

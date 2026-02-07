use super::*;
pub(super) fn handle_token(args: TokenArgs, audit: &AuditLogger) -> anyhow::Result<()> {
    match args.command {
        TokenCommands::Set(args) => handle_set_token(args, audit),
        TokenCommands::Guide(args) => handle_guide_token(args, audit),
        TokenCommands::Validate(args) => handle_validate_token(args, audit),
        TokenCommands::Doctor(args) => handle_doctor_token(args, audit),
    }
}

pub(super) fn handle_set_token(args: SetTokenArgs, audit: &AuditLogger) -> anyhow::Result<()> {
    let result: anyhow::Result<()> = (|| {
        let provider: ProviderKind = args.provider.into();
        let spec = spec_for(provider.clone());
        let scope = spec.parse_scope(args.scope)?;
        let host = host_or_default(args.host.as_deref(), spec.as_ref());
        let account = spec.account_key(&host, &scope)?;
        auth::set_pat(&account, &args.token)?;
        auth::get_pat(&account).context("read token from keyring after write")?;
        let runtime_target = ProviderTarget {
            provider: provider.clone(),
            scope: scope.clone(),
            host: Some(host.clone()),
        };
        let validation = token_check::check_token_validity(&runtime_target);
        if validation.status != token_check::TokenValidity::Ok {
            let _ = auth::delete_pat(&account);
            let mut message = validation.message(&runtime_target);
            if let Some(error) = validation.error.as_deref() {
                message.push_str(": ");
                message.push_str(error);
            }
            anyhow::bail!(message);
        }
        println!("Token stored for {account}");
        let audit_id = audit.record_with_context(
            "token.set",
            AuditStatus::Ok,
            Some("token.set"),
            AuditContext {
                provider: Some(provider.as_prefix().to_string()),
                scope: Some(scope.segments().join("/")),
                repo_id: None,
                path: None,
            },
            None,
            None,
        )?;
        println!("Audit ID: {audit_id}");
        Ok(())
    })();

    if let Err(err) = &result {
        let _ = audit.record(
            "token.set",
            AuditStatus::Failed,
            Some("token.set"),
            None,
            Some(&err.to_string()),
        );
    }
    result
}

pub(super) fn handle_guide_token(args: GuideTokenArgs, audit: &AuditLogger) -> anyhow::Result<()> {
    let result: anyhow::Result<()> = (|| {
        let provider: ProviderKind = args.provider.into();
        let spec = spec_for(provider.clone());
        let scope = spec.parse_scope(args.scope)?;
        let help = mirror_providers::spec::pat_help(provider.clone());
        println!("Provider: {}", provider.as_prefix());
        println!("Scope: {}", scope.segments().join("/"));
        println!("Create PAT at: {}", help.url);
        println!("Required access:");
        for scope in help.scopes {
            println!("  - {scope}");
        }
        let audit_id = audit.record_with_context(
            "token.guide",
            AuditStatus::Ok,
            Some("token.guide"),
            AuditContext {
                provider: Some(provider.as_prefix().to_string()),
                scope: Some(scope.segments().join("/")),
                repo_id: None,
                path: None,
            },
            None,
            None,
        )?;
        println!("Audit ID: {audit_id}");
        Ok(())
    })();

    if let Err(err) = &result {
        let _ = audit.record(
            "token.guide",
            AuditStatus::Failed,
            Some("token.guide"),
            None,
            Some(&err.to_string()),
        );
    }
    result
}

pub(super) fn handle_validate_token(
    args: ValidateTokenArgs,
    audit: &AuditLogger,
) -> anyhow::Result<()> {
    let result: anyhow::Result<()> = (|| {
        let provider: ProviderKind = args.provider.into();
        let spec = spec_for(provider.clone());
        let scope = spec.parse_scope(args.scope)?;
        let host = args
            .host
            .as_ref()
            .map(|value| value.trim_end_matches('/').to_string());
        let runtime_target = ProviderTarget {
            provider: provider.clone(),
            scope: scope.clone(),
            host,
        };
        let registry = ProviderRegistry::new();
        let adapter = registry.provider(provider.clone())?;
        let scopes = adapter.token_scopes(&runtime_target)?;
        let help = mirror_providers::spec::pat_help(provider.clone());
        match scopes {
            Some(scopes) => {
                let missing: Vec<&str> = help
                    .scopes
                    .iter()
                    .copied()
                    .filter(|required| !scopes.iter().any(|s| s == required))
                    .collect();
                if missing.is_empty() {
                    println!("Token scopes valid for {}", provider.as_prefix());
                    let audit_id = audit.record_with_context(
                        "token.validate",
                        AuditStatus::Ok,
                        Some("token.validate"),
                        AuditContext {
                            provider: Some(provider.as_prefix().to_string()),
                            scope: Some(scope.segments().join("/")),
                            repo_id: None,
                            path: None,
                        },
                        None,
                        None,
                    )?;
                    println!("Audit ID: {audit_id}");
                } else {
                    println!("Missing scopes:");
                    for scope in missing {
                        println!("  - {scope}");
                    }
                    let audit_id = audit.record_with_context(
                        "token.validate",
                        AuditStatus::Failed,
                        Some("token.validate"),
                        AuditContext {
                            provider: Some(provider.as_prefix().to_string()),
                            scope: Some(scope.segments().join("/")),
                            repo_id: None,
                            path: None,
                        },
                        None,
                        Some("missing scopes"),
                    )?;
                    println!("Audit ID: {audit_id}");
                }
            }
            None => {
                let token_check_result = token_check::check_token_validity(&runtime_target);
                token_check::ensure_token_valid(&token_check_result, &runtime_target)?;
                println!(
                    "{} (scope validation not supported)",
                    token_check_result.message(&runtime_target)
                );
                let audit_id = audit.record_with_context(
                    "token.validate",
                    AuditStatus::Ok,
                    Some("token.validate"),
                    AuditContext {
                        provider: Some(provider.as_prefix().to_string()),
                        scope: Some(scope.segments().join("/")),
                        repo_id: None,
                        path: None,
                    },
                    None,
                    Some("auth-based validation used (scope validation not supported)"),
                )?;
                println!("Audit ID: {audit_id}");
            }
        }
        Ok(())
    })();

    if let Err(err) = &result {
        let _ = audit.record(
            "token.validate",
            AuditStatus::Failed,
            Some("token.validate"),
            None,
            Some(&err.to_string()),
        );
    }
    result
}

pub(super) fn handle_doctor_token(
    args: DoctorTokenArgs,
    audit: &AuditLogger,
) -> anyhow::Result<()> {
    let result: anyhow::Result<()> = (|| {
        println!(
            "DBUS_SESSION_BUS_ADDRESS: {}",
            std::env::var("DBUS_SESSION_BUS_ADDRESS")
                .ok()
                .filter(|v| !v.trim().is_empty())
                .unwrap_or_else(|| "(missing)".to_string())
        );
        println!(
            "XDG_RUNTIME_DIR: {}",
            std::env::var("XDG_RUNTIME_DIR")
                .ok()
                .filter(|v| !v.trim().is_empty())
                .unwrap_or_else(|| "(missing)".to_string())
        );

        match auth::probe_keyring_roundtrip() {
            Ok(_) => println!("Keyring roundtrip: OK"),
            Err(err) => anyhow::bail!("Keyring roundtrip failed: {err:#}"),
        }

        if !args.scope.is_empty() && args.provider.is_none() {
            anyhow::bail!("--scope requires --provider");
        }
        if let Some(provider_value) = args.provider {
            let provider: ProviderKind = provider_value.into();
            let spec = spec_for(provider.clone());
            let scope = spec.parse_scope(args.scope)?;
            let host = host_or_default(args.host.as_deref(), spec.as_ref());
            let account = spec.account_key(&host, &scope)?;
            match auth::get_pat(&account) {
                Ok(_) => println!("Stored token account check: found ({account})"),
                Err(err) => println!("Stored token account check: missing ({account}) -> {err:#}"),
            }
        }
        let audit_id = audit.record("token.doctor", AuditStatus::Ok, Some("token"), None, None)?;
        println!("Audit ID: {audit_id}");
        Ok(())
    })();

    if let Err(err) = &result {
        let _ = audit.record(
            "token.doctor",
            AuditStatus::Failed,
            Some("token"),
            None,
            Some(&err.to_string()),
        );
    }
    result
}

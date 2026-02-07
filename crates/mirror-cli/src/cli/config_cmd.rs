use super::*;
use crate::i18n::{active_locale, key, resolve_locale, tf};

pub(super) fn handle_config(args: ConfigArgs, audit: &AuditLogger) -> anyhow::Result<()> {
    match args.command {
        ConfigCommands::Init(args) => handle_init(args, audit),
        ConfigCommands::Language(args) => handle_language(args, audit),
    }
}

pub(super) fn handle_init(args: InitArgs, audit: &AuditLogger) -> anyhow::Result<()> {
    let result: anyhow::Result<()> = (|| {
        let config_path = default_config_path()?;
        let (mut config, migrated) = load_or_migrate(&config_path)?;
        config.root = Some(args.root);
        config.save(&config_path)?;
        if migrated {
            println!(
                "{}",
                tf(
                    key::CONFIG_MIGRATED_SAVED,
                    &[("path", config_path.display().to_string())]
                )
            );
        } else {
            println!(
                "{}",
                tf(
                    key::CONFIG_SAVED,
                    &[("path", config_path.display().to_string())]
                )
            );
        }
        Ok(())
    })();

    if let Err(err) = &result {
        let _ = audit.record(
            "config.init",
            AuditStatus::Failed,
            Some("config.init"),
            None,
            Some(&err.to_string()),
        );
    } else {
        let audit_id = audit.record(
            "config.init",
            AuditStatus::Ok,
            Some("config.init"),
            None,
            None,
        )?;
        println!("{}", tf(key::AUDIT_ID, &[("audit_id", audit_id)]));
    }
    result
}

fn handle_language(args: ConfigLanguageArgs, audit: &AuditLogger) -> anyhow::Result<()> {
    match args.command {
        ConfigLanguageCommands::Set(args) => handle_language_set(args, audit),
        ConfigLanguageCommands::Show => handle_language_show(audit),
    }
}

fn handle_language_set(args: ConfigLanguageSetArgs, audit: &AuditLogger) -> anyhow::Result<()> {
    let result: anyhow::Result<()> = (|| {
        let locale = resolve_locale(Some(&args.lang), None, None);
        let config_path = default_config_path()?;
        let (mut config, migrated) = load_or_migrate(&config_path)?;
        config.language = Some(locale.as_bcp47().to_string());
        config.save(&config_path)?;
        if migrated {
            println!(
                "{}",
                tf(
                    key::CONFIG_MIGRATED_SAVED,
                    &[("path", config_path.display().to_string())]
                )
            );
        }
        println!(
            "{}",
            tf(
                key::LANGUAGE_SET,
                &[("lang", locale.as_bcp47().to_string())]
            )
        );
        Ok(())
    })();

    if let Err(err) = &result {
        let _ = audit.record(
            "config.language.set",
            AuditStatus::Failed,
            Some("config.language"),
            None,
            Some(&err.to_string()),
        );
    } else {
        let audit_id = audit.record(
            "config.language.set",
            AuditStatus::Ok,
            Some("config.language"),
            None,
            None,
        )?;
        println!("{}", tf(key::AUDIT_ID, &[("audit_id", audit_id)]));
    }
    result
}

fn handle_language_show(audit: &AuditLogger) -> anyhow::Result<()> {
    let result: anyhow::Result<()> = (|| {
        let config_path = default_config_path()?;
        let (config, _) = load_or_migrate(&config_path)?;
        if let Some(lang) = config.language.as_deref() {
            println!(
                "{}",
                tf(key::LANGUAGE_CONFIGURED, &[("lang", lang.to_string())])
            );
        } else {
            println!(
                "{}",
                tf(
                    key::LANGUAGE_NONE_EFFECTIVE,
                    &[("lang", active_locale().as_bcp47().to_string())]
                )
            );
        }
        println!(
            "{}",
            tf(
                key::LANGUAGE_EFFECTIVE,
                &[("lang", active_locale().as_bcp47().to_string())]
            )
        );
        Ok(())
    })();

    if let Err(err) = &result {
        let _ = audit.record(
            "config.language.show",
            AuditStatus::Failed,
            Some("config.language"),
            None,
            Some(&err.to_string()),
        );
    } else {
        let audit_id = audit.record(
            "config.language.show",
            AuditStatus::Ok,
            Some("config.language"),
            None,
            None,
        )?;
        println!("{}", tf(key::AUDIT_ID, &[("audit_id", audit_id)]));
    }
    result
}

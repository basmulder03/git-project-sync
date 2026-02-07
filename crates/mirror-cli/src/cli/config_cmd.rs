use super::*;
pub(super) fn handle_config(args: ConfigArgs, audit: &AuditLogger) -> anyhow::Result<()> {
    match args.command {
        ConfigCommands::Init(args) => handle_init(args, audit),
    }
}

pub(super) fn handle_init(args: InitArgs, audit: &AuditLogger) -> anyhow::Result<()> {
    let result: anyhow::Result<()> = (|| {
        let config_path = default_config_path()?;
        let (mut config, migrated) = load_or_migrate(&config_path)?;
        config.root = Some(args.root);
        config.save(&config_path)?;
        if migrated {
            println!("Config migrated and saved to {}", config_path.display());
        } else {
            println!("Config saved to {}", config_path.display());
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
        println!("Audit ID: {audit_id}");
    }
    result
}

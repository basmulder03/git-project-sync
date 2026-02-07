use anyhow::Context;
use keyring::Entry;
use std::time::{SystemTime, UNIX_EPOCH};

const SERVICE: &str = "git-project-sync";

pub fn get_pat(account: &str) -> anyhow::Result<String> {
    get_token(account)
}

pub fn get_token(account: &str) -> anyhow::Result<String> {
    let entry = Entry::new(SERVICE, account).context("open keyring entry")?;
    entry.get_password().context("read token from keyring")
}

pub fn set_pat(account: &str, token: &str) -> anyhow::Result<()> {
    let entry = Entry::new(SERVICE, account).context("open keyring entry")?;
    entry.set_password(token).context("write PAT to keyring")
}

pub fn delete_pat(account: &str) -> anyhow::Result<()> {
    let entry = Entry::new(SERVICE, account).context("open keyring entry")?;
    entry.delete_credential().context("delete PAT from keyring")
}

pub fn probe_keyring_roundtrip() -> anyhow::Result<()> {
    let now = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap_or_default()
        .as_nanos();
    let account = format!("diagnostic:{}:{}:{}", std::process::id(), now, SERVICE);
    let token = "probe-ok";
    set_pat(&account, token).context("write diagnostic token to keyring")?;
    let value = get_pat(&account).context("read diagnostic token from keyring")?;
    let _ = delete_pat(&account);
    if value != token {
        anyhow::bail!("keyring roundtrip token mismatch");
    }
    Ok(())
}

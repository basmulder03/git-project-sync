use anyhow::Context;
use keyring::Entry;

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

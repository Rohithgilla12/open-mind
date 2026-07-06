use keyring::Entry;
use serde::{Deserialize, Serialize};

const SERVICE: &str = "fun.gilla.openmind.dock";
const ACCOUNT_URL: &str = "instance_url";
const ACCOUNT_TOKEN: &str = "token";

#[derive(Serialize, Deserialize, Clone)]
pub struct Settings {
    pub instance_url: String,
    pub token: String,
}

fn entry(account: &str) -> Result<Entry, String> {
    Entry::new(SERVICE, account).map_err(|e| e.to_string())
}

#[tauri::command]
pub fn settings_get() -> Result<Option<Settings>, String> {
    let url = match entry(ACCOUNT_URL)?.get_password() {
        Ok(v) => v,
        Err(keyring::Error::NoEntry) => return Ok(None),
        Err(e) => return Err(e.to_string()),
    };
    let token = match entry(ACCOUNT_TOKEN)?.get_password() {
        Ok(v) => v,
        Err(keyring::Error::NoEntry) => return Ok(None),
        Err(e) => return Err(e.to_string()),
    };
    Ok(Some(Settings { instance_url: url, token }))
}

#[tauri::command]
pub fn settings_set(instance_url: String, token: String) -> Result<(), String> {
    let url = instance_url.trim().trim_end_matches('/').to_string();
    entry(ACCOUNT_URL)?.set_password(&url).map_err(|e| e.to_string())?;
    entry(ACCOUNT_TOKEN)?.set_password(token.trim()).map_err(|e| e.to_string())
}

#[tauri::command]
pub fn settings_clear() -> Result<(), String> {
    for account in [ACCOUNT_URL, ACCOUNT_TOKEN] {
        match entry(account)?.delete_credential() {
            Ok(()) | Err(keyring::Error::NoEntry) => {}
            Err(e) => return Err(e.to_string()),
        }
    }
    Ok(())
}

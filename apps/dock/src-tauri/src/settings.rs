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
        Err(keyring::Error::NoEntry) => {
            log::info!("settings_get: no instance_url entry");
            return Ok(None);
        }
        Err(e) => return Err(e.to_string()),
    };
    let token = match entry(ACCOUNT_TOKEN)?.get_password() {
        Ok(v) => v,
        Err(keyring::Error::NoEntry) => {
            log::info!("settings_get: instance_url present but no token entry");
            return Ok(None);
        }
        Err(e) => return Err(e.to_string()),
    };
    // Length and key-kind only — never the token itself. Enough to tell a
    // freshly-minted device key from a stale static token when the panel and
    // the tray disagree about whether the dock is configured.
    log::info!(
        "settings_get: ok (token len {}, device key: {})",
        token.len(),
        token.starts_with("omk_")
    );
    Ok(Some(Settings { instance_url: url, token }))
}

/// Persists settings, then refreshes the tray. The refresh is not optional
/// bookkeeping: the tray menu is only ever rebuilt on a queue mutation, a Desk
/// refresh, or a panel focus, so without it a freshly-connected dock keeps
/// showing whatever placeholder it computed at launch — the panel works while
/// the tray still says "Open Settings first", which is precisely how this was
/// reported.
#[tauri::command]
pub fn settings_set(
    app: tauri::AppHandle,
    instance_url: String,
    token: String,
) -> Result<(), String> {
    let url = instance_url.trim().trim_end_matches('/').to_string();
    log::info!("settings_set: writing url ({} chars) and token ({} chars)", url.len(), token.trim().len());
    entry(ACCOUNT_URL)?.set_password(&url).map_err(|e| e.to_string())?;
    entry(ACCOUNT_TOKEN)?.set_password(token.trim()).map_err(|e| e.to_string())?;
    // Read straight back: a write that reports success but cannot be read is
    // the failure this is chasing, and it is otherwise completely silent.
    match entry(ACCOUNT_URL).and_then(|e| e.get_password().map_err(|e| e.to_string())) {
        Ok(v) => log::info!("settings_set: read-back ok ({} chars)", v.len()),
        Err(e) => log::warn!("settings_set: WROTE BUT CANNOT READ BACK: {e}"),
    }
    // Fetches with the new credential and rebuilds the menu on completion.
    crate::refresh_desk(app);
    Ok(())
}

#[tauri::command]
pub fn settings_clear(app: tauri::AppHandle) -> Result<(), String> {
    for account in [ACCOUNT_URL, ACCOUNT_TOKEN] {
        match entry(account)?.delete_credential() {
            Ok(()) | Err(keyring::Error::NoEntry) => {}
            Err(e) => return Err(e.to_string()),
        }
    }
    // Drop the cached pins before rebuilding: keeping another account's Desk
    // titles visible in the tray after a sign-out would be a small leak, and
    // the placeholder is the honest thing to show once there is no credential.
    crate::clear_desk_cache(&app);
    Ok(())
}

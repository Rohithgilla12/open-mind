mod queue;
mod grab;
mod settings;

use serde::{Deserialize, Serialize};
use serde_json::json;
use std::fs;
use std::str::FromStr;
use std::sync::Mutex;
use std::time::Duration;
use tauri::menu::{Menu, MenuItem};
use tauri::tray::TrayIconBuilder;
use tauri::{AppHandle, Emitter, Manager};
use tauri_plugin_global_shortcut::{Code, GlobalShortcutExt, Modifiers, Shortcut, ShortcutState};
use tauri_plugin_notification::NotificationExt;

// ⌘ on macOS; Ctrl on Windows/Linux (SUPER there is the OS key — Win+Shift+S
// is the system screenshot tool).
#[cfg(target_os = "macos")]
const PRIMARY_MODIFIER: Modifiers = Modifiers::SUPER;
#[cfg(not(target_os = "macos"))]
const PRIMARY_MODIFIER: Modifiers = Modifiers::CONTROL;

// Accelerator strings for the built-in defaults, in the same "+"-joined
// format the global-shortcut plugin's `Shortcut: FromStr` parses (and the
// format `Shortcut`'s `Display` produces — see `parse_accelerator_pair`).
const DEFAULT_QUICK_SAVE_ACCEL: &str = "CmdOrCtrl+Shift+S";
const DEFAULT_QUICK_FIND_ACCEL: &str = "CmdOrCtrl+Shift+O";

fn quick_save_shortcut() -> Shortcut {
    Shortcut::new(Some(PRIMARY_MODIFIER | Modifiers::SHIFT), Code::KeyS)
}

fn toggle_panel_shortcut() -> Shortcut {
    Shortcut::new(Some(PRIMARY_MODIFIER | Modifiers::SHIFT), Code::KeyO)
}

/// Managed state: the currently-active (quick_save, quick_find) shortcuts,
/// consulted by both startup registration and the handler's matching so a
/// rebind takes effect without a restart.
type ShortcutsState = Mutex<(Shortcut, Shortcut)>;

#[derive(Serialize)]
struct ShortcutPair {
    quick_save: String,
    quick_find: String,
}

#[derive(Serialize, Deserialize)]
struct PersistedShortcuts {
    quick_save: String,
    quick_find: String,
}

/// Pure parse of the two accelerator strings, falling back per-field to the
/// built-in default on a parse failure (never lets one bad field take down
/// the other). Never logs the accelerator value itself.
fn parse_accelerator_pair(qs: &str, qf: &str) -> (Shortcut, Shortcut) {
    let quick_save = Shortcut::from_str(qs).unwrap_or_else(|_| {
        log::warn!("quick_save accelerator failed to parse — using default");
        quick_save_shortcut()
    });
    let quick_find = Shortcut::from_str(qf).unwrap_or_else(|_| {
        log::warn!("quick_find accelerator failed to parse — using default");
        toggle_panel_shortcut()
    });
    (quick_save, quick_find)
}

fn shortcuts_path(app: &AppHandle) -> Option<std::path::PathBuf> {
    app.path().app_config_dir().ok().map(|dir| dir.join("shortcuts.json"))
}

/// Loads the persisted shortcut pair from `shortcuts.json` in the app config
/// dir, falling back to the built-in defaults when the file is missing or
/// unparsable (`log::warn!`, never the raw file contents).
fn load_shortcuts(app: &AppHandle) -> (Shortcut, Shortcut) {
    let Some(path) = shortcuts_path(app) else {
        return parse_accelerator_pair(DEFAULT_QUICK_SAVE_ACCEL, DEFAULT_QUICK_FIND_ACCEL);
    };
    let contents = match fs::read_to_string(&path) {
        Ok(c) => c,
        Err(_) => {
            return parse_accelerator_pair(DEFAULT_QUICK_SAVE_ACCEL, DEFAULT_QUICK_FIND_ACCEL);
        }
    };
    match serde_json::from_str::<PersistedShortcuts>(&contents) {
        Ok(persisted) => parse_accelerator_pair(&persisted.quick_save, &persisted.quick_find),
        Err(e) => {
            log::warn!("shortcuts.json failed to parse: {e}");
            parse_accelerator_pair(DEFAULT_QUICK_SAVE_ACCEL, DEFAULT_QUICK_FIND_ACCEL)
        }
    }
}

fn write_shortcuts(app: &AppHandle, quick_save: &str, quick_find: &str) {
    let Some(path) = shortcuts_path(app) else { return };
    if let Some(dir) = path.parent() {
        let _ = fs::create_dir_all(dir);
    }
    let persisted = PersistedShortcuts {
        quick_save: quick_save.to_string(),
        quick_find: quick_find.to_string(),
    };
    if let Ok(json) = serde_json::to_string_pretty(&persisted) {
        let _ = fs::write(path, json);
    }
}

#[tauri::command]
fn get_shortcuts(state: tauri::State<ShortcutsState>) -> ShortcutPair {
    let (quick_save, quick_find) = *state.lock().unwrap();
    ShortcutPair { quick_save: quick_save.to_string(), quick_find: quick_find.to_string() }
}

/// Parses both accelerators, swaps the global registrations, and persists on
/// success. On any registration failure (e.g. the combo is already owned by
/// another app) it best-effort restores the previous pair and returns a
/// field-agnostic error — never leaves the app with no shortcuts registered.
#[tauri::command]
fn rebind_shortcuts(
    app: AppHandle,
    state: tauri::State<ShortcutsState>,
    quick_save: String,
    quick_find: String,
) -> Result<(), String> {
    let new_save = Shortcut::from_str(&quick_save).map_err(|_| INVALID_SHORTCUT_MSG.to_string())?;
    let new_find = Shortcut::from_str(&quick_find).map_err(|_| INVALID_SHORTCUT_MSG.to_string())?;

    let mut guard = state.lock().unwrap();
    let (old_save, old_find) = *guard;

    let gs = app.global_shortcut();
    let _ = gs.unregister(old_save);
    let _ = gs.unregister(old_find);

    let registered = gs.register(new_save).and_then(|()| gs.register(new_find));

    match registered {
        Ok(()) => {
            *guard = (new_save, new_find);
            drop(guard);
            write_shortcuts(&app, &quick_save, &quick_find);
            Ok(())
        }
        Err(_) => {
            let _ = gs.unregister(new_save);
            let _ = gs.unregister(new_find);
            let restore_save = gs.register(old_save);
            let restore_find = gs.register(old_find);
            if restore_save.is_err() || restore_find.is_err() {
                notify(&app, "Shortcuts unavailable — use the tray menu instead.");
            }
            Err(INVALID_SHORTCUT_MSG.to_string())
        }
    }
}

const INVALID_SHORTCUT_MSG: &str = "that combination is taken or invalid";

fn notify(app: &AppHandle, body: &str) {
    let _ = app.notification().builder().title("Openmind").body(body).show();
}

/// Truncates to at most `max` chars, appending an ellipsis when cut short.
fn truncate(s: &str, max: usize) -> String {
    if s.chars().count() <= max {
        return s.to_string();
    }
    let head: String = s.chars().take(max).collect();
    format!("{head}…")
}

fn show_panel(app: &AppHandle) {
    if let Some(window) = app.get_webview_window("panel") {
        let _ = window.show();
        let _ = window.set_focus();
    }
}

fn toggle_panel(app: &AppHandle) {
    if let Some(window) = app.get_webview_window("panel") {
        let visible = window.is_visible().unwrap_or(false);
        if visible {
            let _ = window.hide();
        } else {
            let _ = window.show();
            let _ = window.set_focus();
        }
    }
}

/// Grabs the frontmost browser tab and saves it without opening the panel,
/// falling back to notifications (and, for missing settings, the panel
/// itself) when something goes wrong. Never logs the token.
fn quick_save(app: &AppHandle) {
    let app = app.clone();
    tauri::async_runtime::spawn(async move {
        let settings = match settings::settings_get() {
            Ok(Some(s)) => s,
            Ok(None) => {
                notify(&app, "Open Openmind Dock settings first");
                show_panel(&app);
                return;
            }
            Err(e) => {
                notify(&app, &format!("Settings error: {e}"));
                return;
            }
        };

        let tab = match tauri::async_runtime::spawn_blocking(grab::grab_frontmost_tab).await {
            Ok(Ok(t)) => t,
            Ok(Err(e)) => {
                let msg = match e.as_str() {
                    "automation-denied" => {
                        "Allow automation for your browser in System Settings → Privacy"
                    }
                    "unsupported-platform" => {
                        "Tab grab is macOS-only here — use the panel shortcut to capture."
                    }
                    "unsupported-app" => "Front app isn't a supported browser",
                    "firefox-unsupported" => "Firefox doesn't allow tab access — use ⌘⇧O",
                    "no-tab" => "No tab open in the front browser",
                    _ => "Couldn't read the front tab",
                };
                notify(&app, msg);
                return;
            }
            Err(_) => {
                notify(&app, "Couldn't read the front tab");
                return;
            }
        };

        let client = match reqwest::Client::builder().timeout(Duration::from_secs(15)).build() {
            Ok(c) => c,
            Err(_) => {
                notify(&app, "Save failed (couldn't start request)");
                return;
            }
        };

        let endpoint = format!("{}/api/items", settings.instance_url);
        let response = client
            .post(&endpoint)
            .bearer_auth(&settings.token)
            .json(&json!({ "url": tab.url }))
            .send()
            .await;

        match response {
            Ok(resp) if resp.status().as_u16() == 201 => {
                let title = if tab.title.trim().is_empty() { tab.url.clone() } else { tab.title.clone() };
                notify(&app, &format!("Saved — {}", truncate(&title, 60)));

                let item_id = resp
                    .json::<serde_json::Value>()
                    .await
                    .ok()
                    .and_then(|body| body.get("id").and_then(|v| v.as_str()).map(|s| s.to_string()));

                if let Some(item_id) = item_id {
                    let _ = app.emit_to("panel", "save-confirmed", json!({ "itemId": item_id, "title": title }));
                    show_panel(&app);
                }
            }
            Ok(resp) => {
                notify(&app, &format!("Save failed ({})", resp.status().as_u16()));
            }
            Err(_) => {
                notify(&app, "Save failed (network error)");
            }
        }
    });
}

/// Registers a global shortcut without letting a conflict (another app
/// already owns the combo) take the whole app down. This is a tray-only
/// Accessory app — an unregistered shortcut should degrade to "use the tray
/// menu instead", not kill the process.
fn register_shortcut_or_warn(app: &AppHandle, shortcut: Shortcut, label: &str) {
    if let Err(e) = app.global_shortcut().register(shortcut) {
        log::debug!("failed to register shortcut {label}: {e}");
        notify(
            app,
            &format!("Couldn't register {label} — another app may be using it. The tray menu still works."),
        );
    }
}

fn build_tray(app: &tauri::App) -> tauri::Result<()> {
    let open_panel = MenuItem::with_id(app, "open-panel", "Open panel", true, None::<&str>)?;
    let save_tab = MenuItem::with_id(app, "save-tab", "Save current tab", true, None::<&str>)?;
    let settings_item = MenuItem::with_id(app, "settings", "Settings", true, None::<&str>)?;
    let quit = MenuItem::with_id(app, "quit", "Quit", true, None::<&str>)?;
    let menu = Menu::with_items(app, &[&open_panel, &save_tab, &settings_item, &quit])?;

    let mut tray = TrayIconBuilder::with_id("main").menu(&menu);
    if let Some(icon) = app.default_window_icon() {
        tray = tray.icon(icon.clone());
    }

    tray
        .on_menu_event(|app, event| match event.id().as_ref() {
            "open-panel" => show_panel(app),
            "save-tab" => quick_save(app),
            "settings" => {
                show_panel(app);
                if let Some(window) = app.get_webview_window("panel") {
                    let _ = window.emit("open-settings", ());
                }
            }
            "quit" => app.exit(0),
            _ => {}
        })
        .build(app)?;

    Ok(())
}

// Checks CrabNebula Cloud for a newer release on startup and installs it in
// the background; the running instance keeps working and the new version
// applies on next launch. Failures are silent by design — an offline start
// must not nag.
fn check_for_updates(app: tauri::AppHandle) {
    tauri::async_runtime::spawn(async move {
        use tauri_plugin_updater::UpdaterExt;
        let updater = match app.updater() {
            Ok(u) => u,
            Err(e) => {
                log::debug!("updater unavailable: {e}");
                return;
            }
        };
        match updater.check().await {
            Ok(Some(update)) => {
                let version = update.version.clone();
                match update.download_and_install(|_, _| {}, || {}).await {
                    Ok(()) => notify(
                        &app,
                        &format!("Updated to {version} — quit and reopen the dock to apply."),
                    ),
                    Err(e) => log::debug!("update install failed: {e}"),
                }
            }
            Ok(None) => {}
            Err(e) => log::debug!("update check failed: {e}"),
        }
    });
}

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    tauri::Builder::default()
        .plugin(tauri_plugin_opener::init())
        .plugin(tauri_plugin_notification::init())
        .plugin(tauri_plugin_http::init())
        .plugin(tauri_plugin_updater::Builder::new().build())
        .plugin(tauri_plugin_process::init())
        .plugin(tauri_plugin_autostart::init(
            tauri_plugin_autostart::MacosLauncher::LaunchAgent,
            None::<Vec<&'static str>>,
        ))
        .plugin(
            tauri_plugin_global_shortcut::Builder::new()
                .with_handler(|app, shortcut, event| {
                    if event.state() != ShortcutState::Pressed {
                        return;
                    }
                    let (active_save, active_find) = *app.state::<ShortcutsState>().lock().unwrap();
                    if shortcut == &active_save {
                        quick_save(app);
                    } else if shortcut == &active_find {
                        toggle_panel(app);
                    }
                })
                .build(),
        )
        .invoke_handler(tauri::generate_handler![
            settings::settings_get,
            settings::settings_set,
            settings::settings_clear,
            grab::grab_frontmost_tab,
            get_shortcuts,
            rebind_shortcuts,
        ])
        .setup(|app| {
            if cfg!(debug_assertions) {
                app.handle().plugin(
                    tauri_plugin_log::Builder::default()
                        .level(log::LevelFilter::Info)
                        .build(),
                )?;
            }
            // Background utility: no Dock icon, no app-switcher entry. The
            // panel is toggled by a global shortcut and the tray menu.
            #[cfg(target_os = "macos")]
            {
                app.set_activation_policy(tauri::ActivationPolicy::Accessory);
            }

            let (quick_save, quick_find) = load_shortcuts(app.handle());
            app.manage::<ShortcutsState>(Mutex::new((quick_save, quick_find)));
            register_shortcut_or_warn(app.handle(), quick_save, "quick save");
            register_shortcut_or_warn(app.handle(), quick_find, "quick find");

            build_tray(app)?;

            check_for_updates(app.handle().clone());

            Ok(())
        })
        .run(tauri::generate_context!())
        .expect("error while running tauri application");
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn parses_a_valid_pair() {
        let (quick_save, quick_find) = parse_accelerator_pair("CmdOrCtrl+Shift+S", "F5");
        assert_eq!(quick_save, Shortcut::new(Some(PRIMARY_MODIFIER | Modifiers::SHIFT), Code::KeyS));
        assert_eq!(quick_find, Shortcut::new(None, Code::F5));
    }

    #[test]
    fn invalid_field_falls_back_only_for_that_field() {
        let (quick_save, quick_find) = parse_accelerator_pair("not a shortcut", "CmdOrCtrl+Shift+O");
        assert_eq!(quick_save, quick_save_shortcut());
        assert_eq!(quick_find, Shortcut::new(Some(PRIMARY_MODIFIER | Modifiers::SHIFT), Code::KeyO));

        let (quick_save, quick_find) = parse_accelerator_pair("CmdOrCtrl+Shift+S", "also not a shortcut");
        assert_eq!(quick_save, Shortcut::new(Some(PRIMARY_MODIFIER | Modifiers::SHIFT), Code::KeyS));
        assert_eq!(quick_find, toggle_panel_shortcut());
    }
}

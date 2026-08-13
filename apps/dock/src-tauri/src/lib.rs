mod queue;
mod grab;
mod settings;
mod window;

use serde::{Deserialize, Serialize};
use serde_json::json;
use std::fs;
use std::str::FromStr;
use std::sync::Mutex;
use std::time::Duration;
use tauri::menu::{Menu, MenuItem, PredefinedMenuItem, Submenu};
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

/// Desk pins shown in the tray submenu.
const DESK_MENU_MAX: usize = 8;

#[derive(Clone, Debug)]
pub struct DeskEntry {
    pub id: String,
    pub title: String,
}

/// Cached Desk pins for the tray submenu. Refreshed on launch, after a
/// successful save, and on panel focus — never on a background timer. Only
/// ever overwritten by a successful fetch, so a transient failure keeps
/// serving the last-known-good pins instead of emptying the submenu.
pub type DeskState = Mutex<Vec<DeskEntry>>;

/// Reads Desk rows out of either shape: the ItemPage envelope or the bare
/// array an older instance serves. Mirrors readItemList in src/lib/api.ts.
fn parse_desk(body: &serde_json::Value) -> Vec<DeskEntry> {
    let rows = body
        .get("items")
        .and_then(|v| v.as_array())
        .or_else(|| body.as_array())
        .cloned()
        .unwrap_or_default();

    rows.iter()
        .filter_map(|row| {
            let id = row.get("id").and_then(|v| v.as_str())?.to_string();
            let title = row
                .get("title")
                .and_then(|v| v.as_str())
                .filter(|t| !t.trim().is_empty())
                .or_else(|| row.get("url").and_then(|v| v.as_str()))
                .unwrap_or("Untitled")
                .to_string();
            Some(DeskEntry { id, title })
        })
        .take(DESK_MENU_MAX)
        .collect()
}

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

/// Queues a capture and notifies accordingly — but only promises a retry
/// when the queue actually wrote it to disk. An entry that failed to persist
/// might still drain later this session, but the user must not be told it
/// is safe: it dies at quit like anything else that never made it to disk.
fn enqueue_and_notify(app: &AppHandle, url: String, ok_prefix: &str) {
    let result = queue::enqueue(app, Some(url), None);
    if result.persisted {
        notify(app, &format!("{ok_prefix} ({} pending)", queue::pending_count(app)));
    } else {
        notify(app, "Couldn't save or queue — try again");
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
                enqueue_and_notify(&app, tab.url.clone(), "Saved offline — will retry");
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
                let title = if tab.title.trim().is_empty() {
                    tab.url.clone()
                } else {
                    tab.title.clone()
                };
                notify(&app, &format!("Saved — {}", truncate(&title, 60)));
                refresh_desk(app.clone());

                let item_id = resp
                    .json::<serde_json::Value>()
                    .await
                    .ok()
                    .and_then(|body| body.get("id").and_then(|v| v.as_str()).map(|s| s.to_string()));

                if let Some(item_id) = item_id {
                    let _ = app.emit_to(
                        "panel",
                        "save-confirmed",
                        json!({ "itemId": item_id, "title": title }),
                    );
                    show_panel(&app);
                }
            }
            Ok(resp) => {
                let status = resp.status().as_u16();
                if queue::disposition(status) == queue::Disposition::Retry {
                    // Up but broken (5xx / rate limited): queue rather than
                    // discard, and say so distinctly from an offline save.
                    enqueue_and_notify(&app, tab.url.clone(), "Instance error — queued, will retry");
                } else {
                    notify(&app, &format!("Save failed ({status})"));
                }
            }
            Err(_) => {
                enqueue_and_notify(&app, tab.url.clone(), "Saved offline — will retry");
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

fn build_menu(app: &AppHandle) -> tauri::Result<Menu<tauri::Wry>> {
    let menu = Menu::new(app)?;
    menu.append(&MenuItem::with_id(app, "open-panel", "Open panel", true, None::<&str>)?)?;
    menu.append(&MenuItem::with_id(app, "save-tab", "Save current tab", true, None::<&str>)?)?;

    let desk = app.state::<DeskState>().lock().unwrap().clone();
    log::info!("build_menu: {} cached pins", desk.len());
    let submenu = Submenu::with_id(app, "desk", "Desk", true)?;
    if desk.is_empty() {
        // A disabled placeholder, never a vanishing item: a menu entry that
        // disappears reads as a bug, a greyed one explains itself. All three
        // outcomes get their own label — collapsing a keychain *error* into
        // "not configured" tells the user to re-enter settings they already
        // have, which is the one instruction guaranteed not to help.
        let label = match settings::settings_get() {
            Ok(Some(_)) => "Couldn't load Desk",
            Ok(None) => "Open Settings first",
            Err(e) => {
                // keyring::Error never carries the secret itself, so this is
                // safe to log — and it is the only trace of why the dock
                // cannot see settings that are demonstrably present.
                log::warn!("keychain read failed while building the tray menu: {e}");
                "Keychain unavailable"
            }
        };
        submenu.append(&MenuItem::with_id(app, "desk-empty", label, false, None::<&str>)?)?;
    } else {
        for entry in &desk {
            submenu.append(&MenuItem::with_id(
                app,
                format!("desk:{}", entry.id),
                truncate(&entry.title, 48),
                true,
                None::<&str>,
            )?)?;
        }
    }
    menu.append(&submenu)?;

    let pending = queue::pending_count(app);
    if pending > 0 {
        menu.append(&PredefinedMenuItem::separator(app)?)?;
        let label = if pending == 1 {
            "1 pending save".to_string()
        } else {
            format!("{pending} pending saves")
        };
        menu.append(&MenuItem::with_id(app, "pending-count", label, false, None::<&str>)?)?;
        menu.append(&MenuItem::with_id(
            app,
            "retry-pending",
            "Retry pending saves",
            true,
            None::<&str>,
        )?)?;
    }

    menu.append(&PredefinedMenuItem::separator(app)?)?;
    menu.append(&MenuItem::with_id(app, "settings", "Settings", true, None::<&str>)?)?;
    menu.append(&MenuItem::with_id(app, "quit", "Quit", true, None::<&str>)?)?;
    Ok(menu)
}

/// Rebuilds the whole tray menu. Tauri v2 has no way to mutate a menu in
/// place, so both the queue count and the Desk cache come through here.
pub fn rebuild_tray_menu(app: &AppHandle) {
    let Some(tray) = app.tray_by_id("main") else { return };
    match build_menu(app) {
        Ok(menu) => {
            if let Err(e) = tray.set_menu(Some(menu)) {
                log::warn!("tray menu update failed: {e}");
            }
        }
        Err(e) => log::warn!("tray menu build failed: {e}"),
    }
}

fn build_tray(app: &tauri::App) -> tauri::Result<()> {
    let menu = build_menu(app.handle())?;
    let mut tray = TrayIconBuilder::with_id("main").menu(&menu);
    if let Some(icon) = app.default_window_icon() {
        tray = tray.icon(icon.clone());
    }

    tray.on_menu_event(|app, event| {
        let id = event.id().as_ref().to_string();
        match id.as_str() {
            "open-panel" => show_panel(app),
            "save-tab" => quick_save(app),
            "retry-pending" => {
                let handle = app.clone();
                tauri::async_runtime::spawn(async move { queue::flush(handle).await });
            }
            "settings" => {
                show_panel(app);
                if let Some(window) = app.get_webview_window("panel") {
                    let _ = window.emit("open-settings", ());
                }
            }
            "quit" => app.exit(0),
            other => {
                if let Some(item_id) = other.strip_prefix("desk:") {
                    open_item(app, item_id);
                }
            }
        }
    })
    .build(app)?;

    Ok(())
}

/// Opens an item in the user's browser from the tray.
fn open_item(_app: &AppHandle, item_id: &str) {
    let Ok(Some(settings)) = settings::settings_get() else { return };
    let url = format!("{}/item/{}", settings.instance_url, item_id);
    if let Err(e) = tauri_plugin_opener::open_url(url, None::<&str>) {
        log::warn!("couldn't open a Desk item: {e}");
    }
}

/// Fetches Desk pins and rebuilds the tray. A failed or unconfigured fetch
/// leaves the cache untouched — the last-known-good pins keep serving the
/// submenu rather than being wiped by a transient blip. The disabled
/// placeholder is only ever seen on a cold start, before the first fetch
/// succeeds.
pub fn refresh_desk(app: AppHandle) {
    tauri::async_runtime::spawn(async move {
        let Ok(Some(settings)) = settings::settings_get() else {
            rebuild_tray_menu(&app);
            return;
        };
        let Ok(client) = reqwest::Client::builder().timeout(Duration::from_secs(10)).build() else {
            log::warn!("refresh_desk: couldn't build the HTTP client — skipping this refresh");
            rebuild_tray_menu(&app);
            return;
        };
        let endpoint = format!("{}/api/desk", settings.instance_url);
        match client.get(&endpoint).bearer_auth(&settings.token).send().await {
            Ok(resp) if resp.status().is_success() => {
                if let Ok(body) = resp.json::<serde_json::Value>().await {
                    let entries = parse_desk(&body);
                    log::info!("refresh_desk: 2xx, parsed {} pins", entries.len());
                    *app.state::<DeskState>().lock().unwrap() = entries;
                }
            }
            // Keep the last-known-good pins: a transient failure must not
            // empty the submenu the cache exists to keep serving.
            Ok(resp) => {
                log::info!("refresh_desk: HTTP {} — keeping cached pins", resp.status().as_u16());
            }
            Err(_) => {
                log::info!("refresh_desk: request failed — keeping cached pins");
            }
        }
        rebuild_tray_menu(&app);
    });
}

/// Empties the cached Desk pins and rebuilds the tray. Used on sign-out, where
/// keeping the previous account's pins on screen would be a small leak.
pub fn clear_desk_cache(app: &AppHandle) {
    app.state::<DeskState>().lock().unwrap().clear();
    rebuild_tray_menu(app);
}

/// Lets the panel refresh the Desk submenu when it regains focus — the
/// stand-in for background polling.
#[tauri::command]
fn desk_refresh(app: AppHandle) {
    refresh_desk(app);
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
            queue::queue_list,
            queue::queue_enqueue,
            queue::queue_flush,
            queue::queue_remove,
            desk_refresh,
        ])
        .setup(|app| {
            // Attached unconditionally: release builds need this too, since
            // the log line is the only field diagnosis available for the
            // stateful half of the queue (failed persists, cap eviction,
            // rename failures). Warn keeps a release build quiet; the
            // plugin's default targets already include the log directory,
            // so this is enough for a user to send us a file.
            let log_level =
                if cfg!(debug_assertions) { log::LevelFilter::Info } else { log::LevelFilter::Warn };
            app.handle().plugin(tauri_plugin_log::Builder::default().level(log_level).build())?;
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

            let pending = queue::load(app.handle());
            let had_pending = !pending.is_empty();
            app.manage::<queue::QueueState>(Mutex::new(pending));
            app.manage::<DeskState>(Mutex::new(Vec::new()));

            build_tray(app)?;

            if let Some(panel) = app.get_webview_window("panel") {
                window::restore(&panel);
                let handle = panel.clone();
                panel.on_window_event(move |event| {
                    use tauri::WindowEvent;
                    // Only geometry changes are interesting; ignore focus and
                    // visibility churn, which fire constantly.
                    if !matches!(event, WindowEvent::Moved(_) | WindowEvent::Resized(_)) {
                        return;
                    }
                    // outer_position/outer_size report physical pixels;
                    // window.json (and the constants it's clamped against)
                    // are logical, so convert with this window's own scale
                    // factor before recording.
                    let (Ok(pos), Ok(size), Ok(scale)) =
                        (handle.outer_position(), handle.outer_size(), handle.scale_factor())
                    else {
                        return;
                    };
                    let pos = pos.to_logical::<i32>(scale);
                    let size = size.to_logical::<u32>(scale);
                    window::record(window::Rect {
                        x: pos.x,
                        y: pos.y,
                        width: size.width,
                        height: size.height,
                    });
                });
            }
            window::spawn_persister(app.handle().clone());

            check_for_updates(app.handle().clone());

            // Drain anything left over from the last session, then keep a slow
            // retry loop alive for the rest of this one.
            if had_pending {
                let handle = app.handle().clone();
                tauri::async_runtime::spawn(async move { queue::flush(handle).await });
            }
            queue::spawn_drainer(app.handle().clone());
            refresh_desk(app.handle().clone());

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

    #[test]
    fn desk_entries_read_both_list_shapes() {
        let envelope = r#"{"items":[{"id":"1","title":"One"},{"id":"2","title":"Two"}]}"#;
        let bare = r#"[{"id":"1","title":"One"},{"id":"2","title":"Two"}]"#;
        for raw in [envelope, bare] {
            let entries = parse_desk(&serde_json::from_str(raw).unwrap());
            assert_eq!(entries.len(), 2, "{raw}");
            assert_eq!(entries[0].id, "1");
            assert_eq!(entries[0].title, "One");
        }
    }

    #[test]
    fn desk_entries_fall_back_to_the_url_when_untitled() {
        let raw = r#"[{"id":"1","url":"https://www.example.com/a"}]"#;
        let entries = parse_desk(&serde_json::from_str(raw).unwrap());
        assert_eq!(entries[0].title, "https://www.example.com/a");
    }

    #[test]
    fn desk_entries_ignore_rows_without_an_id() {
        let raw = r#"[{"title":"no id"},{"id":"2","title":"Two"}]"#;
        let entries = parse_desk(&serde_json::from_str(raw).unwrap());
        assert_eq!(entries.len(), 1);
        assert_eq!(entries[0].id, "2");
    }

    #[test]
    fn desk_entries_cap_at_eight() {
        let rows: Vec<String> = (0..20)
            .map(|i| format!(r#"{{"id":"{i}","title":"T{i}"}}"#))
            .collect();
        let raw = format!("[{}]", rows.join(","));
        let entries = parse_desk(&serde_json::from_str(&raw).unwrap());
        assert_eq!(entries.len(), DESK_MENU_MAX);
    }

    /// The panel is `decorations: false`, so the only way to move it is the
    /// drag strip, which works by `data-tauri-drag-region`. Tauri's injected
    /// drag script turns that into `invoke("plugin:window|start_dragging")`,
    /// and that command is **not** part of `core:default` — `core:window`'s
    /// default permission set is entirely read-only commands. Without an
    /// explicit grant the ACL rejects the call and the window simply never
    /// moves, with nothing logged anywhere: the rejection lands in the webview
    /// console, not the Rust log. That silence is why this shipped broken from
    /// the initial release, so it gets a guard rather than a comment.
    #[test]
    fn capabilities_grant_every_window_command_the_panel_uses() {
        // `core:window`'s default permission set is 28 commands and every one is
        // read-only, so each mutating command the webview calls needs granting
        // by name. Miss one and the ACL rejects the call with nothing logged —
        // the rejection lands in the webview console, not the Rust log. That
        // silence is why dragging never worked from the initial release, and why
        // hide() failed at all five of its call sites (Esc, opening an item, the
        // confirm strip's auto-hide, the close button) without a single symptom
        // beyond "the button does nothing".
        let capability = include_str!("../capabilities/default.json");
        for permission in [
            "core:window:allow-start-dragging",
            "core:window:allow-hide",
            "core:window:allow-show",
            "core:window:allow-set-focus",
        ] {
            assert!(
                capability.contains(permission),
                "missing {permission} — the matching window call will be silently denied"
            );
        }
    }
}

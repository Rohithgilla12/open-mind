mod grab;
mod settings;

use serde_json::json;
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

fn quick_save_shortcut() -> Shortcut {
    Shortcut::new(Some(PRIMARY_MODIFIER | Modifiers::SHIFT), Code::KeyS)
}

fn toggle_panel_shortcut() -> Shortcut {
    Shortcut::new(Some(PRIMARY_MODIFIER | Modifiers::SHIFT), Code::KeyO)
}

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
                notify(&app, &format!("Saved — {}", truncate(&tab.title, 60)));
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
                    if shortcut == &quick_save_shortcut() {
                        quick_save(app);
                    } else if shortcut == &toggle_panel_shortcut() {
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

            register_shortcut_or_warn(app.handle(), quick_save_shortcut(), "⌘⇧S");
            register_shortcut_or_warn(app.handle(), toggle_panel_shortcut(), "⌘⇧O");

            build_tray(app)?;

            check_for_updates(app.handle().clone());

            Ok(())
        })
        .run(tauri::generate_context!())
        .expect("error while running tauri application");
}

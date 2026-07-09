use serde::Serialize;
use std::process::Command;

#[derive(Serialize, Clone)]
pub struct TabInfo {
    pub url: String,
    pub title: String,
    pub browser: String,
}

// Separator unlikely to appear in titles; AppleScript returns "url<SEP>title".
const SEP: &str = "\u{241E}"; // ␞ symbol-for-record-separator

fn osascript(script: &str) -> Result<String, String> {
    let out = Command::new("osascript")
        .arg("-e")
        .arg(script)
        .output()
        .map_err(|e| format!("running osascript: {e}"))?;
    if !out.status.success() {
        let err = String::from_utf8_lossy(&out.stderr);
        if err.contains("-1743") || err.to_lowercase().contains("not authorized") {
            return Err("automation-denied".into());
        }
        return Err(format!("script failed: {}", err.trim()));
    }
    Ok(String::from_utf8_lossy(&out.stdout).trim().to_string())
}

pub fn frontmost_bundle_id() -> Result<String, String> {
    osascript(r#"tell application "System Events" to get bundle identifier of first process whose frontmost is true"#)
}

// Maps a bundle id to (display name, tab-grab script). Chromium browsers share
// one scripting dictionary shape; Safari differs. Firefox exposes no URL.
pub fn script_for(bundle_id: &str) -> Option<(&'static str, String)> {
    let chromium = |app: &str| {
        format!(
            r#"tell application "{app}" to set o to (URL of active tab of front window) & "{SEP}" & (title of active tab of front window)
o"#
        )
    };
    match bundle_id {
        "com.apple.Safari" => Some((
            "Safari",
            format!(
                r#"tell application "Safari" to set o to (URL of front document) & "{SEP}" & (name of front document)
o"#
            ),
        )),
        "com.google.Chrome" => Some(("Google Chrome", chromium("Google Chrome"))),
        "com.brave.Browser" => Some(("Brave Browser", chromium("Brave Browser"))),
        "com.microsoft.edgemac" => Some(("Microsoft Edge", chromium("Microsoft Edge"))),
        "company.thebrowser.Browser" => Some(("Arc", chromium("Arc"))),
        _ => None,
    }
}

pub fn parse_output(raw: &str, browser: &str) -> Result<TabInfo, String> {
    let (url, title) = raw.split_once(SEP).ok_or("unexpected script output")?;
    if !url.starts_with("http") {
        return Err("no-tab".into());
    }
    Ok(TabInfo { url: url.to_string(), title: title.to_string(), browser: browser.to_string() })
}

#[tauri::command]
pub fn grab_frontmost_tab() -> Result<TabInfo, String> {
    #[cfg(not(target_os = "macos"))]
    return Err("unsupported-platform".into());
    #[cfg(target_os = "macos")]
    grab_frontmost_tab_macos()
}

#[cfg(target_os = "macos")]
fn grab_frontmost_tab_macos() -> Result<TabInfo, String> {
    let bundle = frontmost_bundle_id()?;
    if bundle == "org.mozilla.firefox" {
        return Err("firefox-unsupported".into());
    }
    let (name, script) = script_for(&bundle).ok_or("unsupported-app")?;
    let raw = osascript(&script)?;
    parse_output(&raw, name)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn chromium_bundles_map() {
        for id in ["com.google.Chrome", "com.brave.Browser", "com.microsoft.edgemac", "company.thebrowser.Browser", "com.apple.Safari"] {
            assert!(script_for(id).is_some(), "{id}");
        }
        assert!(script_for("com.spotify.client").is_none());
    }

    #[test]
    fn parse_happy_and_sad() {
        let raw = format!("https://example.com{SEP}Example Title");
        let t = parse_output(&raw, "Safari").unwrap();
        assert_eq!(t.url, "https://example.com");
        assert_eq!(t.title, "Example Title");
        assert!(parse_output("garbage", "Safari").is_err());
        assert!(parse_output(&format!("about:blank{SEP}x"), "Safari").is_err());
    }
}

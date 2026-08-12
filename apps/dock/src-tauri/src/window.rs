//! Panel geometry: remembered across restarts, and clamped so a position
//! saved on a since-disconnected display cannot come back offscreen.
//!
//! Everything in this module — the constants, `Rect`, the on-disk
//! `window.json` — is in **logical** pixels, matching `tauri.conf.json`'s
//! window dimensions. The Tauri APIs that touch monitors and window frames
//! (`Monitor::work_area`, `outer_position`/`outer_size`) all report
//! **physical** pixels, so callers convert at the boundary with that
//! monitor's or window's own `scale_factor()` before anything reaches
//! `Rect`.
use serde::{Deserialize, Serialize};
use std::fs;
use std::path::PathBuf;
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::Mutex;
use std::time::Duration;
use tauri::{AppHandle, LogicalPosition, LogicalSize, Manager};

/// Logical pixels, matching `tauri.conf.json`'s `minWidth`/`minHeight`.
pub const MIN_W: u32 = 520;
pub const MIN_H: u32 = 360;
/// Logical pixels, matching `tauri.conf.json`'s `width`/`height`.
pub const DEFAULT_W: u32 = 640;
pub const DEFAULT_H: u32 = 420;

/// How much of the panel must fall inside a monitor for a saved position to
/// be considered usable.
const MIN_VISIBLE: i32 = 80;

#[derive(Clone, Copy, Debug, PartialEq, Serialize, Deserialize)]
pub struct Rect {
    pub x: i32,
    pub y: i32,
    pub width: u32,
    pub height: u32,
}

/// Overlap of two rects as (width, height); either may be negative when they
/// do not intersect on that axis.
fn overlap(a: Rect, b: Rect) -> (i32, i32) {
    let w = (a.x + a.width as i32).min(b.x + b.width as i32) - a.x.max(b.x);
    let h = (a.y + a.height as i32).min(b.y + b.height as i32) - a.y.max(b.y);
    (w, h)
}

fn centre_on(monitor: Rect, width: u32, height: u32) -> Rect {
    let w = width.min(monitor.width);
    let h = height.min(monitor.height);
    Rect {
        x: monitor.x + ((monitor.width - w) / 2) as i32,
        y: monitor.y + ((monitor.height - h) / 2) as i32,
        width: w,
        height: h,
    }
}

/// Fits a remembered rect to the monitors that actually exist. A rect with
/// less than MIN_VISIBLE on both axes inside every monitor is treated as
/// offscreen and recentred on the primary (`monitors[0]`).
pub fn clamp_rect(saved: Rect, monitors: &[Rect]) -> Rect {
    if monitors.is_empty() {
        return saved;
    }
    let mut r = saved;
    r.width = r.width.max(MIN_W);
    r.height = r.height.max(MIN_H);

    let target = monitors.iter().copied().find(|m| {
        let (w, h) = overlap(r, *m);
        w >= MIN_VISIBLE && h >= MIN_VISIBLE
    });

    let Some(m) = target else {
        return centre_on(monitors[0], r.width, r.height);
    };

    r.width = r.width.min(m.width);
    r.height = r.height.min(m.height);
    r.x = r.x.min(m.x + m.width as i32 - r.width as i32).max(m.x);
    r.y = r.y.min(m.y + m.height as i32 - r.height as i32).max(m.y);
    r
}

fn window_path(app: &AppHandle) -> Option<PathBuf> {
    app.path().app_config_dir().ok().map(|dir| dir.join("window.json"))
}

fn read_saved(app: &AppHandle) -> Option<Rect> {
    let path = window_path(app)?;
    let raw = fs::read_to_string(path).ok()?;
    match serde_json::from_str::<Rect>(&raw) {
        Ok(r) => Some(r),
        Err(e) => {
            // window.json holds only geometry — unlike the queue file, a
            // serde error here can't quote back a saved URL, so it is safe
            // to interpolate directly.
            log::warn!("window.json failed to parse: {e}");
            None
        }
    }
}

fn write_saved(app: &AppHandle, rect: Rect) {
    let Some(path) = window_path(app) else { return };
    if let Some(dir) = path.parent() {
        let _ = fs::create_dir_all(dir);
    }
    if let Ok(json) = serde_json::to_string_pretty(&rect) {
        let _ = fs::write(path, json);
    }
}

/// Pending geometry awaiting a debounced write.
static DIRTY: AtomicBool = AtomicBool::new(false);
static PENDING: Mutex<Option<Rect>> = Mutex::new(None);

/// Records new geometry. Called on every move and resize event, so it only
/// marks state dirty — `spawn_persister` does the writing.
pub fn record(rect: Rect) {
    *PENDING.lock().unwrap() = Some(rect);
    DIRTY.store(true, Ordering::Relaxed);
}

/// Flushes pending geometry to disk at most twice a second, so a drag does
/// not hammer the filesystem.
pub fn spawn_persister(app: AppHandle) {
    std::thread::spawn(move || loop {
        std::thread::sleep(Duration::from_millis(500));
        if !DIRTY.swap(false, Ordering::Relaxed) {
            continue;
        }
        let pending = *PENDING.lock().unwrap();
        if let Some(rect) = pending {
            write_saved(&app, rect);
        }
    });
}

/// Applies the remembered geometry, or centres at the default size on first
/// run. Monitor rects come back from Tauri in physical pixels, so each is
/// converted to logical with **that monitor's own** scale factor before
/// `clamp_rect` sees it — mixed-DPI setups can have a different factor per
/// display. The work area (rather than the full monitor bounds) is used so a
/// clamp cannot park the panel with its top under the macOS menu bar.
pub fn restore(window: &tauri::WebviewWindow) {
    let app = window.app_handle();
    let monitors: Vec<Rect> = window
        .available_monitors()
        .unwrap_or_default()
        .iter()
        .map(|m| {
            let scale = m.scale_factor();
            let work_area = m.work_area();
            let position = work_area.position.to_logical::<i32>(scale);
            let size = work_area.size.to_logical::<u32>(scale);
            Rect { x: position.x, y: position.y, width: size.width, height: size.height }
        })
        .collect();

    let saved = read_saved(app).unwrap_or_else(|| {
        let primary = monitors.first().copied().unwrap_or(Rect {
            x: 0,
            y: 0,
            width: DEFAULT_W,
            height: DEFAULT_H,
        });
        centre_on(primary, DEFAULT_W, DEFAULT_H)
    });

    let rect = clamp_rect(saved, &monitors);
    let _ = window.set_size(LogicalSize::new(rect.width, rect.height));
    let _ = window.set_position(LogicalPosition::new(rect.x, rect.y));
}

#[cfg(test)]
mod tests {
    use super::*;

    /// A single 1920x1080 display at the origin.
    fn one_screen() -> Vec<Rect> {
        vec![Rect { x: 0, y: 0, width: 1920, height: 1080 }]
    }

    /// Laptop at the origin plus an external display to its right.
    fn two_screens() -> Vec<Rect> {
        vec![
            Rect { x: 0, y: 0, width: 1440, height: 900 },
            Rect { x: 1440, y: 0, width: 2560, height: 1440 },
        ]
    }

    #[test]
    fn a_fully_visible_rect_passes_through_untouched() {
        let saved = Rect { x: 100, y: 100, width: 640, height: 420 };
        assert_eq!(clamp_rect(saved, &one_screen()), saved);
    }

    #[test]
    fn a_rect_on_the_second_display_is_kept_there() {
        let saved = Rect { x: 1600, y: 200, width: 640, height: 420 };
        assert_eq!(clamp_rect(saved, &two_screens()), saved);
    }

    #[test]
    fn a_rect_from_a_disconnected_display_recentres_on_the_primary() {
        // Saved on the external monitor, which is now gone.
        let saved = Rect { x: 2600, y: 300, width: 640, height: 420 };
        let got = clamp_rect(saved, &one_screen());
        assert_eq!(got.width, 640);
        assert_eq!(got.height, 420);
        assert_eq!(got.x, (1920 - 640) / 2);
        assert_eq!(got.y, (1080 - 420) / 2);
    }

    #[test]
    fn a_barely_overlapping_rect_counts_as_offscreen() {
        // Only 40px of the panel is on-screen: less than MIN_VISIBLE.
        let saved = Rect { x: 1880, y: 100, width: 640, height: 420 };
        let got = clamp_rect(saved, &one_screen());
        assert_eq!(got.x, (1920 - 640) / 2, "should have been recentred");
    }

    #[test]
    fn an_oversized_rect_shrinks_to_the_monitor() {
        let saved = Rect { x: 0, y: 0, width: 4000, height: 3000 };
        let got = clamp_rect(saved, &one_screen());
        assert_eq!(got.width, 1920);
        assert_eq!(got.height, 1080);
    }

    #[test]
    fn an_oversized_rect_positioned_offscreen_recentres_without_underflow() {
        // Both larger than the monitor *and* positioned where it overlaps
        // no monitor by MIN_VISIBLE: this is the only combination that
        // reaches `centre_on` with a width/height bigger than the monitor,
        // which is exactly the case `centre_on`'s `.min()` clamp-before-
        // subtract guards against a u32 underflow panic in a debug build.
        let saved = Rect { x: 5000, y: 5000, width: 4000, height: 3000 };
        let got = clamp_rect(saved, &one_screen());
        assert_eq!(got.width, 1920);
        assert_eq!(got.height, 1080);
        assert_eq!(got.x, 0);
        assert_eq!(got.y, 0);
    }

    #[test]
    fn a_rect_hanging_off_the_edge_is_nudged_fully_inside() {
        let saved = Rect { x: 1500, y: 900, width: 640, height: 420 };
        let got = clamp_rect(saved, &one_screen());
        assert_eq!(got.x, 1920 - 640);
        assert_eq!(got.y, 1080 - 420);
    }

    #[test]
    fn sizes_below_the_minimum_are_raised() {
        let saved = Rect { x: 10, y: 10, width: 200, height: 100 };
        let got = clamp_rect(saved, &one_screen());
        assert_eq!(got.width, MIN_W);
        assert_eq!(got.height, MIN_H);
    }

    #[test]
    fn no_monitors_returns_the_saved_rect_unchanged() {
        let saved = Rect { x: 5, y: 5, width: 640, height: 420 };
        assert_eq!(clamp_rect(saved, &[]), saved);
    }
}

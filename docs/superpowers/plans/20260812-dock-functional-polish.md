# Dock Functional Polish Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give the Openmind dock a durable offline save queue, a tray Desk submenu, window size/position memory, and a wider set of supported browsers.

**Architecture:** The queue is Rust-owned — a JSON file in the app config dir fronted by a `Mutex<Vec<QueuedCapture>>` in Tauri managed state, with delivery policy expressed as a pure `disposition(status)` function so it is unit-testable without HTTP. The panel reads it over Tauri commands and a `queue-changed` event. Tray menus are rebuilt wholesale by one helper that both the queue and the Desk cache call. Window placement is a pure `clamp_rect(saved, monitors)` so multi-monitor restore is testable without a display.

**Tech Stack:** Rust (Tauri v2, reqwest, serde), TypeScript/React (Vite), vitest, `cargo test`.

**Spec:** `docs/superpowers/specs/20260811-dock-functional-polish-design.md`

## Global Constraints

- **No new dependencies.** Not `uuid`, not `tokio`, not `tempfile`. Ids come from a millisecond clock plus an `AtomicU64`; intervals come from `std::thread::sleep` inside `std::thread::spawn`. (CLAUDE.md: standard library first; justify every new dependency.)
- **No capability changes.** `apps/dock/src-tauri/capabilities/default.json` already grants `opener:default` and `http:default`; do not edit it.
- **Never log a token, a raw response body, or queue entry contents.** `log::warn!` counts and status codes only. `last_error` holds only the fixed display strings produced by `error_label`.
- **Serde boundary is camelCase** for every new type crossing into TS (`#[serde(rename_all = "camelCase")]`), so the TS side needs no field remapping. Do not follow `settings.rs`, which predates this rule and is remapped by hand in `settings.ts`.
- **Queue policy must match `apps/mobile/lib/capture-queue.ts` exactly:** cap 100 oldest-dropped, URL dedupe returning the existing id, notes never dedupe, oldest-first flush, `401` stops the pass, permanent 4xx drops, transient bumps `attempts` and stops the pass.
- **UK English with the Oxford comma** in all user-facing copy and comments.
- **Comment style:** no `// ===== Section =====` banner comments anywhere.
- **All new pure logic gets tests.** Rust in-file `#[cfg(test)] mod tests`; TS as a sibling `*.test.ts`.
- Run `cd apps/dock/src-tauri && cargo test` and `pnpm --filter dock test` before each commit.

## Task order and why it is sequential

Tasks 3, 4, and 5 all modify `src-tauri/src/lib.rs`. Do **not** parallelise them across worktrees — they converge on one file and will conflict. Task 1 (`grab.rs`) and Task 2 (pure `queue.rs`) are genuinely independent and may run in parallel with each other.

---

### Task 1: Widen browser support in the tab grab

**Files:**
- Modify: `apps/dock/src-tauri/src/grab.rs:36-57` (`script_for`), `apps/dock/src-tauri/src/grab.rs:86-107` (tests)

**Interfaces:**
- Consumes: nothing.
- Produces: nothing new. `script_for(&str) -> Option<(&'static str, String)>` keeps its signature.

- [ ] **Step 1: Write the failing test**

Replace the existing `chromium_bundles_map` test in `apps/dock/src-tauri/src/grab.rs` with:

```rust
    #[test]
    fn known_bundles_map_and_unknown_does_not() {
        for id in [
            "com.apple.Safari",
            "com.apple.SafariTechnologyPreview",
            "com.kagi.kagimacOS",
            "com.google.Chrome",
            "com.google.Chrome.beta",
            "com.google.Chrome.dev",
            "com.google.Chrome.canary",
            "com.brave.Browser",
            "com.microsoft.edgemac",
            "company.thebrowser.Browser",
            "com.vivaldi.Vivaldi",
            "com.operasoftware.Opera",
            "org.chromium.Chromium",
        ] {
            assert!(script_for(id).is_some(), "{id}");
        }
        assert!(script_for("com.spotify.client").is_none());
    }

    #[test]
    fn safari_shaped_browsers_script_their_own_app_name() {
        let (name, script) = script_for("com.kagi.kagimacOS").unwrap();
        assert_eq!(name, "Orion");
        assert!(script.contains(r#"tell application "Orion""#), "{script}");
        assert!(script.contains("front document"), "{script}");
    }

    #[test]
    fn chromium_shaped_browsers_script_their_own_app_name() {
        let (name, script) = script_for("com.vivaldi.Vivaldi").unwrap();
        assert_eq!(name, "Vivaldi");
        assert!(script.contains(r#"tell application "Vivaldi""#), "{script}");
        assert!(script.contains("active tab of front window"), "{script}");
    }
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd apps/dock/src-tauri && cargo test grab`
Expected: FAIL — `script_for("com.apple.SafariTechnologyPreview")` returns `None`, so the first assert trips with that id.

- [ ] **Step 3: Write minimal implementation**

In `apps/dock/src-tauri/src/grab.rs`, replace the body of `script_for` (lines 36-57). Note the Safari branch becomes a closure exactly like `chromium`, because three browsers now share its shape:

```rust
pub fn script_for(bundle_id: &str) -> Option<(&'static str, String)> {
    let chromium = |app: &str| {
        format!(
            r#"tell application "{app}" to set o to (URL of active tab of front window) & "{SEP}" & (title of active tab of front window)
o"#
        )
    };
    let safari = |app: &str| {
        format!(
            r#"tell application "{app}" to set o to (URL of front document) & "{SEP}" & (name of front document)
o"#
        )
    };
    match bundle_id {
        "com.apple.Safari" => Some(("Safari", safari("Safari"))),
        "com.apple.SafariTechnologyPreview" => {
            Some(("Safari Technology Preview", safari("Safari Technology Preview")))
        }
        // Orion is WebKit and mirrors Safari's scripting dictionary. Unverified
        // against a real install — see the README caveat.
        "com.kagi.kagimacOS" => Some(("Orion", safari("Orion"))),
        "com.google.Chrome" => Some(("Google Chrome", chromium("Google Chrome"))),
        "com.google.Chrome.beta" => Some(("Google Chrome Beta", chromium("Google Chrome Beta"))),
        "com.google.Chrome.dev" => Some(("Google Chrome Dev", chromium("Google Chrome Dev"))),
        "com.google.Chrome.canary" => {
            Some(("Google Chrome Canary", chromium("Google Chrome Canary")))
        }
        "com.brave.Browser" => Some(("Brave Browser", chromium("Brave Browser"))),
        "com.microsoft.edgemac" => Some(("Microsoft Edge", chromium("Microsoft Edge"))),
        "company.thebrowser.Browser" => Some(("Arc", chromium("Arc"))),
        "com.vivaldi.Vivaldi" => Some(("Vivaldi", chromium("Vivaldi"))),
        "com.operasoftware.Opera" => Some(("Opera", chromium("Opera"))),
        "org.chromium.Chromium" => Some(("Chromium", chromium("Chromium"))),
        _ => None,
    }
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd apps/dock/src-tauri && cargo test`
Expected: PASS — all three grab tests plus the two pre-existing accelerator tests.

- [ ] **Step 5: Commit**

```bash
git add apps/dock/src-tauri/src/grab.rs
git commit -m "feat(dock): recognise eight more browsers in the tab grab"
```

---

### Task 2: Queue data model and delivery policy (pure)

No I/O, no Tauri, no HTTP. This task is the whole policy, fully tested, before anything is wired up.

**Files:**
- Create: `apps/dock/src-tauri/src/queue.rs`
- Modify: `apps/dock/src-tauri/src/lib.rs:1-2` (add `mod queue;`)

**Interfaces:**
- Consumes: nothing.
- Produces, all used by Task 3 and Task 4:
  - `pub struct QueuedCapture { pub id: String, pub url: Option<String>, pub note: Option<String>, pub created_at: i64, pub attempts: u32, pub last_error: Option<String> }`
  - `pub struct InsertResult { pub id: String, pub deduped: bool, pub dropped: usize }`
  - `pub enum Disposition { Delivered, DropPermanent, StopUnauthorized, Retry }`
  - `pub fn disposition(status: u16) -> Disposition`
  - `pub fn error_label(status: u16) -> String`
  - `pub fn insert(items: &mut Vec<QueuedCapture>, entry: QueuedCapture) -> InsertResult`
  - `pub fn parse_queue(raw: &str) -> Vec<QueuedCapture>`
  - `pub const MAX_QUEUE: usize = 100;`

**Note on a deliberate refinement to the spec:** the spec said the flush tests would need "the HTTP call behind a small trait or function pointer so a fake can return canned statuses". Expressing the policy as a pure `disposition(status)` achieves the same coverage with less machinery, and matches the existing pure-module style (`accelerator.rs`, `save-confirm.ts`). No trait is needed.

- [ ] **Step 1: Write the failing test**

Create `apps/dock/src-tauri/src/queue.rs` containing **only** this test module for now:

```rust
#[cfg(test)]
mod tests {
    use super::*;

    fn entry(id: &str, url: Option<&str>, created_at: i64) -> QueuedCapture {
        QueuedCapture {
            id: id.to_string(),
            url: url.map(|u| u.to_string()),
            note: None,
            created_at,
            attempts: 0,
            last_error: None,
        }
    }

    #[test]
    fn disposition_table() {
        assert_eq!(disposition(201), Disposition::Delivered);
        assert_eq!(disposition(200), Disposition::Delivered);
        assert_eq!(disposition(401), Disposition::StopUnauthorized);
        assert_eq!(disposition(400), Disposition::DropPermanent);
        assert_eq!(disposition(404), Disposition::DropPermanent);
        assert_eq!(disposition(422), Disposition::DropPermanent);
        assert_eq!(disposition(429), Disposition::Retry);
        assert_eq!(disposition(500), Disposition::Retry);
        assert_eq!(disposition(503), Disposition::Retry);
        // 0 is how both clients encode a network failure or timeout.
        assert_eq!(disposition(0), Disposition::Retry);
    }

    #[test]
    fn error_labels_never_leak_a_body() {
        assert_eq!(error_label(0), "Instance unreachable");
        assert_eq!(error_label(429), "Rate limited");
        assert_eq!(error_label(503), "Instance error (503)");
    }

    #[test]
    fn insert_appends_and_reports_the_new_id() {
        let mut items = vec![];
        let r = insert(&mut items, entry("a", Some("https://one.example"), 1));
        assert_eq!(r, InsertResult { id: "a".into(), deduped: false, dropped: 0 });
        assert_eq!(items.len(), 1);
    }

    #[test]
    fn insert_dedupes_a_pending_url_and_returns_the_existing_id() {
        let mut items = vec![entry("a", Some("https://one.example"), 1)];
        let r = insert(&mut items, entry("b", Some("https://one.example"), 2));
        assert_eq!(r, InsertResult { id: "a".into(), deduped: true, dropped: 0 });
        assert_eq!(items.len(), 1, "the duplicate must not be stored");
    }

    #[test]
    fn insert_never_dedupes_notes() {
        let mut a = entry("a", None, 1);
        a.note = Some("same text".into());
        let mut b = entry("b", None, 2);
        b.note = Some("same text".into());
        let mut items = vec![a];
        let r = insert(&mut items, b);
        assert!(!r.deduped, "two notes are two genuine saves");
        assert_eq!(items.len(), 2);
    }

    #[test]
    fn insert_drops_the_oldest_past_the_cap() {
        let mut items: Vec<QueuedCapture> = (0..MAX_QUEUE)
            .map(|i| entry(&format!("id{i}"), Some(&format!("https://{i}.example")), i as i64))
            .collect();
        let r = insert(&mut items, entry("new", Some("https://new.example"), 9_999));
        assert_eq!(r.dropped, 1);
        assert_eq!(items.len(), MAX_QUEUE);
        assert_eq!(items[0].id, "id1", "id0 was the oldest and must be gone");
        assert_eq!(items[MAX_QUEUE - 1].id, "new");
    }

    #[test]
    fn parse_queue_round_trips_camel_case() {
        let raw = r#"[{"id":"a","url":"https://one.example","createdAt":42,"attempts":3,"lastError":"Instance unreachable"}]"#;
        let items = parse_queue(raw);
        assert_eq!(items.len(), 1);
        assert_eq!(items[0].created_at, 42);
        assert_eq!(items[0].attempts, 3);
        assert_eq!(items[0].last_error.as_deref(), Some("Instance unreachable"));
        assert_eq!(items[0].note, None);
    }

    #[test]
    fn parse_queue_treats_corruption_as_empty() {
        assert!(parse_queue("").is_empty());
        assert!(parse_queue("{not json").is_empty());
        assert!(parse_queue(r#"{"id":"a"}"#).is_empty(), "an object is not an array");
        // Truncated by a crash mid-write.
        assert!(parse_queue(r#"[{"id":"a","createdAt":1,"att"#).is_empty());
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Add `mod queue;` as the first line of `apps/dock/src-tauri/src/lib.rs` (before `mod grab;`).

Run: `cd apps/dock/src-tauri && cargo test queue`
Expected: FAIL to **compile** — `cannot find type QueuedCapture in this scope`, and likewise for `disposition`, `insert`, `parse_queue`, `MAX_QUEUE`.

- [ ] **Step 3: Write minimal implementation**

Prepend to `apps/dock/src-tauri/src/queue.rs`, above the test module:

```rust
//! Durable offline capture queue. A save that fails for a transient reason
//! lands here and is retried later, so a capture is never lost to a flaky
//! network. Policy mirrors apps/mobile/lib/capture-queue.ts so both clients
//! behave identically.
use serde::{Deserialize, Serialize};

/// Cap on stored captures. Past this, the oldest are dropped.
pub const MAX_QUEUE: usize = 100;

#[derive(Clone, Debug, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct QueuedCapture {
    pub id: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub url: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub note: Option<String>,
    pub created_at: i64,
    pub attempts: u32,
    /// Display text for the panel strip. Never a raw body, never the token.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub last_error: Option<String>,
}

#[derive(Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct InsertResult {
    pub id: String,
    pub deduped: bool,
    pub dropped: usize,
}

/// What to do with an entry after one delivery attempt.
#[derive(Clone, Copy, Debug, PartialEq)]
pub enum Disposition {
    /// Delivered — remove the entry.
    Delivered,
    /// Will never succeed — remove it, and keep flushing the rest so one
    /// poison entry cannot block the queue.
    DropPermanent,
    /// Bad token. Stop the pass with the queue untouched: retrying every
    /// entry against a rejected token would burn the whole queue.
    StopUnauthorized,
    /// Transient — bump attempts, record the error, stop the pass.
    Retry,
}

/// Maps an HTTP status to a disposition. `0` means network failure or
/// timeout, the same encoding the TS client uses.
pub fn disposition(status: u16) -> Disposition {
    match status {
        401 => Disposition::StopUnauthorized,
        429 => Disposition::Retry,
        s if (200..300).contains(&s) => Disposition::Delivered,
        s if (400..500).contains(&s) => Disposition::DropPermanent,
        _ => Disposition::Retry,
    }
}

/// Fixed, body-free display text for `last_error`.
pub fn error_label(status: u16) -> String {
    match status {
        0 => "Instance unreachable".to_string(),
        429 => "Rate limited".to_string(),
        s => format!("Instance error ({s})"),
    }
}

/// Appends an entry, deduping against an already-pending identical URL and
/// evicting the oldest past `MAX_QUEUE`. Notes never dedupe.
pub fn insert(items: &mut Vec<QueuedCapture>, entry: QueuedCapture) -> InsertResult {
    if let Some(url) = entry.url.as_deref() {
        if let Some(existing) = items.iter().find(|q| q.url.as_deref() == Some(url)) {
            return InsertResult { id: existing.id.clone(), deduped: true, dropped: 0 };
        }
    }
    let id = entry.id.clone();
    items.push(entry);
    let mut dropped = 0;
    if items.len() > MAX_QUEUE {
        dropped = items.len() - MAX_QUEUE;
        items.drain(0..dropped);
    }
    InsertResult { id, deduped: false, dropped }
}

/// Reads the persisted queue. Anything unparsable — including a file
/// truncated by a crash mid-write — is treated as empty, with a warning that
/// never includes the file contents.
pub fn parse_queue(raw: &str) -> Vec<QueuedCapture> {
    if raw.trim().is_empty() {
        return Vec::new();
    }
    match serde_json::from_str::<Vec<QueuedCapture>>(raw) {
        Ok(items) => items,
        Err(e) => {
            // Never interpolate `{e}` here: serde_json's Display quotes the
            // offending value on a type mismatch, which would put a fragment
            // of a queued URL or note in the log. classify/line/column carry
            // no document content.
            log::warn!(
                "queue.json failed to parse ({:?} at line {}, column {}; {} bytes) — treating the queue as empty",
                e.classify(),
                e.line(),
                e.column(),
                raw.len()
            );
            Vec::new()
        }
    }
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd apps/dock/src-tauri && cargo test`
Expected: PASS — 8 new queue tests, plus grab and accelerator tests.

- [ ] **Step 5: Commit**

```bash
git add apps/dock/src-tauri/src/queue.rs apps/dock/src-tauri/src/lib.rs
git commit -m "feat(dock): offline queue data model and delivery policy"
```

---

### Task 3: Persist and drain the queue, and rebuild the tray around it

This task covers both the queue's stateful half and the whole tray menu. They
are one task because `queue.rs` notifies the tray on every mutation and the tray
reads the queue for its pending count — splitting them would mean shipping a
temporary no-op stub for `rebuild_tray_menu`, i.e. mandated dead code.

It is the largest task in the plan (roughly 400 lines of Rust across two files).
Work through the steps in order; each has its own test or build gate.

**Files:**
- Modify: `apps/dock/src-tauri/src/queue.rs` (append the stateful half)
- Modify: `apps/dock/src-tauri/src/lib.rs` (managed state, commands, `quick_save` arms, drainer, Desk cache, tray rebuild)

**Interfaces:**
- Consumes from Task 2: `QueuedCapture`, `InsertResult`, `Disposition`, `disposition`, `error_label`, `insert`, `parse_queue`, `MAX_QUEUE`.
- Produces, used by Tasks 5, 6, and 7:
  - `pub type QueueState = std::sync::Mutex<Vec<QueuedCapture>>`
  - `pub fn load(app: &AppHandle) -> Vec<QueuedCapture>`
  - `pub fn pending_count(app: &AppHandle) -> usize`
  - `pub fn enqueue(app: &AppHandle, url: Option<String>, note: Option<String>) -> InsertResult`
  - `pub async fn flush(app: AppHandle)`
  - `pub fn rebuild_tray_menu(app: &AppHandle)`
  - `pub type DeskState = std::sync::Mutex<Vec<DeskEntry>>` where `pub struct DeskEntry { pub id: String, pub title: String }`
  - `pub fn refresh_desk(app: AppHandle)`
  - Commands `queue_list`, `queue_enqueue`, `queue_flush`, `queue_remove`, `desk_refresh`
  - Event `queue-changed`, payload `Vec<QueuedCapture>`, emitted to the `panel` window

- [ ] **Step 1: Append the stateful half of `queue.rs`**

Insert this **above** the `#[cfg(test)]` module in `apps/dock/src-tauri/src/queue.rs`, and extend the `use` block at the top of the file to:

```rust
use serde::{Deserialize, Serialize};
use serde_json::json;
use std::fs;
use std::path::PathBuf;
use std::sync::atomic::{AtomicBool, AtomicU64, Ordering};
use std::sync::Mutex;
use std::time::{Duration, SystemTime, UNIX_EPOCH};
use tauri::{AppHandle, Emitter, Manager};
```

Then append:

```rust
/// In-memory authority for the queue, loaded from disk at startup. Every
/// mutation persists inside the same critical section, so the file and this
/// vector cannot disagree.
pub type QueueState = Mutex<Vec<QueuedCapture>>;

/// Guarantees a single flush pass at a time. A second caller returns
/// immediately rather than double-sending an entry.
static FLUSHING: AtomicBool = AtomicBool::new(false);
static ID_COUNTER: AtomicU64 = AtomicU64::new(0);

/// Resets `FLUSHING` however `flush` returns, including on an early exit.
struct FlushGuard;

impl Drop for FlushGuard {
    fn drop(&mut self) {
        FLUSHING.store(false, Ordering::SeqCst);
    }
}

fn now_millis() -> i64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|d| d.as_millis() as i64)
        .unwrap_or(0)
}

/// Unique within a process lifetime without pulling in a uuid dependency:
/// the clock gives cross-restart uniqueness, the counter gives it within a
/// millisecond.
fn new_id() -> String {
    let millis = now_millis();
    let n = ID_COUNTER.fetch_add(1, Ordering::Relaxed);
    format!("{millis:x}-{n:x}")
}

fn queue_path(app: &AppHandle) -> Option<PathBuf> {
    app.path().app_config_dir().ok().map(|dir| dir.join("queue.json"))
}

/// Writes to a temp file and renames, so a crash mid-write leaves the
/// previous good queue rather than a truncated one. Must be called with the
/// state lock held.
fn persist(app: &AppHandle, items: &[QueuedCapture]) {
    let Some(path) = queue_path(app) else { return };
    if let Some(dir) = path.parent() {
        let _ = fs::create_dir_all(dir);
    }
    let Ok(json) = serde_json::to_string_pretty(items) else { return };
    let tmp = path.with_extension("json.tmp");
    if fs::write(&tmp, json).is_err() {
        log::warn!("queue.json temp write failed");
        return;
    }
    if let Err(e) = fs::rename(&tmp, &path) {
        log::warn!("queue.json rename failed: {e}");
    }
}

/// Reads the persisted queue at startup.
pub fn load(app: &AppHandle) -> Vec<QueuedCapture> {
    let Some(path) = queue_path(app) else { return Vec::new() };
    match fs::read_to_string(&path) {
        Ok(raw) => parse_queue(&raw),
        Err(_) => Vec::new(),
    }
}

pub fn pending_count(app: &AppHandle) -> usize {
    app.state::<QueueState>().lock().unwrap().len()
}

/// Tells the panel and the tray that the queue changed. Call with the lock
/// released — the tray rebuild reads state itself.
fn notify_changed(app: &AppHandle, items: Vec<QueuedCapture>) {
    let _ = app.emit_to("panel", "queue-changed", &items);
    crate::rebuild_tray_menu(app);
}

/// Queues a capture. Exactly one of url/note should be set.
pub fn enqueue(app: &AppHandle, url: Option<String>, note: Option<String>) -> InsertResult {
    let (result, snapshot) = {
        let state = app.state::<QueueState>();
        let mut items = state.lock().unwrap();
        let entry = QueuedCapture {
            id: new_id(),
            url,
            note,
            created_at: now_millis(),
            attempts: 0,
            last_error: None,
        };
        let result = insert(&mut items, entry);
        if result.dropped > 0 {
            log::warn!("queue cap {MAX_QUEUE} exceeded; dropped {} oldest", result.dropped);
        }
        persist(app, &items);
        (result, items.clone())
    };
    notify_changed(app, snapshot);
    result
}

async fn post_capture(
    client: &reqwest::Client,
    settings: &crate::settings::Settings,
    entry: &QueuedCapture,
) -> u16 {
    let body = match (&entry.url, &entry.note) {
        (Some(url), _) => json!({ "url": url }),
        (None, Some(note)) => json!({ "note": note }),
        (None, None) => return 400, // nothing to send: drop as permanent
    };
    let endpoint = format!("{}/api/items", settings.instance_url);
    match client.post(&endpoint).bearer_auth(&settings.token).json(&body).send().await {
        Ok(resp) => resp.status().as_u16(),
        Err(_) => 0,
    }
}

/// Delivers queued captures oldest-first. The state lock is held only for
/// short synchronous sections, never across the HTTP await — so an enqueue
/// arriving mid-flush cannot be clobbered.
pub async fn flush(app: AppHandle) {
    if FLUSHING.swap(true, Ordering::SeqCst) {
        return;
    }
    let _guard = FlushGuard;

    let settings = match crate::settings::settings_get() {
        Ok(Some(s)) => s,
        // Unconfigured or keychain error: leave the queue alone rather than
        // burning attempts against nothing.
        _ => return,
    };
    let Ok(client) = reqwest::Client::builder().timeout(Duration::from_secs(15)).build() else {
        return;
    };

    loop {
        let next = {
            let items = app.state::<QueueState>().lock().unwrap();
            items.iter().min_by_key(|q| q.created_at).cloned()
        };
        let Some(entry) = next else { break };

        let status = post_capture(&client, &settings, &entry).await;
        let disp = disposition(status);

        let snapshot = {
            let state = app.state::<QueueState>();
            let mut items = state.lock().unwrap();
            match disp {
                Disposition::Delivered => items.retain(|q| q.id != entry.id),
                Disposition::DropPermanent => {
                    log::warn!("dropping a queued capture after permanent HTTP {status}");
                    items.retain(|q| q.id != entry.id);
                }
                Disposition::StopUnauthorized => {}
                Disposition::Retry => {
                    if let Some(q) = items.iter_mut().find(|q| q.id == entry.id) {
                        q.attempts += 1;
                        q.last_error = Some(error_label(status));
                    }
                }
            }
            persist(app, &items);
            items.clone()
        };
        notify_changed(&app, snapshot);

        if matches!(disp, Disposition::StopUnauthorized | Disposition::Retry) {
            break;
        }
    }
}

#[tauri::command]
pub fn queue_list(state: tauri::State<QueueState>) -> Vec<QueuedCapture> {
    state.lock().unwrap().clone()
}

#[tauri::command]
pub fn queue_enqueue(app: AppHandle, url: Option<String>, note: Option<String>) -> InsertResult {
    enqueue(&app, url, note)
}

#[tauri::command]
pub async fn queue_flush(app: AppHandle) {
    flush(app).await;
}

#[tauri::command]
pub fn queue_remove(app: AppHandle, id: String) {
    let snapshot = {
        let state = app.state::<QueueState>();
        let mut items = state.lock().unwrap();
        items.retain(|q| q.id != id);
        persist(&app, &items);
        items.clone()
    };
    notify_changed(&app, snapshot);
}

/// Retries the queue every 60s, idling when it is empty so an idle dock makes
/// no network calls. A plain thread rather than a tokio interval — tokio is in
/// the tree via tauri but is not a direct dependency.
pub fn spawn_drainer(app: AppHandle) {
    std::thread::spawn(move || loop {
        std::thread::sleep(Duration::from_secs(60));
        if pending_count(&app) == 0 {
            continue;
        }
        let handle = app.clone();
        tauri::async_runtime::spawn(async move { flush(handle).await });
    });
}
```

At this point `queue.rs` calls `crate::rebuild_tray_menu`, which does not exist
yet — the crate will not compile until Step 4 adds it. That is expected; do not
add a stub, and do not run `cargo test` until Step 5.

**Steps 2 to 4 build the tray half.** `parse_desk` is the only part with real
logic risk, because `/api/desk` may return either the `ItemPage` envelope or the
bare array an older instance serves, so it gets tests first.

- [ ] **Step 2: Write the failing Desk tests**

Add to the `#[cfg(test)] mod tests` at the bottom of `apps/dock/src-tauri/src/lib.rs`:

```rust
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
```

- [ ] **Step 3: Confirm the tests cannot yet pass**

Run: `cd apps/dock/src-tauri && cargo test desk`
Expected: FAIL to compile — `cannot find function parse_desk`, `cannot find value DESK_MENU_MAX`, and `cannot find function rebuild_tray_menu` from Step 1.

- [ ] **Step 4: Add the Desk cache and the tray menu**

In `apps/dock/src-tauri/src/lib.rs`, extend the menu imports:

```rust
use tauri::menu::{Menu, MenuItem, PredefinedMenuItem, Submenu};
```

Add near the other constants:

```rust
/// Desk pins shown in the tray submenu.
const DESK_MENU_MAX: usize = 8;

#[derive(Clone, Debug)]
pub struct DeskEntry {
    pub id: String,
    pub title: String,
}

/// Cached Desk pins for the tray submenu. Refreshed on launch, after a
/// successful save, and on panel focus — never on a background timer.
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
```

Then replace `build_tray` (currently lines 291-319) with the menu builder, the
rebuild, the new `build_tray`, the item opener, and the Desk refresh:

```rust
fn build_menu(app: &AppHandle) -> tauri::Result<Menu<tauri::Wry>> {
    let menu = Menu::new(app)?;
    menu.append(&MenuItem::with_id(app, "open-panel", "Open panel", true, None::<&str>)?)?;
    menu.append(&MenuItem::with_id(app, "save-tab", "Save current tab", true, None::<&str>)?)?;

    let desk = app.state::<DeskState>().lock().unwrap().clone();
    let submenu = Submenu::with_id(app, "desk", "Desk", true)?;
    if desk.is_empty() {
        // A disabled placeholder, never a vanishing item: a menu entry that
        // disappears reads as a bug, a greyed one explains itself.
        let configured = settings::settings_get().ok().flatten().is_some();
        let label = if configured { "Couldn't load Desk" } else { "Open Settings first" };
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
fn open_item(app: &AppHandle, item_id: &str) {
    let Ok(Some(settings)) = settings::settings_get() else { return };
    let url = format!("{}/item/{}", settings.instance_url, item_id);
    if let Err(e) = tauri_plugin_opener::open_url(url, None::<&str>) {
        log::warn!("couldn't open a Desk item: {e}");
    }
}

/// Fetches Desk pins and rebuilds the tray. Silent on failure — the submenu
/// falls back to its disabled placeholder.
pub fn refresh_desk(app: AppHandle) {
    tauri::async_runtime::spawn(async move {
        let Ok(Some(settings)) = settings::settings_get() else {
            rebuild_tray_menu(&app);
            return;
        };
        let Ok(client) = reqwest::Client::builder().timeout(Duration::from_secs(10)).build() else {
            return;
        };
        let endpoint = format!("{}/api/desk", settings.instance_url);
        match client.get(&endpoint).bearer_auth(&settings.token).send().await {
            Ok(resp) if resp.status().is_success() => {
                if let Ok(body) = resp.json::<serde_json::Value>().await {
                    // Single statement on purpose: the guard must drop before
                    // rebuild_tray_menu locks DeskState again.
                    *app.state::<DeskState>().lock().unwrap() = parse_desk(&body);
                }
            }
            // Keep the last-known-good pins. A transient failure must not empty
            // the submenu the cache exists to keep serving — the disabled
            // placeholder is for a cold start, not for going offline.
            _ => {}
        }
        rebuild_tray_menu(&app);
    });
}

/// Lets the panel refresh the Desk submenu when it regains focus — the
/// stand-in for background polling.
#[tauri::command]
pub fn desk_refresh(app: AppHandle) {
    refresh_desk(app);
}
```

- [ ] **Step 5: Wire `lib.rs`**

In `apps/dock/src-tauri/src/lib.rs`:

a. Replace `quick_save`'s response `match` (currently lines 251-273) with:

```rust
        match response {
            Ok(resp) if resp.status().as_u16() == 201 => {
                let title = if tab.title.trim().is_empty() {
                    tab.url.clone()
                } else {
                    tab.title.clone()
                };
                notify(&app, &format!("Saved — {}", truncate(&title, 60)));

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
                    queue::enqueue(&app, Some(tab.url.clone()), None);
                    notify(
                        &app,
                        &format!(
                            "Instance error — queued, will retry ({} pending)",
                            queue::pending_count(&app)
                        ),
                    );
                } else {
                    notify(&app, &format!("Save failed ({status})"));
                }
            }
            Err(_) => {
                queue::enqueue(&app, Some(tab.url.clone()), None);
                notify(
                    &app,
                    &format!(
                        "Saved offline — will retry ({} pending)",
                        queue::pending_count(&app)
                    ),
                );
            }
        }
```

b. In `quick_save`, inside the `201` arm immediately after the `notify` call, refresh the Desk cache so a just-pinned item can appear:

```rust
                refresh_desk(app.clone());
```

c. Register the commands — extend the `tauri::generate_handler!` list with:

```rust
            queue::queue_list,
            queue::queue_enqueue,
            queue::queue_flush,
            queue::queue_remove,
            desk_refresh,
```

d. In `setup()`, immediately after the `register_shortcut_or_warn` calls and **before** `build_tray(app)?` — the menu builder reads both of these states, so both must be managed first:

```rust
            let pending = queue::load(app.handle());
            let had_pending = !pending.is_empty();
            app.manage::<queue::QueueState>(Mutex::new(pending));
            app.manage::<DeskState>(Mutex::new(Vec::new()));
```

e. In `setup()`, after `check_for_updates(...)`:

```rust
            // Drain anything left over from the last session, then keep a slow
            // retry loop alive for the rest of this one.
            if had_pending {
                let handle = app.handle().clone();
                tauri::async_runtime::spawn(async move { queue::flush(handle).await });
            }
            queue::spawn_drainer(app.handle().clone());
            refresh_desk(app.handle().clone());
```

- [ ] **Step 6: Run tests and a build**

Run: `cd apps/dock/src-tauri && cargo test && cargo build`
Expected: PASS — the 8 queue tests from Task 2, the 4 new desk tests, grab and accelerator tests. Clean build with no unused-item warnings.

- [ ] **Step 7: Verify the tray by hand before committing**

`tray.set_menu()` replaces the menu that `TrayIconBuilder` was given, but `on_menu_event` is registered on the *tray icon*, not on the menu. Run `pnpm exec tauri dev`, trigger a rebuild (save something, or queue one), then click a Desk item and confirm it still opens. If the handler stops firing after a rebuild, move the whole `match` into an `app.on_menu_event(...)` call inside `setup()`; the arms are unchanged.

- [ ] **Step 8: Commit**

```bash
git add apps/dock/src-tauri/src/queue.rs apps/dock/src-tauri/src/lib.rs
git commit -m "feat(dock): persist and drain failed saves, and rebuild the tray around the queue

quick_save no longer discards a capture on a network error. A network
failure queues it as 'Saved offline'; a 5xx or 429 queues it as
'Instance error' so an up-but-broken instance is distinguishable. The
tray gains a Desk submenu and a pending-save count, both served by one
rebuild helper."
```

---

### Task 4: MERGED INTO TASK 3 — do not implement

The tray work below was originally its own task. It is now Steps 2, 4, 5b, 5d and
5e of Task 3, because splitting it would have required shipping a temporary no-op
`rebuild_tray_menu` stub — mandated dead code. **Skip this section entirely; it is
retained only so the step numbering of Tasks 5-9 stays stable.**

<details>
<summary>Superseded original Task 4 (do not implement)</summary>

**Files:**
- Modify: `apps/dock/src-tauri/src/lib.rs` (replace `build_tray`, replace the `rebuild_tray_menu` stub, add the Desk cache)

**Interfaces:**
- Consumes from Task 3: `queue::QueueState`, `queue::pending_count`, `queue::flush`.
- Produces:
  - `pub fn rebuild_tray_menu(app: &AppHandle)` — replaces the Task 3 stub
  - `pub type DeskState = std::sync::Mutex<Vec<DeskEntry>>` where `pub struct DeskEntry { pub id: String, pub title: String }`
  - `pub fn refresh_desk(app: AppHandle)` — spawns the fetch, then rebuilds the menu
  - Command `desk_refresh`, so the panel can trigger a refresh on focus (Task 7)

- [ ] **Step 1: Write the failing test**

The menu itself needs a running app, but the list-shape parsing does not — and that is where the real risk is, because `/api/desk` may return either the `ItemPage` envelope or a bare array (mirroring `readItemList` in `src/lib/api.ts`). Add to the `#[cfg(test)] mod tests` at the bottom of `lib.rs`:

```rust
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd apps/dock/src-tauri && cargo test desk`
Expected: FAIL to compile — `cannot find function parse_desk`, `cannot find value DESK_MENU_MAX`.

- [ ] **Step 3: Write minimal implementation**

In `apps/dock/src-tauri/src/lib.rs`, extend the imports:

```rust
use tauri::menu::{Menu, MenuItem, PredefinedMenuItem, Submenu};
```

Add near the other constants:

```rust
/// Desk pins shown in the tray submenu.
const DESK_MENU_MAX: usize = 8;

#[derive(Clone, Debug)]
pub struct DeskEntry {
    pub id: String,
    pub title: String,
}

/// Cached Desk pins for the tray submenu. Refreshed on launch, after a
/// successful save, and on panel focus — never on a background timer.
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
```

Replace `build_tray` (lines 291-319) with a builder plus a rebuild, sharing one menu construction:

```rust
fn build_menu(app: &AppHandle) -> tauri::Result<Menu<tauri::Wry>> {
    let menu = Menu::new(app)?;
    menu.append(&MenuItem::with_id(app, "open-panel", "Open panel", true, None::<&str>)?)?;
    menu.append(&MenuItem::with_id(app, "save-tab", "Save current tab", true, None::<&str>)?)?;

    let desk = app.state::<DeskState>().lock().unwrap().clone();
    let submenu = Submenu::with_id(app, "desk", "Desk", true)?;
    if desk.is_empty() {
        // A disabled placeholder, never a vanishing item: a menu entry that
        // disappears reads as a bug, a greyed one explains itself.
        let configured = settings::settings_get().ok().flatten().is_some();
        let label = if configured { "Couldn't load Desk" } else { "Open Settings first" };
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
fn open_item(app: &AppHandle, item_id: &str) {
    let Ok(Some(settings)) = settings::settings_get() else { return };
    let url = format!("{}/item/{}", settings.instance_url, item_id);
    if let Err(e) = tauri_plugin_opener::open_url(url, None::<&str>) {
        log::warn!("couldn't open a Desk item: {e}");
    }
}

/// Fetches Desk pins and rebuilds the tray. Silent on failure — the submenu
/// falls back to its disabled placeholder.
pub fn refresh_desk(app: AppHandle) {
    tauri::async_runtime::spawn(async move {
        let Ok(Some(settings)) = settings::settings_get() else {
            rebuild_tray_menu(&app);
            return;
        };
        let Ok(client) = reqwest::Client::builder().timeout(Duration::from_secs(10)).build() else {
            return;
        };
        let endpoint = format!("{}/api/desk", settings.instance_url);
        match client.get(&endpoint).bearer_auth(&settings.token).send().await {
            Ok(resp) if resp.status().is_success() => {
                if let Ok(body) = resp.json::<serde_json::Value>().await {
                    // Single statement on purpose: the guard must drop before
                    // rebuild_tray_menu locks DeskState again.
                    *app.state::<DeskState>().lock().unwrap() = parse_desk(&body);
                }
            }
            // Keep the last-known-good pins. A transient failure must not empty
            // the submenu the cache exists to keep serving — the disabled
            // placeholder is for a cold start, not for going offline.
            _ => {}
        }
        rebuild_tray_menu(&app);
    });
}

/// Lets the panel refresh the Desk submenu when it regains focus — the spec's
/// stand-in for background polling.
#[tauri::command]
pub fn desk_refresh(app: AppHandle) {
    refresh_desk(app);
}
```

Register it alongside the queue commands in `tauri::generate_handler!`:

```rust
            desk_refresh,
```

- [ ] **Step 4: Wire the Desk state and its refresh triggers**

In `setup()`, **before** `build_tray(app)?` (the menu builder reads both states):

```rust
            app.manage::<DeskState>(Mutex::new(Vec::new()));
```

After `build_tray(app)?`:

```rust
            refresh_desk(app.handle().clone());
```

In `quick_save`, inside the `201` arm after the `notify` call, add:

```rust
                refresh_desk(app.clone());
```

Remove the temporary `rebuild_tray_menu` stub added in Task 3.

- [ ] **Step 5: Run tests and build**

Run: `cd apps/dock/src-tauri && cargo test && cargo build`
Expected: PASS, 4 new desk tests.

**Verify by hand before committing** — `tray.set_menu()` replaces the menu built by `TrayIconBuilder`, and the `on_menu_event` handler is registered on the *tray icon*, not the menu. Run `pnpm exec tauri dev`, save something to force a rebuild, then click a Desk item and confirm it still opens. If the handler no longer fires after a rebuild, move the whole `match` into an `app.on_menu_event(...)` call inside `setup()` instead; the arms are unchanged.

- [ ] **Step 6: Commit**

```bash
git add apps/dock/src-tauri/src/lib.rs
git commit -m "feat(dock): tray Desk submenu and pending-save items"
```

</details>

---

### Task 5: Window size and position memory

**Files:**
- Create: `apps/dock/src-tauri/src/window.rs`
- Modify: `apps/dock/src-tauri/src/lib.rs` (`mod window;`, restore on setup, persist on move/resize)
- Modify: `apps/dock/src-tauri/tauri.conf.json:12-24`
- Modify: `apps/dock/src/panel/Panel.tsx` (`styles.shell`)

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces:
  - `pub struct Rect { pub x: i32, pub y: i32, pub width: u32, pub height: u32 }`
  - `pub fn clamp_rect(saved: Rect, monitors: &[Rect]) -> Rect`
  - `pub fn restore(window: &tauri::WebviewWindow)`
  - `pub fn spawn_persister(app: AppHandle)`
  - `pub const MIN_W: u32 = 520; pub const MIN_H: u32 = 360;`

- [ ] **Step 1: Write the failing test**

Create `apps/dock/src-tauri/src/window.rs` with only this test module:

```rust
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
```

- [ ] **Step 2: Run test to verify it fails**

Add `mod window;` to the top of `apps/dock/src-tauri/src/lib.rs`.

Run: `cd apps/dock/src-tauri && cargo test window`
Expected: FAIL to compile — `cannot find type Rect`, `cannot find function clamp_rect`, `cannot find value MIN_W`.

- [ ] **Step 3: Write minimal implementation**

Prepend to `apps/dock/src-tauri/src/window.rs`:

```rust
//! Panel geometry: remembered across restarts, and clamped so a position
//! saved on a since-disconnected display cannot come back offscreen.
use serde::{Deserialize, Serialize};
use std::fs;
use std::path::PathBuf;
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::Mutex;
use std::time::Duration;
use tauri::{AppHandle, Manager, PhysicalPosition, PhysicalSize};

pub const MIN_W: u32 = 520;
pub const MIN_H: u32 = 360;
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
/// run. Uses physical pixels throughout, which is what both the monitor
/// query and the setters report.
pub fn restore(window: &tauri::WebviewWindow) {
    let app = window.app_handle();
    let monitors: Vec<Rect> = window
        .available_monitors()
        .unwrap_or_default()
        .iter()
        .map(|m| Rect {
            x: m.position().x,
            y: m.position().y,
            width: m.size().width,
            height: m.size().height,
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
    let _ = window.set_size(PhysicalSize::new(rect.width, rect.height));
    let _ = window.set_position(PhysicalPosition::new(rect.x, rect.y));
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd apps/dock/src-tauri && cargo test window`
Expected: PASS — 8 window tests.

- [ ] **Step 5: Wire it into `lib.rs`**

In `setup()`, after the tray is built:

```rust
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
                    let (Ok(pos), Ok(size)) = (handle.outer_position(), handle.outer_size()) else {
                        return;
                    };
                    window::record(window::Rect {
                        x: pos.x,
                        y: pos.y,
                        width: size.width,
                        height: size.height,
                    });
                });
            }
            window::spawn_persister(app.handle().clone());
```

- [ ] **Step 6: Make the window resizable and the shell fluid**

In `apps/dock/src-tauri/tauri.conf.json`, replace the `windows[0]` object with:

```json
      {
        "label": "panel",
        "title": "Openmind",
        "width": 640,
        "height": 420,
        "minWidth": 520,
        "minHeight": 360,
        "visible": false,
        "decorations": false,
        "alwaysOnTop": true,
        "skipTaskbar": true,
        "resizable": true,
        "transparent": true
      }
```

`center` is gone on purpose — `window::restore` owns placement now.

In `apps/dock/src/panel/Panel.tsx`, `styles.shell` currently hardcodes the same dimensions a second time. Change its first two properties from `width: 640, height: 420` to:

```ts
    width: "100%",
    height: "100vh",
```

- [ ] **Step 7: Verify by hand**

Run `pnpm exec tauri dev`. Confirm: the panel can be dragged by its strip and resized from an edge; the content reflows rather than clipping; quitting and relaunching restores the same size and position.

- [ ] **Step 8: Commit**

```bash
git add apps/dock/src-tauri/src/window.rs apps/dock/src-tauri/src/lib.rs \
        apps/dock/src-tauri/tauri.conf.json apps/dock/src/panel/Panel.tsx
git commit -m "feat(dock): resizable panel with clamped size and position memory"
```

---

### Task 6: TypeScript queue client and pure strip logic

**Files:**
- Create: `apps/dock/src/lib/queue.ts`, `apps/dock/src/lib/queue.test.ts`
- Create: `apps/dock/src/lib/pending-summary.ts`, `apps/dock/src/lib/pending-summary.test.ts`
- Create: `apps/dock/src/lib/url.ts`
- Modify: `apps/dock/src/panel/Panel.tsx` (import `host` from `lib/url` instead of defining it)

**Interfaces:**
- Consumes from Task 3: commands `queue_list`, `queue_enqueue`, `queue_flush`, `queue_remove`; event `queue-changed`.
- Produces, used by Task 7:
  - `type QueuedCapture = { id: string; url?: string; note?: string; createdAt: number; attempts: number; lastError?: string }`
  - `listQueue(): Promise<QueuedCapture[]>`
  - `enqueueCapture(input: { url?: string; note?: string }): Promise<{ id: string; deduped: boolean; dropped: number }>`
  - `flushQueue(): Promise<void>`
  - `removeQueued(id: string): Promise<void>`
  - `subscribeQueue(cb: (items: QueuedCapture[]) => void): Promise<() => void>`
  - `pendingSummary(items: QueuedCapture[]): { label: string; stuck: boolean }`
  - `entryLabel(entry: QueuedCapture): string`
  - `relativeAge(createdAt: number, now: number): string`
  - `host(url: string): string` (moved out of `Panel.tsx`)

- [ ] **Step 1: Write the failing tests**

Create `apps/dock/src/lib/pending-summary.test.ts`:

```ts
import { describe, expect, it } from "vitest";
import { entryLabel, pendingSummary, relativeAge, STUCK_ATTEMPTS } from "./pending-summary";
import type { QueuedCapture } from "./queue";

function entry(over: Partial<QueuedCapture> = {}): QueuedCapture {
  return { id: "a", createdAt: 0, attempts: 0, ...over };
}

describe("pendingSummary", () => {
  it("singularises one save", () => {
    expect(pendingSummary([entry()]).label).toBe("1 save waiting to sync");
  });

  it("pluralises more than one", () => {
    expect(pendingSummary([entry({ id: "a" }), entry({ id: "b" })]).label).toBe(
      "2 saves waiting to sync",
    );
  });

  it("is not stuck while attempts stay under the threshold", () => {
    expect(pendingSummary([entry({ attempts: STUCK_ATTEMPTS - 1 })]).stuck).toBe(false);
  });

  it("is stuck once any entry reaches the threshold", () => {
    expect(pendingSummary([entry({ attempts: STUCK_ATTEMPTS })]).stuck).toBe(true);
  });
});

describe("entryLabel", () => {
  it("shows a bare hostname for a URL", () => {
    expect(entryLabel(entry({ url: "https://www.example.com/deep/path" }))).toBe("example.com");
  });

  it("excerpts a note", () => {
    const note = "a".repeat(80);
    const label = entryLabel(entry({ note }));
    expect(label.length).toBeLessThanOrEqual(49);
    expect(label.endsWith("…")).toBe(true);
  });

  it("leaves a short note intact", () => {
    expect(entryLabel(entry({ note: "short note" }))).toBe("short note");
  });

  it("falls back for an entry with neither", () => {
    expect(entryLabel(entry())).toBe("Untitled save");
  });
});

describe("relativeAge", () => {
  const now = 1_000_000_000_000;

  it("reads just now under a minute", () => {
    expect(relativeAge(now - 30_000, now)).toBe("just now");
  });

  it("reads minutes", () => {
    expect(relativeAge(now - 5 * 60_000, now)).toBe("5m ago");
  });

  it("reads hours", () => {
    expect(relativeAge(now - 3 * 3_600_000, now)).toBe("3h ago");
  });

  it("reads days", () => {
    expect(relativeAge(now - 2 * 86_400_000, now)).toBe("2d ago");
  });

  it("never reads negative for a clock skewed into the future", () => {
    expect(relativeAge(now + 60_000, now)).toBe("just now");
  });
});
```

Create `apps/dock/src/lib/queue.test.ts`:

```ts
import { beforeEach, describe, expect, it, vi } from "vitest";

const invokeMock = vi.fn();
const listenMock = vi.fn();

vi.mock("@tauri-apps/api/core", () => ({ invoke: (...a: unknown[]) => invokeMock(...a) }));
vi.mock("@tauri-apps/api/event", () => ({ listen: (...a: unknown[]) => listenMock(...a) }));

import { enqueueCapture, flushQueue, listQueue, removeQueued, subscribeQueue } from "./queue";

describe("queue client", () => {
  beforeEach(() => {
    invokeMock.mockReset();
    listenMock.mockReset();
  });

  it("lists via queue_list", async () => {
    invokeMock.mockResolvedValueOnce([]);
    expect(await listQueue()).toEqual([]);
    expect(invokeMock).toHaveBeenCalledWith("queue_list");
  });

  it("returns an empty list when the command fails", async () => {
    invokeMock.mockRejectedValueOnce(new Error("no state"));
    expect(await listQueue()).toEqual([]);
  });

  it("passes url and note through to queue_enqueue", async () => {
    invokeMock.mockResolvedValueOnce({ id: "x", deduped: false, dropped: 0 });
    await enqueueCapture({ url: "https://example.com" });
    expect(invokeMock).toHaveBeenCalledWith("queue_enqueue", {
      url: "https://example.com",
      note: undefined,
    });
  });

  it("flushes via queue_flush and swallows a failure", async () => {
    invokeMock.mockRejectedValueOnce(new Error("offline"));
    await expect(flushQueue()).resolves.toBeUndefined();
    expect(invokeMock).toHaveBeenCalledWith("queue_flush");
  });

  it("removes by id", async () => {
    invokeMock.mockResolvedValueOnce(undefined);
    await removeQueued("abc");
    expect(invokeMock).toHaveBeenCalledWith("queue_remove", { id: "abc" });
  });

  it("subscribes to queue-changed and hands the payload to the callback", async () => {
    let fire: ((e: { payload: unknown }) => void) | undefined;
    listenMock.mockImplementation((_name: string, cb: (e: { payload: unknown }) => void) => {
      fire = cb;
      return Promise.resolve(() => {});
    });
    const seen: unknown[] = [];
    await subscribeQueue((items) => seen.push(items));
    expect(listenMock.mock.calls[0][0]).toBe("queue-changed");
    fire?.({ payload: [{ id: "a", createdAt: 1, attempts: 0 }] });
    expect(seen).toEqual([[{ id: "a", createdAt: 1, attempts: 0 }]]);
  });
});
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `pnpm --filter dock test`
Expected: FAIL — cannot resolve `./queue` or `./pending-summary`.

- [ ] **Step 3: Write the implementations**

Create `apps/dock/src/lib/url.ts`:

```ts
/** Best-effort hostname for display; falls back to the raw string. */
export function host(url: string): string {
  try {
    return new URL(url).hostname.replace(/^www\./, "");
  } catch {
    return url;
  }
}
```

Create `apps/dock/src/lib/queue.ts`:

```ts
// Thin wrappers over the Rust queue_* commands. The queue itself lives in
// Rust (src-tauri/src/queue.rs) because a ⌘⇧S save that fails does so there,
// with no webview involved.
import { invoke } from "@tauri-apps/api/core";
import { listen } from "@tauri-apps/api/event";

export type QueuedCapture = {
  id: string;
  url?: string;
  note?: string;
  createdAt: number;
  attempts: number;
  lastError?: string;
};

export type EnqueueResult = { id: string; deduped: boolean; dropped: number };

/** Current queue. Returns [] rather than throwing — the strip is chrome and
 *  must never take the panel down with it. */
export async function listQueue(): Promise<QueuedCapture[]> {
  try {
    return await invoke<QueuedCapture[]>("queue_list");
  } catch {
    return [];
  }
}

export function enqueueCapture(input: { url?: string; note?: string }): Promise<EnqueueResult> {
  return invoke<EnqueueResult>("queue_enqueue", { url: input.url, note: input.note });
}

/** Asks Rust to retry now. Failures are ignored: the periodic drainer will
 *  try again regardless. */
export async function flushQueue(): Promise<void> {
  try {
    await invoke("queue_flush");
  } catch {
    // Ignored by design.
  }
}

export async function removeQueued(id: string): Promise<void> {
  try {
    await invoke("queue_remove", { id });
  } catch {
    // Ignored by design.
  }
}

/** Subscribes to queue mutations emitted by Rust. Resolves to an unlisten fn. */
export function subscribeQueue(
  cb: (items: QueuedCapture[]) => void,
): Promise<() => void> {
  return listen<QueuedCapture[]>("queue-changed", (event) => cb(event.payload));
}
```

Create `apps/dock/src/lib/pending-summary.ts`:

```ts
// Pure display logic for the pending-saves strip, kept out of the component
// so it can be tested the way save-confirm.ts and home-lists.ts are.
import type { QueuedCapture } from "./queue";
import { host } from "./url";

/** Attempts at which an entry stops reading as "pending" and starts reading
 *  as "stuck" — the strip switches to the danger colour here. */
export const STUCK_ATTEMPTS = 5;

const NOTE_EXCERPT = 48;

export function pendingSummary(items: QueuedCapture[]): { label: string; stuck: boolean } {
  const label =
    items.length === 1 ? "1 save waiting to sync" : `${items.length} saves waiting to sync`;
  return { label, stuck: items.some((q) => q.attempts >= STUCK_ATTEMPTS) };
}

export function entryLabel(entry: QueuedCapture): string {
  if (entry.url) return host(entry.url);
  const note = entry.note?.trim();
  if (!note) return "Untitled save";
  return note.length > NOTE_EXCERPT ? `${note.slice(0, NOTE_EXCERPT)}…` : note;
}

export function relativeAge(createdAt: number, now: number): string {
  const ms = Math.max(0, now - createdAt);
  const minutes = Math.floor(ms / 60_000);
  if (minutes < 1) return "just now";
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  return `${Math.floor(hours / 24)}d ago`;
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `pnpm --filter dock test`
Expected: PASS — all existing tests plus 6 queue and 13 pending-summary tests.

- [ ] **Step 5: De-duplicate `host`**

In `apps/dock/src/panel/Panel.tsx`, delete the local `host` function (lines 25-32) and add `import { host } from "../lib/url";` to the import block.

Run: `pnpm --filter dock test && pnpm --filter dock exec tsc --noEmit`
Expected: PASS, no type errors.

- [ ] **Step 6: Commit**

```bash
git add apps/dock/src/lib/queue.ts apps/dock/src/lib/queue.test.ts \
        apps/dock/src/lib/pending-summary.ts apps/dock/src/lib/pending-summary.test.ts \
        apps/dock/src/lib/url.ts apps/dock/src/panel/Panel.tsx
git commit -m "feat(dock): typed queue client and pure pending-strip logic"
```

---

### Task 7: Pending strip in the panel, and extract the confirm strip

**Files:**
- Create: `apps/dock/src/panel/PendingStrip.tsx`, `apps/dock/src/panel/ConfirmStrip.tsx`
- Modify: `apps/dock/src/panel/Panel.tsx`

**Interfaces:**
- Consumes from Task 6: `QueuedCapture`, `listQueue`, `subscribeQueue`, `flushQueue`, `removeQueued`, `enqueueCapture`, `pendingSummary`, `entryLabel`, `relativeAge`.
- Produces: two components, consumed only by `Panel.tsx`.
  - `<ConfirmStrip confirm={ConfirmState} title={string} error={string | null} inputRef={RefObject<HTMLInputElement>} onChangeTags={(v: string) => void} onKeyDown={(e: KeyboardEvent<HTMLInputElement>) => void} />`
  - `<PendingStrip items={QueuedCapture[]} onRetry={() => void} onDiscard={(id: string) => void} />`

- [ ] **Step 1: Extract `ConfirmStrip`**

Create `apps/dock/src/panel/ConfirmStrip.tsx`, moving the JSX currently at `Panel.tsx:667-695` and the five `confirm*` style objects from `Panel.tsx`'s `styles` verbatim:

```tsx
import type { CSSProperties, KeyboardEvent, RefObject } from "react";
import { tokens } from "@openmind/ui";
import type { ConfirmState } from "../lib/save-confirm";

export function ConfirmStrip({
  confirm,
  title,
  error,
  inputRef,
  onChangeTags,
  onKeyDown,
}: {
  confirm: ConfirmState;
  title: string;
  error: string | null;
  inputRef: RefObject<HTMLInputElement | null>;
  onChangeTags: (value: string) => void;
  onKeyDown: (e: KeyboardEvent<HTMLInputElement>) => void;
}) {
  if (confirm.kind === "hidden") return null;
  return (
    <div style={styles.strip}>
      <span style={styles.title}>Saved — {title}</span>
      {confirm.kind === "done" ? (
        <span style={styles.done}>Tagged ✓</span>
      ) : (
        <>
          <input
            ref={inputRef}
            style={styles.input}
            value={confirm.kind === "confirming" || confirm.kind === "saving-tags" ? confirm.tags : ""}
            onChange={(e) => onChangeTags(e.target.value)}
            onKeyDown={onKeyDown}
            placeholder="Add tags…"
            disabled={confirm.kind === "saving-tags"}
            autoCapitalize="off"
            autoCorrect="off"
            spellCheck={false}
          />
          <span style={{ ...styles.hint, ...(error ? { color: tokens.color.danger } : {}) }}>
            {error ?? (confirm.kind === "saving-tags" ? "Saving…" : "Enter to tag · Esc to skip")}
          </span>
        </>
      )}
    </div>
  );
}

const styles: Record<string, CSSProperties> = {
  strip: {
    display: "flex",
    alignItems: "center",
    gap: 10,
    padding: "8px 16px",
    borderBottom: `1px solid ${tokens.color.hairline}`,
    background: tokens.color.noteSurface,
  },
  title: {
    fontSize: 13,
    fontWeight: 600,
    color: tokens.color.ink,
    whiteSpace: "nowrap",
    overflow: "hidden",
    textOverflow: "ellipsis",
    maxWidth: "40%",
  },
  input: {
    flex: 1,
    border: `1px solid ${tokens.color.hairline}`,
    borderRadius: 8,
    background: tokens.color.cardSurface,
    color: tokens.color.ink,
    fontSize: 13,
    fontFamily: tokens.font.sans,
    padding: "6px 10px",
    minWidth: 0,
  },
  hint: {
    fontFamily: tokens.font.mono,
    fontSize: 10,
    color: tokens.color.inkFaint,
    whiteSpace: "nowrap",
  },
  done: {
    fontSize: 13,
    fontWeight: 600,
    color: tokens.color.green,
  },
};
```

In `Panel.tsx`, replace the block at lines 667-695 with:

```tsx
      <ConfirmStrip
        confirm={confirm}
        title={confirmTitleRef.current}
        error={confirmError}
        inputRef={confirmInputRef}
        onChangeTags={(value) => {
          setConfirmError(null);
          dispatchConfirm({ type: "type-tags", tags: value });
        }}
        onKeyDown={onConfirmTagKeyDown}
      />
```

Delete the five now-unused `confirm*` entries from `Panel.tsx`'s `styles`, and add `import { ConfirmStrip } from "./ConfirmStrip";`.

- [ ] **Step 2: Verify the extraction changed nothing**

Run: `pnpm --filter dock test && pnpm --filter dock exec tsc --noEmit`
Expected: PASS. This step is a pure move — no behaviour change, so no new test.

- [ ] **Step 3: Create `PendingStrip`**

Create `apps/dock/src/panel/PendingStrip.tsx`:

```tsx
import { useState } from "react";
import type { CSSProperties } from "react";
import { tokens } from "@openmind/ui";
import { entryLabel, pendingSummary, relativeAge } from "../lib/pending-summary";
import type { QueuedCapture } from "../lib/queue";

/** Rows shown when expanded; the rest collapse into a "+N more" line. */
const VISIBLE_ROWS = 5;

export function PendingStrip({
  items,
  onRetry,
  onDiscard,
}: {
  items: QueuedCapture[];
  onRetry: () => void;
  onDiscard: (id: string) => void;
}) {
  const [expanded, setExpanded] = useState(false);
  if (items.length === 0) return null;

  const { label, stuck } = pendingSummary(items);
  const accent = stuck ? tokens.color.danger : tokens.color.gold;
  const now = Date.now();
  const shown = items.slice(0, VISIBLE_ROWS);
  const overflow = items.length - shown.length;

  return (
    <div style={{ ...styles.strip, borderLeft: `2px solid ${accent}` }}>
      <div style={styles.headRow}>
        <span style={{ ...styles.label, color: accent }}>{label}</span>
        <button type="button" style={styles.action} onClick={onRetry}>
          Retry now
        </button>
        <button
          type="button"
          style={styles.chevron}
          aria-expanded={expanded}
          aria-label={expanded ? "Hide pending saves" : "Show pending saves"}
          onClick={() => setExpanded((v) => !v)}
        >
          {expanded ? "▾" : "▸"}
        </button>
      </div>

      {expanded ? (
        <ul style={styles.list}>
          {shown.map((entry) => (
            <li key={entry.id} style={styles.row}>
              <span style={styles.rowLabel}>{entryLabel(entry)}</span>
              <span style={styles.rowMeta}>
                {[relativeAge(entry.createdAt, now), entry.lastError].filter(Boolean).join(" · ")}
              </span>
              <button
                type="button"
                style={styles.discard}
                aria-label={`Discard ${entryLabel(entry)}`}
                onClick={() => onDiscard(entry.id)}
              >
                ×
              </button>
            </li>
          ))}
          {overflow > 0 ? <li style={styles.more}>+{overflow} more</li> : null}
        </ul>
      ) : null}
    </div>
  );
}

const styles: Record<string, CSSProperties> = {
  strip: {
    display: "flex",
    flexDirection: "column",
    gap: 6,
    padding: "8px 16px",
    borderBottom: `1px solid ${tokens.color.hairline}`,
    background: tokens.color.noteSurface,
  },
  headRow: { display: "flex", alignItems: "center", gap: 10 },
  label: { flex: 1, fontSize: 13, fontWeight: 600, minWidth: 0 },
  action: {
    border: `1px solid ${tokens.color.hairline}`,
    borderRadius: 999,
    background: tokens.color.cardSurface,
    color: tokens.color.cobalt,
    fontSize: 12,
    fontWeight: 600,
    padding: "4px 10px",
    cursor: "pointer",
  },
  chevron: {
    border: "none",
    background: "none",
    color: tokens.color.inkMuted,
    fontSize: 12,
    cursor: "pointer",
    padding: "2px 4px",
  },
  list: { listStyle: "none", margin: 0, padding: 0, display: "flex", flexDirection: "column", gap: 4 },
  row: { display: "flex", alignItems: "center", gap: 8 },
  rowLabel: {
    flex: 1,
    fontSize: 12,
    color: tokens.color.ink,
    overflow: "hidden",
    textOverflow: "ellipsis",
    whiteSpace: "nowrap",
    minWidth: 0,
  },
  rowMeta: {
    fontFamily: tokens.font.mono,
    fontSize: 10,
    color: tokens.color.inkFaint,
    whiteSpace: "nowrap",
  },
  discard: {
    border: "none",
    background: "none",
    color: tokens.color.inkFaint,
    fontSize: 14,
    lineHeight: 1,
    cursor: "pointer",
    padding: "0 2px",
  },
  more: { fontFamily: tokens.font.mono, fontSize: 10, color: tokens.color.inkFaint },
};
```

- [ ] **Step 4: Wire it into `Panel.tsx`**

a. Add imports:

```tsx
import { PendingStrip } from "./PendingStrip";
import { enqueueCapture, flushQueue, listQueue, removeQueued, subscribeQueue, type QueuedCapture } from "../lib/queue";
```

b. Add state beside the other `useState` calls:

```tsx
  const [pending, setPending] = useState<QueuedCapture[]>([]);
```

c. Add the subscription effect, after the `open-settings` listener effect:

```tsx
  // Rust owns the queue; mirror it here for the strip.
  useEffect(() => {
    let unlisten: (() => void) | undefined;
    void listQueue().then(setPending);
    void subscribeQueue(setPending).then((fn) => {
      unlisten = fn;
    });
    return () => unlisten?.();
  }, []);
```

d. In the existing `onFocusChanged` handler, inside `if (focused) { … }`, add after `bumpHome()`:

```tsx
          // Coming back to the panel is a good moment to retry: it usually
          // means the machine woke or the network came back. It is also when
          // the tray Desk submenu is refreshed, in place of a background timer.
          void flushQueue();
          void invoke("desk_refresh").catch(() => {});
```

`invoke` needs importing in `Panel.tsx`: `import { invoke } from "@tauri-apps/api/core";`

e. Render the strip immediately after `<ConfirmStrip … />`:

```tsx
      <PendingStrip
        items={pending}
        onRetry={() => void flushQueue()}
        onDiscard={(id) => void removeQueued(id)}
      />
```

f. Make panel saves queue instead of erroring. In `performSave`, replace the failure branch:

```tsx
      if (res.status === 401) {
        showErrorToast("Token rejected — open Settings");
      } else if (res.status === 0 || res.status === 429 || res.status >= 500) {
        const offline = res.status === 0;
        try {
          // Never lose the capture — queue it and let the strip explain.
          await enqueueCapture(body);
          showErrorToast(
            offline ? "Saved offline — will retry" : "Instance error — queued, will retry",
          );
        } catch {
          // enqueueCapture deliberately does not swallow, and every caller
          // uses `void saveRawInput()` — so an unguarded await here escapes as
          // an unhandled rejection and the user sees nothing at all while the
          // capture is lost. If the queue itself failed, nothing is holding
          // this capture: say so rather than implying it is safe.
          showErrorToast("Couldn't queue the save — try again");
        }
      } else {
        showErrorToast(`Save failed (${res.status})`);
      }
```

- [ ] **Step 5: Verify**

Run: `pnpm --filter dock test && pnpm --filter dock exec tsc --noEmit && pnpm --filter dock build`
Expected: PASS.

Then `pnpm exec tauri dev` and confirm by hand:
- With the instance reachable, the strip is absent.
- Turn wifi off, press ⌘⇧S in a browser: the notification reads "Saved offline — will retry (1 pending)", the strip appears, and the tray shows "1 pending save".
- Type in the search box while the strip is visible: **focus must stay in the search input**, and ↑/↓ must move through results only, never into the strip.
- Turn wifi on and click "Retry now": the strip empties and the item appears in the library.

- [ ] **Step 6: Commit**

```bash
git add apps/dock/src/panel/PendingStrip.tsx apps/dock/src/panel/ConfirmStrip.tsx \
        apps/dock/src/panel/Panel.tsx
git commit -m "feat(dock): pending-saves strip, and extract the confirm strip

Panel.tsx sheds the confirm-strip markup and styles as it gains the
pending strip, so it gets shorter rather than longer."
```

---

### Task 8: Documentation

**Files:**
- Modify: `apps/dock/README.md`
- Modify: `apps/web/lib/architecture.ts` (the Dock client row, and `LAST_UPDATED`)
- Modify: `TODO.md`

**Interfaces:** none.

- [ ] **Step 1: Update the dock README**

In `apps/dock/README.md`, replace the last line of the Notes section (`- Not yet: Windows/Linux, offline save queue.`) with:

```markdown
- **Offline saves are never lost.** A save that fails on a network error — or
  on a 5xx/429 from the instance — is queued to disk and retried: on launch,
  every 60 seconds, whenever the panel regains focus, and on demand from the
  tray ("Retry pending saves") or the panel strip ("Retry now"). Pending saves
  show in a strip at the top of the panel, where each can be discarded. A
  rejected token stops the queue rather than burning it, and the cap is 100
  captures (oldest dropped).
- **The panel is resizable** and remembers its size and position across
  restarts. A position saved on a display you have since disconnected is
  recentred rather than restored offscreen.
- The tray menu has a **Desk** submenu of your pinned items — refreshed on
  launch, after a save, and when the panel regains focus.
- Not yet: Windows/Linux tab grab.
```

Then replace the browser list in the Hotkeys section. The current text reads
"Supported browsers: Safari, Chrome, Brave, Edge, Arc." — replace with:

```markdown
  Supported browsers: Safari, Chrome, Brave, Edge, and Arc (all verified), plus
  Safari Technology Preview, Orion, Vivaldi, Opera, Chromium, and Chrome
  Beta/Dev/Canary (bundle ids follow vendor convention and are **not yet
  verified against a real install** — if one of these is your front app and
  the grab reports "Front app isn't a supported browser", please open an
  issue with the output of `osascript -e 'id of app "<Name>"'`).
```

- [ ] **Step 2: Update the architecture page**

CLAUDE.md requires the public `/architecture` page to stay current. In
`apps/web/lib/architecture.ts`, the Dock client row currently reads:

```ts
  { name: "Dock", stack: "Tauri", role: "Floating desktop capture + Desk/Recents, global hotkey." },
```

Change it to match how the Mobile row already advertises its queue:

```ts
  { name: "Dock", stack: "Tauri", role: "Floating desktop capture + Desk/Recents, global hotkey, offline queue." },
```

Bump `LAST_UPDATED` in the same file to `2026-08-12`.

- [ ] **Step 3: Run the architecture test**

Run: `pnpm --filter web test`
Expected: PASS — `apps/web/lib/architecture.ts` has vitest coverage; if a test asserts the old Dock copy or the old date, update the assertion to match.

- [ ] **Step 4: Update TODO.md**

Delete the stale `- Dock follow-ups: tray Desk submenu, Win/Linux tab-grab, hotkey rebinding, DMG/notarisation` line from **Later** (hotkey rebinding and DMG/notarisation both shipped in dock v2 — the line was wrong on two counts). Add to **Later**:

```markdown
- Dock follow-ups: Win/Linux tab grab (no AppleScript equivalent — would need
  per-browser platform work or a bridge through the extension); unify the two
  HTTP paths to `POST /api/items` (Rust reqwest at 15s, TS plugin-http at 12s)
  behind one Rust `save_item` command so the queue cannot be bypassed; verify
  the eight newly-added browser bundle ids against real installs (only the
  original five are confirmed)
```

Add to **Done (recent)**:

```markdown
- Dock functional polish (2026-08-12) — durable offline save queue (Rust-owned,
  policy mirrored from mobile's `capture-queue.ts`: cap 100, URL dedupe,
  oldest-first flush, 401 stops the pass, permanent 4xx dropped, transient
  bumps attempts and stops), pending strip in the panel, tray Desk submenu +
  pending count, resizable panel with clamped size/position memory, and eight
  more browsers in the tab grab. Spec:
  `docs/superpowers/specs/20260811-dock-functional-polish-design.md`.
  **⌘⇧S previously discarded a capture outright on a network error.**
```

- [ ] **Step 5: Commit**

```bash
git add apps/dock/README.md apps/web/lib/architecture.ts TODO.md
git commit -m "docs(dock): offline queue, window memory, and the wider browser list"
```

---

### Task 9: Release gate

The dock auto-updates on every launch, so a release reaches every installed dock. Do not cut one until the checklist below actually passes on a real machine.

**Files:**
- Modify: `apps/dock/src-tauri/tauri.conf.json` (version)

- [ ] **Step 1: Full test sweep**

```bash
cd apps/dock/src-tauri && cargo test
cd ../../.. && pnpm --filter dock test && pnpm --filter dock exec tsc --noEmit
pnpm --filter web test
task lint
```

Expected: all green.

- [ ] **Step 2: Human verification checklist**

None of these can be automated. Each must be confirmed on a real machine before release:

- [ ] Offline round trip: wifi off → ⌘⇧S → "Saved offline — will retry (1 pending)" → strip and tray both show it → wifi on → drains → item is in the library.
- [ ] Bad-token behaviour: set a junk token, queue two saves, retry — the queue does **not** empty, and both entries survive.
- [ ] Kill the app with entries queued, relaunch: the entries are still there and drain on launch.
- [ ] Corrupt `queue.json` by hand (truncate it mid-object), relaunch: the app starts with an empty queue and no crash.
- [ ] **Tray handler survives a menu rebuild.** Queue a save first, so `rebuild_tray_menu` has replaced the menu via `set_menu`, and *then* click a Desk item. `on_menu_event` is registered on the tray **icon**, not the menu, and this was never verifiable headless. If the handler has stopped firing, every Desk item and "Retry pending saves" silently does nothing, and the fix is to move the `match` into `app.on_menu_event(...)` in `setup()`. Checking the *first* menu passes trivially and does not test this.
- [ ] With no settings configured, the Desk submenu shows a disabled "Open Settings first"; once configured and fetched, it lists pins and opens one in the browser.
- [ ] Resize and move the panel, quit, relaunch: geometry is restored.
- [ ] Move the panel to a second display, quit, disconnect it, relaunch: the panel appears centred on the remaining display.
- [ ] **Retina default geometry.** On a 2× display, delete `window.json` and launch: the panel must open at 640×420 *logical* and centred — not at the 520×360 minimum, and not off-centre. This is the regression test for the logical-vs-physical pixel bug the whole-branch review caught, it reaches every installed dock on first launch after an auto-update, and no test can cover it.
- [ ] **The panel is genuinely resizable from its edges** with `decorations: false`, and the drag strip still moves it. `resizable` was flipped and `center` dropped in the same change, and neither has been exercised outside a headless build.
- [ ] **Revoked token is legible.** With entries queued, revoke the token server-side (not just a junk string — that is the case above), wait through two 60 s drainer cycles, and describe what the user can actually see. If the answer is "a gold 'N saves waiting to sync' and nothing else unless I expand the strip", decide whether that ships.
- [ ] **Duplicate on a committed-but-timed-out save.** Against a slow instance, force a save to exceed the timeout *after* the server has committed, then let the queue retry. Confirm whether a duplicate item appears. Mobile guards this and the dock does not; 60 seconds of manual work confirms or kills it.
- [ ] **Failed disk write tells the truth.** Make the queue directory unwritable, then save from the panel: the toast must read "Couldn't queue the save — try again", never "Saved offline — will retry". Applies to notes especially, which exist nowhere else.
- [ ] **Settings survive a quit.** Connect, quit, relaunch: the dock must come back connected. keyring 3.x silently failed this on macOS 26 — `set_password` returned `Ok` and nothing persisted — so this is now a standing check, not a given.
- [ ] **Esc and the × both hide the panel**, and the panel still hides after opening an item. Every one of `hide`'s five call sites was silently denied by the ACL until `core:window:allow-hide` was granted, so these need eyes rather than assumption.

### How to run this checklist — non-obvious and it cost a session to learn

**Test from a signed `.app` bundle in `/Applications`, never from `tauri dev` or a bare `cargo` binary.** Two independent reasons:

- macOS keychain ACLs bind to the code signature. A bare `target/debug/app` is ad-hoc, linker-signed, with `Info.plist=not bound` and an identifier that changes on every rebuild — so it cannot read items written by a Developer-ID-signed build, and every rebuild looks like a new app. Keychain-backed settings are simply untestable that way.
- `tauri dev` competes for the cargo lock with any other cargo build on the machine (a 3m24s stall was traced to two unrelated builds in other sessions), and its app process can die without restarting.

The working recipe:

```bash
pnpm exec tauri build --debug --bundles app
codesign --force --deep -s "Developer ID Application: <you> (<TEAMID>)" \
  src-tauri/target/debug/bundle/macos/openmind-dock.app
# do NOT pass --options runtime: the hardened runtime needs entitlements
# (WebKit JIT) that a local build does not carry
cp -R src-tauri/target/debug/bundle/macos/openmind-dock.app /Applications/
open /Applications/openmind-dock.app
```

**Check for a second installation first.** A released dock already in `/Applications` shares the bundle id *and* the keychain service, so both fight over the tray and the hotkeys, and it is easy to spend an hour testing the wrong one:

```bash
mdfind "kMDItemCFBundleIdentifier == 'fun.gilla.openmind.dock'"
ps ax | grep "openmind-dock.app/Contents/MacOS"   # expect exactly one
```

**Read the log from disk, not a terminal** — a bundled app has no attached stdout:
`~/Library/Logs/fun.gilla.openmind.dock/openmind-dock.log`
- [ ] The pending strip never steals focus from the search input, and ↑/↓ never enters it.
- [ ] At least one newly-added browser confirmed by hand if installed (`osascript -e 'id of app "Vivaldi"'` to check the bundle id first).

- [ ] **Step 3: Bump the version and release**

Only once every box above is ticked. Bump `version` in `apps/dock/src-tauri/tauri.conf.json` from `0.3.0` to `0.4.0`, commit, and follow the repo's dock release process (CrabNebula CDN + GitHub Releases; signing and notarisation come from CI secrets).

```bash
git add apps/dock/src-tauri/tauri.conf.json
git commit -m "chore(dock): bump version to 0.4.0"
```

---

## Notes for the implementer

- **`quick_save` still bypasses the queue on the happy path.** It posts directly
  and only enqueues on failure. That is intentional for this pass — see the spec's
  Out of scope on unifying the two HTTP paths.
- **The 60s drainer thread never exits.** It is a daemon thread on a tray app that
  lives until quit; that is fine, but do not copy the pattern anywhere that
  expects clean shutdown.
- **The tray rebuild is coalesced in `flush`.** A rebuild is not cheap: it is
  roughly 20 blocking main-thread round-trips, and while the Desk cache is empty
  `build_menu` also reads the keychain twice. Doing that once per entry meant up
  to 100 rebuilds per drain, so `flush` emits `queue-changed` per entry (the
  panel strip needs live progress) but rebuilds the tray once after the pass, and
  only when something actually changed. Single mutations — `enqueue`,
  `queue_remove` — still do both at once via `notify_changed`. The
  `StopUnauthorized` arm skips the persist and the emit entirely, since it
  mutates nothing.
  This supersedes an earlier note in this plan that deferred the coalescing;
  the maintainer chose to do it during Task 3's review, 2026-08-12.

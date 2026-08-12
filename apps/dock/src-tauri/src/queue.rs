//! Durable offline capture queue. A save that fails for a transient reason
//! lands here and is retried later, so a capture is never lost to a flaky
//! network. The delivery policy (what to do with each HTTP outcome) mirrors
//! apps/mobile/lib/capture-queue.ts exactly; the enqueue trigger does not —
//! the dock also queues a fresh 429/5xx save, where mobile only queues a
//! network error (status 0). That is a deliberate maintainer decision, not
//! drift to fix.
use serde::{Deserialize, Serialize};
use serde_json::json;
use std::fs;
use std::io;
use std::path::PathBuf;
use std::sync::atomic::{AtomicBool, AtomicU64, Ordering};
use std::sync::Mutex;
use std::time::{Duration, SystemTime, UNIX_EPOCH};
use tauri::{AppHandle, Emitter, Manager};

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
    /// Whether the queue file on disk was actually updated to include this
    /// call's outcome. `insert` alone (no disk access) always sets this to
    /// `true`; `enqueue` overwrites it with the real result of `persist`.
    pub persisted: bool,
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
            return InsertResult { id: existing.id.clone(), deduped: true, dropped: 0, persisted: true };
        }
    }
    let id = entry.id.clone();
    items.push(entry);
    let mut dropped = 0;
    if items.len() > MAX_QUEUE {
        dropped = items.len() - MAX_QUEUE;
        items.drain(0..dropped);
    }
    InsertResult { id, deduped: false, dropped, persisted: true }
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
/// state lock held. Returns `Err` when the write did not durably land —
/// callers that promised the user a retry must not do so on that path.
fn persist(app: &AppHandle, items: &[QueuedCapture]) -> Result<(), io::Error> {
    let Some(path) = queue_path(app) else {
        return Err(io::Error::new(io::ErrorKind::NotFound, "no app config dir"));
    };
    if let Some(dir) = path.parent() {
        let _ = fs::create_dir_all(dir);
    }
    // Serialising a Vec<QueuedCapture> cannot fail in practice (no NaN
    // floats, no non-string keys); treat a failure as opaque and unlogged
    // rather than risk ever interpolating serde_json's error text.
    let json = serde_json::to_string_pretty(items)
        .map_err(|_| io::Error::new(io::ErrorKind::InvalidData, "queue serialisation failed"))?;
    let tmp = path.with_extension("json.tmp");
    if let Err(e) = fs::write(&tmp, json) {
        log::warn!("queue.json temp write failed: {e}");
        return Err(e);
    }
    if let Err(e) = fs::rename(&tmp, &path) {
        log::warn!("queue.json rename failed: {e}");
        return Err(e);
    }
    Ok(())
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

/// Tells the panel the queue changed. Cheap — no tray work, so a caller that
/// rebuilds the tray itself (e.g. `flush`, once per pass) can use this
/// per-mutation without paying for a menu rebuild each time.
fn emit_changed(app: &AppHandle, items: &[QueuedCapture]) {
    let _ = app.emit_to("panel", "queue-changed", items);
}

/// Tells the panel and the tray that the queue changed. Call with the lock
/// released — the tray rebuild reads state itself.
fn notify_changed(app: &AppHandle, items: Vec<QueuedCapture>) {
    emit_changed(app, &items);
    crate::rebuild_tray_menu(app);
}

/// Queues a capture. Exactly one of url/note should be set. `persisted` on
/// the result tells the caller whether the write actually landed on disk —
/// an in-memory-only entry might still drain this session, but the caller
/// must not promise the user a retry that survives a quit.
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
        let mut result = insert(&mut items, entry);
        if result.dropped > 0 {
            log::warn!("queue cap {MAX_QUEUE} exceeded; dropped {} oldest", result.dropped);
        }
        result.persisted = persist(app, &items).is_ok();
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

    // The 60s drainer already skips an empty queue before calling in; the
    // panel-focus path does not, so check here too rather than paying for a
    // keychain read on every focus.
    if pending_count(&app) == 0 {
        return;
    }

    let settings = match crate::settings::settings_get() {
        Ok(Some(s)) => s,
        // Unconfigured or keychain error: leave the queue alone rather than
        // burning attempts against nothing.
        _ => return,
    };
    let Ok(client) = reqwest::Client::builder().timeout(Duration::from_secs(15)).build() else {
        log::warn!("flush: couldn't build the HTTP client — skipping this pass");
        return;
    };

    // One rebuild for the whole pass rather than one per entry: draining a
    // full queue can mean up to MAX_QUEUE round trips through the tray menu
    // builder otherwise. The panel still gets a per-entry emit for its live
    // progress strip.
    let mut mutated = false;

    loop {
        let next = {
            let state = app.state::<QueueState>();
            let items = state.lock().unwrap();
            items.iter().min_by_key(|q| q.created_at).cloned()
        };
        let Some(entry) = next else { break };

        let status = post_capture(&client, &settings, &entry).await;
        let disp = disposition(status);

        // A bad token: stop the pass rather than burning the whole queue
        // against a rejected token, but still say so — otherwise a revoked
        // token leaves "N pending" showing forever with no explanation.
        // Deliberately does not bump `attempts`: that count means "delivery
        // attempts that could plausibly have worked", and this one never
        // could.
        if disp == Disposition::StopUnauthorized {
            let snapshot = {
                let state = app.state::<QueueState>();
                let mut items = state.lock().unwrap();
                if let Some(q) = items.iter_mut().find(|q| q.id == entry.id) {
                    q.last_error = Some("Token rejected — open Settings".to_string());
                }
                persist(&app, &items).ok();
                items.clone()
            };
            mutated = true;
            emit_changed(&app, &snapshot);
            break;
        }

        let snapshot = {
            let state = app.state::<QueueState>();
            let mut items = state.lock().unwrap();
            match disp {
                Disposition::Delivered => items.retain(|q| q.id != entry.id),
                Disposition::DropPermanent => {
                    log::warn!("dropping a queued capture after permanent HTTP {status}");
                    items.retain(|q| q.id != entry.id);
                }
                Disposition::Retry => {
                    if let Some(q) = items.iter_mut().find(|q| q.id == entry.id) {
                        q.attempts += 1;
                        q.last_error = Some(error_label(status));
                    }
                }
                // Not reached in practice: the early `break` above already
                // stops the pass on this disposition before we get here.
                Disposition::StopUnauthorized => {}
            }
            persist(&app, &items).ok();
            items.clone()
        };
        mutated = true;
        emit_changed(&app, &snapshot);

        if disp == Disposition::Retry {
            break;
        }
    }

    if mutated {
        crate::rebuild_tray_menu(&app);
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
        persist(&app, &items).ok();
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
        assert_eq!(r, InsertResult { id: "a".into(), deduped: false, dropped: 0, persisted: true });
        assert_eq!(items.len(), 1);
    }

    #[test]
    fn insert_dedupes_a_pending_url_and_returns_the_existing_id() {
        let mut items = vec![entry("a", Some("https://one.example"), 1)];
        let r = insert(&mut items, entry("b", Some("https://one.example"), 2));
        assert_eq!(r, InsertResult { id: "a".into(), deduped: true, dropped: 0, persisted: true });
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

    #[test]
    fn parse_queue_rejects_a_type_mismatched_field() {
        // createdAt is a string where an integer is required: serde reports
        // this as a Data error whose Display would quote the offending value.
        let raw = r#"[{"id":"a","url":"https://one.example","createdAt":"nope","attempts":0}]"#;
        assert!(parse_queue(raw).is_empty());
    }
}

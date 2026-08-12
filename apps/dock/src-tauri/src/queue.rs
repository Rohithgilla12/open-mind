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

    #[test]
    fn parse_queue_rejects_a_type_mismatched_field() {
        // createdAt is a string where an integer is required: serde reports
        // this as a Data error whose Display would quote the offending value.
        let raw = r#"[{"id":"a","url":"https://one.example","createdAt":"nope","attempts":0}]"#;
        assert!(parse_queue(raw).is_empty());
    }
}

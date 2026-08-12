import { useEffect, useState } from "react";
import type { CSSProperties } from "react";
import { tokens } from "@openmind/ui";
import { entryLabel, pendingSummary, relativeAge } from "../lib/pending-summary";
import type { QueuedCapture } from "../lib/queue";

/** Rows shown when expanded; the rest collapse into a "+N more" line. */
const VISIBLE_ROWS = 5;

/** How often the relative-age text re-renders while items are pending. */
const AGE_REFRESH_MS = 30_000;

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
  // Forces a re-render so relative ages ("3m ago") advance even when the
  // queue itself is stalled and no queue-changed event ever arrives.
  const [, tick] = useState(0);
  useEffect(() => {
    if (items.length === 0) return;
    const id = setInterval(() => tick((n) => n + 1), AGE_REFRESH_MS);
    return () => clearInterval(id);
  }, [items.length]);

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

import type { CSSProperties, ReactNode } from "react";
import { tokens } from "@openmind/ui";
import { LogoMark } from "./LogoMark";

/** Marks a region the user can drag to reposition the frameless panel window. */
export function DragRegion({
  children,
  style,
}: {
  children?: ReactNode;
  style?: CSSProperties;
}) {
  // data-tauri-drag-region only fires when the mousedown targets the element
  // itself — children swallow the event otherwise, which made the panel feel
  // stuck. Content is presentation-only, so it opts out of pointer events.
  return (
    <div data-tauri-drag-region style={style}>
      <div style={{ pointerEvents: "none" }}>{children}</div>
    </div>
  );
}

/** Shared top chrome: terracotta rule + wordmark — the primary drag handle.
 *  `onClose` adds an × at the right. It re-enables pointer events on itself,
 *  since DragRegion disables them for children; and because Tauri's drag
 *  script treats a BUTTON as clickable, it blocks dragging rather than
 *  starting one, so no extra guard is needed. */
export function PanelDragStrip({ onClose }: { onClose?: () => void }) {
  return (
    <DragRegion style={styles.strip}>
      <div style={styles.accent} />
      <div style={styles.row}>
        <LogoMark size={16} />
        <span style={styles.wordmark}>Openmind</span>
        {onClose ? (
          <button type="button" style={styles.close} aria-label="Close panel" onClick={onClose}>
            ×
          </button>
        ) : null}
      </div>
    </DragRegion>
  );
}

const styles: Record<string, CSSProperties> = {
  strip: {
    flex: "none",
    background: tokens.color.header,
    borderBottom: `1px solid ${tokens.color.hairline}`,
  },
  accent: {
    height: 2,
    background: `linear-gradient(90deg, ${tokens.color.terracotta}, ${tokens.color.terracotta} 42%, transparent)`,
  },
  row: {
    display: "flex",
    alignItems: "center",
    gap: 8,
    padding: "8px 16px 10px",
  },
  close: {
    // DragRegion disables pointer events for children; re-enable for this one.
    pointerEvents: "auto",
    marginLeft: "auto",
    border: "none",
    background: "none",
    color: tokens.color.inkFaint,
    fontSize: 16,
    lineHeight: 1,
    cursor: "pointer",
    padding: "0 2px",
  },
  wordmark: {
    fontFamily: tokens.font.quote,
    fontSize: 14,
    fontWeight: 600,
    letterSpacing: "-0.01em",
    color: tokens.color.inkMuted,
  },
};

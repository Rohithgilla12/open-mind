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

/** Shared top chrome: terracotta rule + wordmark — the primary drag handle. */
export function PanelDragStrip() {
  return (
    <DragRegion style={styles.strip}>
      <div style={styles.accent} />
      <div style={styles.row}>
        <LogoMark size={16} />
        <span style={styles.wordmark}>Openmind</span>
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
  wordmark: {
    fontFamily: tokens.font.quote,
    fontSize: 14,
    fontWeight: 600,
    letterSpacing: "-0.01em",
    color: tokens.color.inkMuted,
  },
};

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

"use client";

import { useState, useTransition } from "react";
import { useRouter } from "next/navigation";
import { tokens } from "@openmind/ui";

const { color, font } = tokens;

/**
 * Unsubscribe from a feed. Confirms first (already-imported items are kept —
 * this just stops polling), then DELETEs via the proxy and refreshes the list.
 */
export function DeleteFeedButton({ id, title }: { id: string; title: string }) {
  const router = useRouter();
  const [busy, setBusy] = useState(false);
  const [pending, startTransition] = useTransition();
  const [error, setError] = useState<string | null>(null);

  async function onDelete() {
    if (!window.confirm(`Unsubscribe from “${title}”? Saved posts are kept.`)) return;
    setBusy(true);
    setError(null);
    try {
      const res = await fetch(`/api/feeds/${id}`, { method: "DELETE" });
      if (res.ok || res.status === 404) {
        startTransition(() => router.refresh());
        return;
      }
      setError("Couldn't unsubscribe — try again.");
    } catch {
      setError("Couldn't unsubscribe — try again.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <span style={{ display: "inline-flex", alignItems: "center", gap: 8 }}>
      {error && (
        <span style={{ fontFamily: font.mono, fontSize: 11, color: color.danger }} aria-live="polite">
          {error}
        </span>
      )}
      <button
        type="button"
        onClick={onDelete}
        disabled={busy || pending}
        aria-label={`Unsubscribe from ${title}`}
        style={{
          flex: "none",
          fontFamily: font.mono,
          fontSize: 11,
          letterSpacing: ".04em",
          color: color.inkFaintAlt,
          background: "transparent",
          border: `1px solid ${color.hairline}`,
          borderRadius: 8,
          padding: "6px 10px",
          cursor: busy || pending ? "default" : "pointer",
          opacity: busy || pending ? 0.6 : 1,
        }}
      >
        {busy ? "Removing…" : "Unsubscribe"}
      </button>
    </span>
  );
}

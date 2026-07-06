"use client";

import { tokens } from "@openmind/ui";
import { useRouter } from "next/navigation";
import { useState, useTransition } from "react";

const { color, font } = tokens;

/**
 * Toggle an item's Desk pin. Optimistic-free: PATCH `{pinned}` through the
 * same-origin proxy, then `router.refresh()` to re-read server state. Inline
 * error on failure, mirroring DeleteButton / TagEditor.
 */
export function PinButton({ itemId, pinned }: { itemId: string; pinned: boolean }) {
  const router = useRouter();
  const [pending, startTransition] = useTransition();
  const [error, setError] = useState<string | null>(null);

  function toggle() {
    setError(null);
    startTransition(async () => {
      try {
        const res = await fetch(`/api/items/${itemId}`, {
          method: "PATCH",
          headers: { "content-type": "application/json" },
          body: JSON.stringify({ pinned: !pinned }),
        });
        if (!res.ok) {
          setError("Could not update. Please try again.");
          return;
        }
        router.refresh();
      } catch {
        setError("Could not update. Please try again.");
      }
    });
  }

  return (
    <span style={{ display: "inline-flex", alignItems: "center", gap: 8 }}>
      {error ? (
        <span aria-live="polite">
          <span style={{ fontFamily: font.mono, fontSize: "0.72rem", color: color.danger }}>
            {error}
          </span>
        </span>
      ) : null}
      <button
        type="button"
        onClick={toggle}
        disabled={pending}
        aria-pressed={pinned}
        aria-label={pinned ? "Unpin from desk" : "Pin to desk"}
        style={{
          display: "inline-flex",
          alignItems: "center",
          gap: 6,
          fontFamily: font.mono,
          fontSize: "0.78rem",
          color: pinned ? color.gold : color.inkMuted,
          background: "none",
          border: "none",
          padding: 0,
          cursor: pending ? "default" : "pointer",
          opacity: pending ? 0.5 : 1,
        }}
      >
        <span aria-hidden style={{ fontSize: "0.85rem", lineHeight: 1 }}>
          {pinned ? "◆" : "◇"}
        </span>
        {pinned ? "On desk — unpin" : "Pin to desk"}
      </button>
    </span>
  );
}

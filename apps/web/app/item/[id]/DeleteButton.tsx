"use client";

import { tokens } from "@openmind/ui";
import { useRouter } from "next/navigation";
import { useState } from "react";

export function DeleteButton({ id }: { id: string }) {
  const router = useRouter();
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function onDelete() {
    if (!confirm("Delete this item?")) return;
    setBusy(true);
    setError(null);
    try {
      const res = await fetch(`/api/items/${id}`, { method: "DELETE" });
      if (res.status === 204) {
        router.push("/");
        router.refresh();
        return;
      }
      setError("Could not delete. Please try again.");
    } catch {
      setError("Could not delete. Please try again.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <span style={{ display: "inline-flex", alignItems: "center", gap: 8 }}>
      {error ? (
        <span aria-live="polite">
          <span
            style={{
              fontFamily: tokens.font.mono,
              fontSize: "0.72rem",
              color: tokens.color.danger,
            }}
          >
            {error}
          </span>
        </span>
      ) : null}
      <button
        type="button"
        onClick={onDelete}
        disabled={busy}
        aria-label="Delete this item"
        style={{
          fontFamily: tokens.font.mono,
          fontSize: "0.78rem",
          color: tokens.color.danger,
          background: "none",
          border: "none",
          padding: 0,
          cursor: busy ? "default" : "pointer",
          opacity: busy ? 0.5 : 1,
        }}
      >
        {busy ? "deleting…" : "delete"}
      </button>
    </span>
  );
}

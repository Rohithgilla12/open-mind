"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { tokens } from "@openmind/ui";

const { color, font } = tokens;

/** Deletes a Lens (never its items — a Lens is only a saved view). */
export function DeleteLensButton({ id, name }: { id: string; name: string }) {
  const router = useRouter();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function onDelete() {
    if (!confirm(`Delete the lens “${name}”? Your saved items are not affected.`)) return;
    setBusy(true);
    setError(null);
    try {
      const res = await fetch(`/api/lenses/${id}`, { method: "DELETE" });
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
      {error && (
        <span style={{ fontFamily: font.mono, fontSize: 11, color: color.danger }} aria-live="polite">
          {error}
        </span>
      )}
      <button
        type="button"
        onClick={onDelete}
        disabled={busy}
        aria-label="Delete this lens"
        style={{
          fontFamily: font.mono,
          fontSize: 11,
          letterSpacing: ".04em",
          color: color.danger,
          background: "none",
          border: "none",
          padding: 0,
          cursor: busy ? "default" : "pointer",
          opacity: busy ? 0.5 : 1,
        }}
      >
        {busy ? "deleting…" : "delete lens"}
      </button>
    </span>
  );
}

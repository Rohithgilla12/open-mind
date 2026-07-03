"use client";

import { useState, useTransition } from "react";
import { useRouter } from "next/navigation";
import { tokens } from "@openmind/ui";

export function QuickAdd() {
  const router = useRouter();
  const [value, setValue] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [pending, startTransition] = useTransition();
  const [submitting, setSubmitting] = useState(false);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    const v = value.trim();
    if (!v) return;
    setError(null);
    setSubmitting(true);
    try {
      const isURL = /^https?:\/\//i.test(v);
      const res = await fetch("/api/items", {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify(isURL ? { url: v } : { note: v }),
      });
      if (res.status === 201) {
        setValue("");
        startTransition(() => router.refresh());
        return;
      }
      setError("Could not save — please try again.");
    } catch {
      setError("Could not save — please try again.");
    } finally {
      setSubmitting(false);
    }
  }

  const disabled = submitting || pending;

  return (
    <form
      onSubmit={handleSubmit}
      style={{ display: "flex", flexDirection: "column", gap: "0.5rem", marginBottom: "1rem" }}
    >
      <div style={{ display: "flex", gap: "0.5rem" }}>
        <input
          value={value}
          onChange={(e) => setValue(e.target.value)}
          placeholder="Drop a link or a thought…"
          disabled={disabled}
          style={{
            flex: 1,
            padding: "0.6rem 0.75rem",
            fontFamily: tokens.font.sans,
            fontSize: "0.95rem",
            border: `1px solid ${tokens.color.line}`,
            borderRadius: 8,
            backgroundColor: tokens.color.surface,
            color: tokens.color.ink,
          }}
        />
        <button
          type="submit"
          disabled={disabled || !value.trim()}
          style={{
            padding: "0.6rem 1rem",
            fontFamily: tokens.font.sans,
            fontWeight: 600,
            color: tokens.color.surface,
            backgroundColor: tokens.color.cobalt,
            border: "none",
            borderRadius: 8,
            cursor: disabled || !value.trim() ? "not-allowed" : "pointer",
            opacity: disabled || !value.trim() ? 0.6 : 1,
          }}
        >
          {disabled ? "Saving…" : "Save"}
        </button>
      </div>
      {error ? (
        <p style={{ color: tokens.color.danger, fontFamily: tokens.font.sans, fontSize: "0.85rem", margin: 0 }}>
          {error}
        </p>
      ) : null}
    </form>
  );
}

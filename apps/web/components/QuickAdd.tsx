"use client";

import { useState, useTransition } from "react";
import { useRouter } from "next/navigation";
import { tokens } from "@openmind/ui";

const { color, font } = tokens;

export function QuickAdd() {
  const router = useRouter();
  const [value, setValue] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [pending, startTransition] = useTransition();
  const [submitting, setSubmitting] = useState(false);
  const [focused, setFocused] = useState(false);

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

  const idle = disabled || !value.trim();

  return (
    <form
      onSubmit={handleSubmit}
      style={{ display: "flex", flexDirection: "column", gap: 8 }}
    >
      <div style={{ display: "flex", gap: 8 }}>
        <input
          value={value}
          onChange={(e) => setValue(e.target.value)}
          onFocus={() => setFocused(true)}
          onBlur={() => setFocused(false)}
          placeholder="Drop a link or a thought…"
          aria-label="Save a link or a note"
          disabled={disabled}
          style={{
            flex: 1,
            minWidth: 0,
            padding: "11px 14px",
            fontFamily: font.sans,
            fontSize: 14,
            border: `1px solid ${focused ? color.cobalt : color.hairline}`,
            borderRadius: 10,
            backgroundColor: color.cardSurface,
            color: color.ink,
            outline: "none",
            boxShadow: focused ? "0 0 0 3px rgba(27,63,209,.15)" : "none",
            transition: ".15s",
          }}
        />
        <button
          type="submit"
          className="savebtn"
          disabled={idle}
          style={{
            flex: "none",
            cursor: idle ? "not-allowed" : "pointer",
            opacity: idle ? 0.55 : 1,
          }}
        >
          {disabled ? "Saving…" : "Save"}
        </button>
      </div>
      {error ? (
        <p style={{ color: color.danger, fontFamily: font.sans, fontSize: "0.85rem", margin: 0 }}>
          {error}
        </p>
      ) : null}
    </form>
  );
}

"use client";

import { tokens } from "@openmind/ui";
import { useRef, useState } from "react";

const { color, font } = tokens;

type Status = "idle" | "sending" | "sent" | "unconfigured" | "empty" | "error";

const SENT_RESET_MS = 4000;

/**
 * Sends an item (or a Lens's current matches as a digest) to the configured
 * Kindle e-mail. Fire-and-forget from the caller's perspective: the API queues
 * an async job and returns 202 immediately — this button just reflects that.
 */
export function KindleButton({ target, id }: { target: "item" | "lens"; id: string }) {
  const [status, setStatus] = useState<Status>("idle");
  const resetTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  async function send() {
    if (status === "sending") return;
    if (resetTimer.current) clearTimeout(resetTimer.current);
    setStatus("sending");
    try {
      const res = await fetch(`/api/${target === "item" ? "items" : "lenses"}/${id}/kindle`, {
        method: "POST",
      });
      if (res.status === 202) {
        setStatus("sent");
        resetTimer.current = setTimeout(() => setStatus("idle"), SENT_RESET_MS);
        return;
      }
      if (res.status === 409) {
        setStatus("unconfigured");
        return;
      }
      if (res.status === 422) {
        setStatus("empty");
        return;
      }
      setStatus("error");
    } catch {
      setStatus("error");
    }
  }

  const idleLabel = target === "item" ? "Send to Kindle" : "Send digest to Kindle";
  const busy = status === "sending";

  const message: { text: string; color: string } | null =
    status === "sent"
      ? { text: "Sent ✓ — arrives on your Kindle shortly", color: color.green }
      : status === "unconfigured"
        ? { text: "Kindle isn't configured — see self-hosting docs", color: color.danger }
        : status === "empty"
          ? { text: "Nothing to send yet — no archived text", color: color.danger }
          : status === "error"
            ? { text: "Could not send. Please try again.", color: color.danger }
            : null;

  return (
    <span style={{ display: "inline-flex", alignItems: "center", gap: 8 }}>
      {message ? (
        <span aria-live="polite">
          <span style={{ fontFamily: font.mono, fontSize: "0.72rem", color: message.color }}>
            {message.text}
          </span>
        </span>
      ) : null}
      <button
        type="button"
        onClick={send}
        disabled={busy}
        aria-label={idleLabel}
        style={{
          display: "inline-flex",
          alignItems: "center",
          gap: 6,
          fontFamily: font.mono,
          fontSize: "0.78rem",
          color: color.inkMuted,
          background: "none",
          border: "none",
          padding: 0,
          cursor: busy ? "default" : "pointer",
          opacity: busy ? 0.5 : 1,
        }}
      >
        <span aria-hidden style={{ fontSize: "0.85rem", lineHeight: 1 }}>
          ⇢
        </span>
        {busy ? "sending…" : idleLabel}
      </button>
    </span>
  );
}

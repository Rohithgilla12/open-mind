"use client";

import { useState, useTransition } from "react";
import { useRouter } from "next/navigation";
import { tokens } from "@openmind/ui";
import type { Feed } from "../../lib/types";

const { color, font } = tokens;

/**
 * Subscribe to a feed: post the URL to /api/feeds and show an inline result.
 * On success the row appears after router.refresh(); the message reports the
 * entries backfilled when the API returns a count, else a generic confirmation.
 */
export function AddFeed() {
  const router = useRouter();
  const [url, setUrl] = useState("");
  const [busy, setBusy] = useState(false);
  const [pending, startTransition] = useTransition();
  const [error, setError] = useState<string | null>(null);
  const [ok, setOk] = useState<string | null>(null);

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    const trimmed = url.trim();
    if (!trimmed) {
      setError("Enter a valid feed URL");
      setOk(null);
      return;
    }
    setBusy(true);
    setError(null);
    setOk(null);
    try {
      const res = await fetch("/api/feeds", {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ url: trimmed }),
      });
      if (res.ok) {
        const feed = (await res.json().catch(() => null)) as
          | (Feed & { addedCount?: number })
          | null;
        const count = feed?.addedCount;
        setOk(
          typeof count === "number"
            ? `Subscribed — ${count.toLocaleString("en-GB")} ${count === 1 ? "entry" : "entries"} imported`
            : "Subscribed — new posts will be saved automatically",
        );
        setUrl("");
        startTransition(() => router.refresh());
        return;
      }
      if (res.status === 409) setError("Already subscribed");
      else if (res.status === 400) setError("Enter a valid feed URL");
      else if (res.status === 502) setError("Couldn’t reach or parse that feed");
      else setError("Couldn’t subscribe. Try again.");
    } catch {
      setError("Couldn’t subscribe. Try again.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <form onSubmit={onSubmit} style={{ maxWidth: 560, display: "flex", flexDirection: "column", gap: 12 }}>
      <div style={{ display: "flex", gap: 10, flexWrap: "wrap" }}>
        <input
          type="url"
          inputMode="url"
          value={url}
          onChange={(e) => setUrl(e.target.value)}
          placeholder="https://example.com/feed.xml"
          aria-label="Feed URL"
          style={{
            flex: "1 1 300px",
            fontFamily: font.mono,
            fontSize: 13,
            color: color.ink,
            background: color.cardSurface,
            border: `1px solid ${color.hairline}`,
            borderRadius: 10,
            padding: "11px 14px",
          }}
        />
        <button
          type="submit"
          className="savebtn"
          disabled={busy || pending}
          style={{ cursor: busy ? "default" : "pointer", opacity: busy || pending ? 0.6 : 1 }}
        >
          {busy ? "Subscribing…" : "Subscribe"}
        </button>
      </div>

      {error ? (
        <p style={{ color: color.danger, fontFamily: font.sans, fontSize: 13, margin: 0 }}>{error}</p>
      ) : null}
      {ok ? (
        <p style={{ color: color.green, fontFamily: font.sans, fontSize: 13, margin: 0 }}>{ok}</p>
      ) : null}
    </form>
  );
}

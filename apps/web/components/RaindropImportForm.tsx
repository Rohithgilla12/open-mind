"use client";

import { useState, useTransition } from "react";
import { useRouter } from "next/navigation";
import { tokens } from "@openmind/ui";
import type { ImportResult } from "../lib/types";

const { color, font } = tokens;

const stat = {
  fontFamily: font.mono,
  fontSize: 22,
  fontWeight: 600,
  lineHeight: 1,
  color: color.ink,
} as const;

/**
 * Import straight from Raindrop.io: the user pastes an API test token, the
 * server pulls the account's export once (the token is never stored) and each
 * new bookmark becomes a card. Shows the same imported / skipped / failed
 * summary as the file form; re-running is safe.
 */
export function RaindropImportForm() {
  const router = useRouter();
  const [token, setToken] = useState("");
  const [busy, setBusy] = useState(false);
  const [pending, startTransition] = useTransition();
  const [error, setError] = useState<string | null>(null);
  const [result, setResult] = useState<ImportResult | null>(null);

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!token.trim()) {
      setError("Paste your Raindrop.io test token.");
      return;
    }
    setBusy(true);
    setError(null);
    setResult(null);
    try {
      const res = await fetch("/api/import/raindrop", {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ token: token.trim() }),
      });
      if (res.ok) {
        setResult((await res.json()) as ImportResult);
        setToken("");
        startTransition(() => router.refresh());
        return;
      }
      const body = (await res.json().catch(() => null)) as { error?: string } | null;
      setError(body?.error ?? "Import failed. Check the token and try again.");
    } catch {
      setError("Import failed. Check the token and try again.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <form onSubmit={onSubmit} style={{ maxWidth: 560, display: "flex", flexDirection: "column", gap: 20 }}>
      <div
        style={{
          border: `1px solid ${color.hairline}`,
          borderRadius: 12,
          padding: "22px 22px",
          background: color.cardSurface,
          display: "flex",
          flexDirection: "column",
          gap: 12,
          alignItems: "flex-start",
        }}
      >
        <div className="meta">Import from Raindrop.io</div>
        <p
          className="meta"
          style={{ textTransform: "none", letterSpacing: ".02em", color: color.inkMuted, margin: 0, lineHeight: 1.5 }}
        >
          In Raindrop, open{" "}
          <a
            href="https://app.raindrop.io/settings/integrations"
            target="_blank"
            rel="noreferrer"
            style={{ color: color.cobalt }}
          >
            Settings → Integrations
          </a>
          , create an app, and copy its <em>test token</em>. It’s used for a single export request and never
          stored — your tags come along, and each collection becomes a tag.
        </p>
        <input
          type="password"
          value={token}
          onChange={(e) => setToken(e.target.value)}
          placeholder="Raindrop test token"
          aria-label="Raindrop.io test token"
          autoComplete="off"
          style={{
            fontFamily: font.mono,
            fontSize: 13,
            color: color.ink,
            background: color.paper,
            border: `1px solid ${color.hairline}`,
            borderRadius: 8,
            padding: "9px 12px",
            width: "100%",
            boxSizing: "border-box",
          }}
        />
        <button
          type="submit"
          className="savebtn"
          disabled={busy || pending}
          style={{ cursor: busy ? "default" : "pointer", opacity: busy || pending ? 0.6 : 1 }}
        >
          {busy ? "Importing…" : "Import from Raindrop"}
        </button>
      </div>

      {error ? (
        <p style={{ color: color.danger, fontFamily: font.sans, fontSize: 13, margin: 0 }}>{error}</p>
      ) : null}

      {result ? (
        <div
          style={{
            display: "flex",
            gap: 26,
            padding: "18px 22px",
            background: color.panel,
            border: `1px solid ${color.hairline}`,
            borderRadius: 12,
          }}
        >
          {[
            { n: result.imported, label: "imported", tone: color.green },
            { n: result.skipped, label: "skipped", tone: color.inkMuted },
            { n: result.failed, label: "failed", tone: result.failed > 0 ? color.terracotta : color.inkFaint },
          ].map((s) => (
            <div key={s.label}>
              <div style={{ ...stat, color: s.tone }}>{s.n.toLocaleString("en-GB")}</div>
              <div className="meta" style={{ marginTop: 6 }}>
                {s.label}
              </div>
            </div>
          ))}
        </div>
      ) : null}

      {result && result.imported > 0 ? (
        <p className="meta" style={{ textTransform: "none", letterSpacing: ".02em", color: color.inkFaintAlt, margin: 0 }}>
          Imported items are enriching in the background — they’ll fill in on{" "}
          <a href="/" style={{ color: color.cobalt }}>
            the Mind
          </a>{" "}
          shortly.
        </p>
      ) : null}
    </form>
  );
}

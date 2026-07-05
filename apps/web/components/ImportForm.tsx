"use client";

import { useRef, useState, useTransition } from "react";
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
 * Upload a bookmark/read-later export and post it to /api/import. Shows a
 * per-file summary (imported / skipped / failed) on success; imported items
 * enrich asynchronously, so it nudges the reader back to the library.
 */
export function ImportForm() {
  const router = useRouter();
  const inputRef = useRef<HTMLInputElement>(null);
  const [fileName, setFileName] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [pending, startTransition] = useTransition();
  const [error, setError] = useState<string | null>(null);
  const [result, setResult] = useState<ImportResult | null>(null);

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    const file = inputRef.current?.files?.[0];
    if (!file) {
      setError("Choose a file to import.");
      return;
    }
    setBusy(true);
    setError(null);
    setResult(null);
    try {
      const form = new FormData();
      form.append("file", file);
      const res = await fetch("/api/import", { method: "POST", body: form });
      if (res.ok) {
        setResult((await res.json()) as ImportResult);
        startTransition(() => router.refresh());
        return;
      }
      const body = (await res.json().catch(() => null)) as { error?: string } | null;
      setError(body?.error ?? "Import failed. Check the file and try again.");
    } catch {
      setError("Import failed. Check the file and try again.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <form onSubmit={onSubmit} style={{ maxWidth: 560, display: "flex", flexDirection: "column", gap: 20 }}>
      <div
        style={{
          border: `1.5px dashed ${color.hairline}`,
          borderRadius: 12,
          padding: "26px 22px",
          background: color.cardSurface,
          display: "flex",
          flexDirection: "column",
          gap: 14,
          alignItems: "flex-start",
        }}
      >
        <input
          ref={inputRef}
          type="file"
          accept=".html,.htm,.csv,.txt,text/html,text/csv,text/plain"
          onChange={(e) => setFileName(e.target.files?.[0]?.name ?? null)}
          aria-label="Export file to import"
          style={{ fontFamily: font.sans, fontSize: 13, color: color.ink }}
        />
        {fileName ? (
          <span className="meta" style={{ textTransform: "none", letterSpacing: ".02em", color: color.inkMuted }}>
            {fileName}
          </span>
        ) : null}
        <button
          type="submit"
          className="savebtn"
          disabled={busy || pending}
          style={{ cursor: busy ? "default" : "pointer", opacity: busy || pending ? 0.6 : 1 }}
        >
          {busy ? "Importing…" : "Import"}
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

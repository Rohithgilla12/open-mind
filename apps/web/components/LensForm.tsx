"use client";

import { useState, useTransition } from "react";
import { useRouter } from "next/navigation";
import { tokens } from "@openmind/ui";
import { cardKind, typeLabel, type CardKind } from "../lib/cards";
import { COLOR_SWATCHES, resolveColor } from "../lib/colors";

const { color, font } = tokens;

const ALL_TYPES: readonly CardKind[] = [
  "article",
  "product",
  "book",
  "recipe",
  "video",
  "tweet",
  "image",
  "note",
  "quote",
];

const fieldLabel = {
  fontFamily: font.mono,
  fontSize: 10,
  letterSpacing: ".08em",
  textTransform: "uppercase",
  color: color.inkFaint,
  marginBottom: 8,
  display: "block",
} as const;

const textInput = {
  width: "100%",
  padding: "10px 13px",
  fontFamily: font.sans,
  fontSize: 14,
  border: `1px solid ${color.hairline}`,
  borderRadius: 9,
  background: color.cardSurface,
  color: color.ink,
  outline: "none",
} as const;

/**
 * Create/refine a Lens. A Lens = a named saved rule (text, colour, domains, card types);
 * the same signals /search uses. The form seeds from the current search when
 * reached via "Save as lens", so a query you like becomes a standing view.
 */
export function LensForm({
  initialName = "",
  initialQ = "",
  initialColor = "",
  initialTypes = [],
  initialDomains = [],
}: {
  initialName?: string;
  initialQ?: string;
  initialColor?: string;
  initialTypes?: string[];
  initialDomains?: string[];
}) {
  const router = useRouter();
  const [name, setName] = useState(initialName);
  const [q, setQ] = useState(initialQ);
  const [domains, setDomains] = useState(() => initialDomains.join(", "));
  const [swatch, setSwatch] = useState(initialColor.toLowerCase());
  const [types, setTypes] = useState<Set<string>>(
    () => new Set(initialTypes.filter((t) => (ALL_TYPES as readonly string[]).includes(t))),
  );
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [pending, startTransition] = useTransition();

  const domainList = domains
    .split(/[\s,]+/)
    .map((d) => d.trim())
    .filter(Boolean);
  const hasRule = q.trim() !== "" || swatch !== "" || types.size > 0 || domainList.length > 0;
  const canSubmit = name.trim() !== "" && hasRule && !busy && !pending;

  function toggleType(t: string) {
    setTypes((prev) => {
      const next = new Set(prev);
      if (next.has(t)) next.delete(t);
      else next.add(t);
      return next;
    });
  }

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!canSubmit) return;
    setBusy(true);
    setError(null);
    const rule: { q?: string; color?: string; types?: string[]; domains?: string[] } = {};
    if (q.trim()) rule.q = q.trim();
    if (swatch) rule.color = swatch;
    if (types.size > 0) rule.types = [...types];
    if (domainList.length) rule.domains = domainList;
    try {
      const res = await fetch("/api/lenses", {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ name: name.trim(), rule }),
      });
      if (res.status === 201) {
        const lens = (await res.json()) as { id: string };
        startTransition(() => {
          router.push(`/lens/${lens.id}`);
          router.refresh();
        });
        return;
      }
      const body = (await res.json().catch(() => null)) as { error?: string } | null;
      setError(body?.error ?? "Could not save the lens. Please try again.");
    } catch {
      setError("Could not save the lens. Please try again.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <form onSubmit={onSubmit} style={{ maxWidth: 520, display: "flex", flexDirection: "column", gap: 22 }}>
      <div>
        <label htmlFor="lens-name" style={fieldLabel}>
          Name
        </label>
        <input
          id="lens-name"
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="Design inspiration"
          maxLength={120}
          style={textInput}
        />
      </div>

      <div>
        <label htmlFor="lens-q" style={fieldLabel}>
          Query
        </label>
        <input
          id="lens-q"
          value={q}
          onChange={(e) => setQ(e.target.value)}
          placeholder="anything about running shoes"
          style={textInput}
        />
      </div>

      <div>
        <label htmlFor="lens-domains" style={fieldLabel}>
          Domains
        </label>
        <input
          id="lens-domains"
          value={domains}
          onChange={(e) => setDomains(e.target.value)}
          placeholder="x.com, twitter.com"
          style={textInput}
        />
        <p className="meta" style={{ textTransform: "none", letterSpacing: ".02em", margin: "8px 0 0" }}>
          Host only — subdomains match
        </p>
      </div>

      <div>
        <span style={fieldLabel}>Colour</span>
        <div style={{ display: "flex", alignItems: "center", gap: 9, flexWrap: "wrap" }}>
          {COLOR_SWATCHES.map((c) => {
            const active = swatch === c;
            return (
              <button
                key={c}
                type="button"
                title={c}
                aria-label={c}
                aria-pressed={active}
                onClick={() => setSwatch(active ? "" : c)}
                style={{
                  width: 22,
                  height: 22,
                  borderRadius: 11,
                  background: resolveColor(c) ?? "transparent",
                  border: "none",
                  cursor: "pointer",
                  boxShadow: active
                    ? `0 0 0 2px ${color.paper}, 0 0 0 3.5px ${color.ink}`
                    : "0 0 0 1px rgba(28,26,22,.14) inset",
                  transition: ".15s",
                }}
              />
            );
          })}
          {swatch && (
            <span className="meta" style={{ textTransform: "none", letterSpacing: ".02em" }}>
              {swatch}
            </span>
          )}
        </div>
      </div>

      <div>
        <span style={fieldLabel}>Card types</span>
        <div style={{ display: "flex", flexWrap: "wrap", gap: 8 }}>
          {ALL_TYPES.map((t) => {
            const active = types.has(t);
            return (
              <button
                key={t}
                type="button"
                className="chip"
                aria-pressed={active}
                onClick={() => toggleType(t)}
                style={
                  active
                    ? { background: color.ink, color: color.paper, borderColor: color.ink }
                    : undefined
                }
              >
                {typeLabel[cardKind(t)]}
              </button>
            );
          })}
        </div>
      </div>

      {error && (
        <p style={{ color: color.danger, fontFamily: font.sans, fontSize: 13, margin: 0 }}>{error}</p>
      )}
      {!hasRule && (
        <p className="meta" style={{ textTransform: "none", letterSpacing: ".02em", margin: 0 }}>
          Add a query, a colour, a domain, or at least one card type — a lens needs something to match.
        </p>
      )}

      <div style={{ display: "flex", alignItems: "center", gap: 14 }}>
        <button
          type="submit"
          className="savebtn"
          disabled={!canSubmit}
          style={{ cursor: canSubmit ? "pointer" : "not-allowed", opacity: canSubmit ? 1 : 0.55 }}
        >
          {busy || pending ? "Saving…" : "Save lens"}
        </button>
        <button
          type="button"
          onClick={() => router.back()}
          style={{
            background: "none",
            border: "none",
            padding: 0,
            fontFamily: font.mono,
            fontSize: 12,
            color: color.inkMuted,
            cursor: "pointer",
          }}
        >
          Cancel
        </button>
      </div>
    </form>
  );
}

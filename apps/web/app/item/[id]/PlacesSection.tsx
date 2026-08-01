"use client";

import { tokens } from "@openmind/ui";
import { useRouter } from "next/navigation";
import { useState, useTransition, type CSSProperties, type ReactNode } from "react";

const { color, font } = tokens;

export type Place = {
  id: string;
  name: string;
  hint: string;
  address: string;
  lat?: number;
  lng?: number;
  source: string;
};

const removeBtn: CSSProperties = {
  display: "inline-flex",
  alignItems: "center",
  justifyContent: "center",
  flex: "0 0 auto",
  width: 18,
  height: 18,
  marginTop: -1,
  borderRadius: "50%",
  border: "none",
  background: "none",
  color: color.inkFaint,
  fontFamily: font.mono,
  fontSize: 13,
  lineHeight: 1,
  padding: 0,
};

function mapsUrl(p: Place): string {
  const query =
    p.lat != null && p.lng != null
      ? `${p.lat},${p.lng}`
      : encodeURIComponent(`${p.name} ${p.hint}`.trim());
  return `https://www.google.com/maps/search/?api=1&query=${query}`;
}

/**
 * Extracted places for one item, each removable. Extraction is a guess — a
 * reel caption's brand name or a model's invention lands here alongside the
 * real venues — so every row gets an escape hatch. Removal is optimistic:
 * the row goes immediately and only comes back if the server rejects it.
 *
 * The rail's leading divider is rendered here rather than by the parent so
 * removing the last place takes the rule with it, instead of leaving a stray
 * hairline until the server re-render lands.
 */
export function PlacesSection({
  itemId,
  places,
  divider,
}: {
  itemId: string;
  places: Place[];
  divider: ReactNode;
}) {
  const router = useRouter();
  const [pending, startTransition] = useTransition();
  const [removed, setRemoved] = useState<string[]>([]);
  const [error, setError] = useState<string | null>(null);

  const visible = places.filter((p) => !removed.includes(p.id));
  if (visible.length === 0) return null;

  function remove(place: Place) {
    setError(null);
    setRemoved((prev) => [...prev, place.id]);
    startTransition(async () => {
      try {
        const res = await fetch(`/api/items/${itemId}/places/${place.id}`, { method: "DELETE" });
        // 404 means it is already gone server-side — the row staying hidden is
        // the right outcome, so only a real failure restores it.
        if (!res.ok && res.status !== 404) {
          setRemoved((prev) => prev.filter((id) => id !== place.id));
          setError(`Could not remove ${place.name}. Please try again.`);
          return;
        }
        router.refresh();
      } catch {
        setRemoved((prev) => prev.filter((id) => id !== place.id));
        setError(`Could not remove ${place.name}. Please try again.`);
      }
    });
  }

  return (
    <>
      {divider}
      <div className="meta" style={{ color: color.inkFaintAlt }}>
        Places
      </div>
      {visible.map((p) => (
        <div key={p.id} style={{ marginTop: 9 }}>
          <div style={{ display: "flex", alignItems: "flex-start", gap: 6 }}>
            <div
              style={{
                flex: 1,
                minWidth: 0,
                fontFamily: font.sans,
                fontSize: 13,
                fontWeight: 600,
                color: color.ink,
              }}
            >
              {p.name}
            </div>
            <button
              type="button"
              onClick={() => remove(p)}
              disabled={pending}
              aria-label={`Remove place ${p.name}`}
              title="Remove this place"
              style={{ ...removeBtn, cursor: pending ? "default" : "pointer", opacity: pending ? 0.4 : 1 }}
            >
              ×
            </button>
          </div>
          {p.address ? (
            <div
              style={{
                fontFamily: font.sans,
                fontSize: 12,
                lineHeight: 1.4,
                color: color.inkMuted,
                marginTop: 2,
              }}
            >
              {p.address}
            </div>
          ) : null}
          <a
            href={mapsUrl(p)}
            target="_blank"
            rel="noreferrer"
            style={{ fontFamily: font.mono, fontSize: 11, color: color.cobalt, textDecoration: "none" }}
          >
            Open in maps ↗
          </a>
        </div>
      ))}
      {error ? (
        <p
          aria-live="polite"
          style={{ fontFamily: font.mono, fontSize: "0.72rem", color: color.danger, margin: "8px 0 0" }}
        >
          {error}
        </p>
      ) : null}
    </>
  );
}

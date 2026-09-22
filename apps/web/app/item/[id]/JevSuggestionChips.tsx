"use client";

import { tokens } from "@openmind/ui";
import { useRouter } from "next/navigation";
import { useState, useTransition, type CSSProperties } from "react";

const { color, font } = tokens;

type JevSuggestions = {
  action: string;
  suggestedTags: string[];
  appliedTags: string[];
  userVerdict?: string | null;
};

const chipBase: CSSProperties = {
  display: "inline-flex",
  alignItems: "center",
  gap: 6,
  font: `500 10px/1 ${font.mono}`,
  letterSpacing: ".02em",
  borderRadius: 20,
  padding: "6px 8px 6px 10px",
};

const suggestChip: CSSProperties = {
  ...chipBase,
  color: color.inkMuted,
  background: `color-mix(in srgb, ${color.gold} 14%, transparent)`,
  border: `1px dashed color-mix(in srgb, ${color.gold} 45%, transparent)`,
  cursor: "pointer",
};

const appliedChip: CSSProperties = {
  ...chipBase,
  color: color.green,
  background: `color-mix(in srgb, ${color.green} 9%, transparent)`,
  border: `1px solid color-mix(in srgb, ${color.green} 22%, transparent)`,
};

const dismissBtn: CSSProperties = {
  display: "inline-flex",
  alignItems: "center",
  justifyContent: "center",
  minWidth: 18,
  minHeight: 18,
  borderRadius: "50%",
  border: "none",
  background: "none",
  fontFamily: font.mono,
  fontSize: 13,
  lineHeight: 1,
  cursor: "pointer",
  padding: 0,
};

/**
 * One-tap chips for Phase 2 Jev capture: accept / dismiss mid-confidence
 * suggestions, and undo auto-applied tags. Hidden when there is nothing to show.
 */
export function JevSuggestionChips({
  itemId,
  suggestions,
}: {
  itemId: string;
  suggestions?: JevSuggestions | null;
}) {
  const router = useRouter();
  const [pending, startTransition] = useTransition();
  const [error, setError] = useState<string | null>(null);

  if (!suggestions) return null;
  const suggested = suggestions.suggestedTags ?? [];
  const applied = suggestions.appliedTags ?? [];
  if (suggested.length === 0 && applied.length === 0) return null;

  function act(action: "accept" | "dismiss" | "undo" | "keep", tag: string) {
    setError(null);
    startTransition(async () => {
      try {
        const res = await fetch(`/api/items/${itemId}/jev-verdict`, {
          method: "POST",
          headers: { "content-type": "application/json" },
          body: JSON.stringify({ action, tag }),
        });
        if (!res.ok) {
          setError("Could not update suggestion. Please try again.");
          return;
        }
        router.refresh();
      } catch {
        setError("Could not update suggestion. Please try again.");
      }
    });
  }

  return (
    <div style={{ marginTop: 14 }}>
      {suggested.length > 0 ? (
        <>
          <div className="meta" style={{ color: color.inkFaintAlt }}>
            Suggested tags
          </div>
          <div style={{ display: "flex", flexWrap: "wrap", gap: 5, marginTop: 9, alignItems: "center" }}>
            {suggested.map((t) => (
              <span key={t} style={{ ...suggestChip, opacity: pending ? 0.6 : 1 }}>
                <button
                  type="button"
                  onClick={() => act("accept", t)}
                  disabled={pending}
                  aria-label={`Add suggested tag ${t}`}
                  style={{
                    border: "none",
                    background: "none",
                    padding: 0,
                    margin: 0,
                    font: `500 10px/1 ${font.mono}`,
                    letterSpacing: ".02em",
                    color: color.inkMuted,
                    cursor: pending ? "default" : "pointer",
                  }}
                >
                  + {t}
                </button>
                <button
                  type="button"
                  onClick={(e) => {
                    e.stopPropagation();
                    act("dismiss", t);
                  }}
                  disabled={pending}
                  aria-label={`Dismiss suggested tag ${t}`}
                  style={{ ...dismissBtn, color: color.inkFaint }}
                >
                  ×
                </button>
              </span>
            ))}
          </div>
        </>
      ) : null}
      {applied.length > 0 ? (
        <>
          <div className="meta" style={{ color: color.inkFaintAlt, marginTop: suggested.length ? 12 : 0 }}>
            Auto-tagged
          </div>
          <div style={{ display: "flex", flexWrap: "wrap", gap: 5, marginTop: 9, alignItems: "center" }}>
            {applied.map((t) => (
              <span key={t} style={{ ...appliedChip, opacity: pending ? 0.6 : 1 }}>
                {t}
                <button
                  type="button"
                  onClick={() => act("undo", t)}
                  disabled={pending}
                  aria-label={`Undo auto-applied tag ${t}`}
                  style={{ ...dismissBtn, color: color.green }}
                >
                  ×
                </button>
              </span>
            ))}
          </div>
        </>
      ) : null}
      {error ? (
        <p
          aria-live="polite"
          style={{ fontFamily: font.mono, fontSize: "0.72rem", color: color.danger, margin: "8px 0 0" }}
        >
          {error}
        </p>
      ) : null}
    </div>
  );
}

"use client";

import { tokens } from "@openmind/ui";
import { useRouter } from "next/navigation";
import { useState, useTransition, type CSSProperties } from "react";

const { color, font } = tokens;

// Cobalt-tinted chip so user tags read as distinct from the plain AI `.tag`.
const userChip: CSSProperties = {
  display: "inline-flex",
  alignItems: "center",
  gap: 5,
  font: `500 10px/1 ${font.mono}`,
  letterSpacing: ".02em",
  color: color.cobalt,
  background: `color-mix(in srgb, ${color.cobalt} 9%, transparent)`,
  border: `1px solid color-mix(in srgb, ${color.cobalt} 22%, transparent)`,
  padding: "4px 4px 4px 8px",
  borderRadius: 20,
};

const removeBtn: CSSProperties = {
  display: "inline-flex",
  alignItems: "center",
  justifyContent: "center",
  width: 14,
  height: 14,
  borderRadius: "50%",
  border: "none",
  background: "none",
  color: color.cobalt,
  fontFamily: font.mono,
  fontSize: 12,
  lineHeight: 1,
  cursor: "pointer",
  padding: 0,
};

/** Normalise a typed tag the way the server canonicalises: trim + lowercase. */
function canonical(raw: string): string {
  return raw.trim().toLowerCase();
}

export function TagEditor({ itemId, userTags }: { itemId: string; userTags: string[] }) {
  const router = useRouter();
  const [pending, startTransition] = useTransition();
  const [error, setError] = useState<string | null>(null);
  const [draft, setDraft] = useState("");

  function save(next: string[]) {
    setError(null);
    startTransition(async () => {
      try {
        const res = await fetch(`/api/items/${itemId}`, {
          method: "PATCH",
          headers: { "content-type": "application/json" },
          body: JSON.stringify({ userTags: next }),
        });
        if (!res.ok) {
          setError("Could not save tags. Please try again.");
          return;
        }
        router.refresh();
      } catch {
        setError("Could not save tags. Please try again.");
      }
    });
  }

  function remove(tag: string) {
    save(userTags.filter((t) => t !== tag));
  }

  function add() {
    const tag = canonical(draft);
    if (!tag) return;
    if (userTags.some((t) => t.toLowerCase() === tag)) {
      setDraft("");
      return;
    }
    setDraft("");
    save([...userTags, tag]);
  }

  return (
    <div>
      <div className="meta" style={{ color: color.inkFaintAlt }}>
        Your tags
      </div>
      <div style={{ display: "flex", flexWrap: "wrap", gap: 5, marginTop: 9, alignItems: "center" }}>
        {userTags.map((t) => (
          <span key={t} style={userChip}>
            {t}
            <button
              type="button"
              onClick={() => remove(t)}
              disabled={pending}
              aria-label={`Remove tag ${t}`}
              style={{ ...removeBtn, cursor: pending ? "default" : "pointer", opacity: pending ? 0.5 : 1 }}
            >
              ×
            </button>
          </span>
        ))}
      </div>
      <div style={{ display: "flex", gap: 6, marginTop: 9, alignItems: "center" }}>
        <input
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              e.preventDefault();
              add();
            }
          }}
          disabled={pending}
          placeholder="+ add tag"
          aria-label="Add a tag"
          style={{
            font: `500 10px/1 ${font.mono}`,
            letterSpacing: ".02em",
            color: color.ink,
            background: color.paper,
            border: `1px solid ${color.hairline}`,
            borderRadius: 20,
            padding: "6px 9px",
            width: 96,
            outline: "none",
          }}
        />
        <button
          type="button"
          onClick={add}
          disabled={pending || canonical(draft).length === 0}
          style={{
            font: `500 10px/1 ${font.mono}`,
            letterSpacing: ".02em",
            color: color.cobalt,
            background: "none",
            border: "none",
            padding: 0,
            cursor: pending || canonical(draft).length === 0 ? "default" : "pointer",
            opacity: pending || canonical(draft).length === 0 ? 0.5 : 1,
          }}
        >
          add
        </button>
      </div>
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

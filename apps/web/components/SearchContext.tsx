import Link from "next/link";
import { tokens } from "@openmind/ui";
import { cardKind, typeLabel } from "../lib/cards";
import { COLOR_SWATCHES, resolveColor } from "../lib/colors";
import type { UnderstoodQuery } from "../lib/types";

const { color, font } = tokens;

/** Build a home href carrying q (+ optional colour), dropping empties. */
function href(q: string | undefined, colorTerm?: string): string {
  const p = new URLSearchParams();
  if (q) p.set("q", q);
  if (colorTerm) p.set("color", colorTerm);
  const qs = p.toString();
  return qs ? `/?${qs}` : "/";
}

/** A colour term (hex or name) rendered as a swatch dot + label pill. */
function ColorChip({ term, onClear }: { term: string; onClear?: string }) {
  const hex = resolveColor(term) ?? term;
  return (
    <span
      className="chip"
      style={{ display: "inline-flex", alignItems: "center", gap: 7, cursor: "default" }}
    >
      <span
        aria-hidden
        style={{
          width: 12,
          height: 12,
          borderRadius: 6,
          background: hex,
          boxShadow: "0 0 0 1px rgba(28,26,22,.14) inset",
          flex: "none",
        }}
      />
      {term.toLowerCase()}
      {onClear && (
        <Link
          href={onClear}
          aria-label="Clear colour filter"
          style={{ color: color.inkFaint, textDecoration: "none", lineHeight: 1 }}
        >
          ×
        </Link>
      )}
    </span>
  );
}

/**
 * Adaptive strip beneath the type filters. When a search was interpreted
 * (parse=true) or a colour filter is active, it echoes what was actually
 * searched — the refined text, the colour swatch, and any card-type filters —
 * so the machine's reading of a fuzzy query is visible ("understood as"). It
 * always offers the brand colour swatches on the right as one-tap colour search.
 */
export function SearchContext({
  q,
  understood,
  colorParam,
}: {
  q?: string;
  understood?: UnderstoodQuery;
  colorParam?: string;
}) {
  const colorTerm = understood?.color || colorParam || undefined;
  const types = understood?.types ?? [];
  // Only surface the text portion when the parse refined it (differs from the
  // raw query); under the noop provider text === q and the echo is redundant.
  const refinedText =
    understood?.text && understood.text.trim() && understood.text.trim() !== (q ?? "").trim()
      ? understood.text.trim()
      : undefined;
  const hasEcho = Boolean(colorTerm || types.length || refinedText);
  const activeColor = colorParam?.toLowerCase();

  // "Save as lens" seeds the new-lens form with the effective search: the text
  // actually searched (understood split, or the raw query), the colour, and any
  // type filters. Shown whenever there is a rule worth saving.
  const lensQuery: Record<string, string> = {};
  const qForLens = (understood?.text ?? q ?? "").trim();
  if (qForLens) lensQuery.q = qForLens;
  if (colorTerm) lensQuery.color = colorTerm;
  if (types.length) lensQuery.types = types.join(",");
  const canSave = Object.keys(lensQuery).length > 0;

  return (
    <div
      style={{
        display: "flex",
        alignItems: "center",
        flexWrap: "wrap",
        gap: "10px 12px",
        padding: "10px 28px",
        borderBottom: `1px solid ${color.hairline}`,
        background: color.header,
      }}
    >
      <span className="meta" style={{ flex: "none" }}>
        {hasEcho ? "Understood as" : "Search by colour"}
      </span>

      {hasEcho && (
        <div style={{ display: "flex", alignItems: "center", flexWrap: "wrap", gap: 8 }}>
          {refinedText && (
            <span
              className="chip serif"
              style={{ fontStyle: "italic", cursor: "default", fontSize: 13 }}
            >
              “{refinedText}”
            </span>
          )}
          {colorTerm && (
            <ColorChip term={colorTerm} onClear={colorParam ? href(q) : undefined} />
          )}
          {types.map((t) => (
            <span key={t} className="chip" style={{ cursor: "default" }}>
              {typeLabel[cardKind(t)]}
            </span>
          ))}
        </div>
      )}

      {canSave && (
        <Link
          href={{ pathname: "/lens/new", query: lensQuery }}
          className="chip"
          style={{ marginLeft: "auto", display: "inline-flex", alignItems: "center", gap: 6 }}
        >
          <span aria-hidden>◫</span> Save as lens
        </Link>
      )}

      <div
        style={{ display: "flex", alignItems: "center", gap: 7, marginLeft: canSave ? 0 : "auto" }}
        role="group"
        aria-label="Filter by colour"
      >
        {COLOR_SWATCHES.map((name) => {
          const isActive = activeColor === name;
          return (
            <Link
              key={name}
              href={href(q, name)}
              title={name}
              aria-label={`Search ${name}`}
              aria-current={isActive ? "true" : undefined}
              style={{
                width: 16,
                height: 16,
                borderRadius: 8,
                background: resolveColor(name) ?? "transparent",
                boxShadow: isActive
                  ? `0 0 0 2px ${color.header}, 0 0 0 3.5px ${color.ink}`
                  : "0 0 0 1px rgba(28,26,22,.14) inset",
                flex: "none",
                transition: ".15s",
                fontFamily: font.mono,
              }}
            />
          );
        })}
      </div>
    </div>
  );
}

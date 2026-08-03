import { tokens } from "@openmind/ui";
import Link from "next/link";
import { notFound } from "next/navigation";
import { HighlightableBody } from "../../../../components/HighlightableBody";
import { KindleButton } from "../../../../components/KindleButton";
import { Palette } from "../../../../components/Palette";
import { ReadingProgress } from "../../../../components/ReadingProgress";
import { apiFetch } from "../../../../lib/api";
import { cardKind, domainOf, isTextForward, typeLabel } from "../../../../lib/cards";
import { derivedPalette } from "../../../../lib/palette";
import { readingMinutes } from "../../../../lib/reading-time";
import { renderInlineMarkdown } from "../../../../lib/text";
import type { ItemDetail } from "../../../../lib/types";

/** Text-forward types worth painting highlights over — mirrors the "Read"
 * affordance's `readableBody` condition on the item detail page. */
function textForward(item: ItemDetail): boolean {
  return isTextForward(cardKind(item.cardType));
}

const { color, font } = tokens;

/** "ARTICLE · 8 MIN · domain · 4 JUL 2026" kicker. */
function metaLine(item: ItemDetail): string {
  const parts: string[] = [typeLabel[cardKind(item.cardType)]];
  const minutes = readingMinutes(item.body);
  if (minutes) parts.push(`${minutes} min`);
  const domain = domainOf(item.url);
  if (domain) parts.push(domain);
  const d = new Date(item.createdAt);
  if (!Number.isNaN(d.getTime())) {
    parts.push(d.toLocaleDateString("en-GB", { day: "numeric", month: "short", year: "numeric" }));
  }
  return parts.join(" · ");
}

const barLink = {
  fontFamily: font.mono,
  fontSize: 11,
  letterSpacing: ".04em",
  color: color.cobalt,
  textDecoration: "none",
} as const;

/**
 * Distraction-free reader: a single calm reading column on paper, no sidebar or
 * rail. Serif body at a generous measure and line-height for long-form reading;
 * the surrounding chrome collapses to a slim top bar plus a terracotta
 * progress hairline, and the piece closes with a colophon — the item's palette
 * fingerprint and the actions that make sense once you've read it.
 */
export default async function ReaderPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const res = await apiFetch(`/items/${id}`);
  if (!res.ok) notFound();
  const item = (await res.json()) as ItemDetail;

  const external = item.url && !item.url.startsWith("/assets/") ? item.url : null;
  const body = (item.body ?? "").trim();
  const paragraphs = body.split(/\n\n+/).map((p) => p.trim()).filter(Boolean);
  const highlightable = textForward(item) && paragraphs.length > 0;
  const colors =
    item.palette && item.palette.length > 0
      ? item.palette
      : derivedPalette(
          `${item.title ?? ""} ${(item.tags ?? []).join(" ")}`.trim() || item.cardType || "item",
        );

  return (
    <main style={{ minHeight: "100vh", background: color.paper }}>
      <ReadingProgress />
      <div
        style={{
          position: "sticky",
          top: 0,
          zIndex: 1,
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
          gap: 12,
          padding: "13px 22px",
          background: "rgba(244,240,230,.86)",
          borderBottom: `1px solid ${color.hairline}`,
          backdropFilter: "blur(8px)",
        }}
      >
        <Link href={`/item/${item.id}`} style={barLink}>
          ← Back to card
        </Link>
        {external ? (
          <a href={external} target="_blank" rel="noreferrer" style={barLink}>
            Open original ↗
          </a>
        ) : null}
      </div>

      <article style={{ maxWidth: 680, margin: "0 auto", padding: "clamp(36px, 7vh, 56px) 24px 96px" }}>
        <div className="meta" style={{ color: color.inkFaint }}>
          {metaLine(item)}
        </div>
        {item.title ? (
          <h1
            className="serif"
            style={{
              fontSize: "clamp(30px, 6vw, 42px)",
              fontWeight: 600,
              lineHeight: 1.12,
              letterSpacing: "-.02em",
              color: color.ink,
              margin: "16px 0 0",
              display: "-webkit-box",
              WebkitLineClamp: 3,
              WebkitBoxOrient: "vertical",
              overflow: "hidden",
            }}
          >
            {item.title}
          </h1>
        ) : null}

        {item.summary ? (
          <p
            className="serif"
            style={{
              fontStyle: "italic",
              fontSize: 20,
              lineHeight: 1.6,
              color: color.inkMuted,
              margin: "22px 0 0",
            }}
          >
            {renderInlineMarkdown(item.summary)}
          </p>
        ) : null}

        {(item.title || item.summary) && paragraphs.length > 0 ? (
          <hr style={{ border: "none", borderTop: `1px solid ${color.hairline}`, margin: "34px 0" }} />
        ) : (
          <div style={{ height: 30 }} />
        )}

        {highlightable ? (
          <p
            className="meta"
            style={{
              color: color.inkFaint,
              textTransform: "none",
              letterSpacing: ".02em",
              margin: "-16px 0 26px",
            }}
          >
            Select a passage to keep it as a highlight.
          </p>
        ) : null}

        {paragraphs.length > 0 ? (
          highlightable ? (
            <HighlightableBody body={body} itemId={item.id} />
          ) : (
            paragraphs.map((p, i) => (
              <p
                key={i}
                className="serif"
                style={{
                  fontSize: 19,
                  lineHeight: 1.85,
                  color: color.ink,
                  margin: "0 0 1.5rem",
                  whiteSpace: "pre-wrap",
                }}
              >
                {p}
              </p>
            ))
          )
        ) : (
          <p style={{ fontFamily: font.sans, fontSize: 15, lineHeight: 1.7, color: color.inkMuted }}>
            {item.status === "pending"
              ? "Still enriching — the archived text will appear here once ready."
              : "No archived text to read for this item."}
            {external ? (
              <>
                {" "}
                <a href={external} target="_blank" rel="noreferrer" style={{ color: color.cobalt }}>
                  Open the original ↗
                </a>
              </>
            ) : null}
          </p>
        )}

        {/* Colophon: the item's colour fingerprint closes the reading, followed
            by the actions that make sense once you've finished. */}
        <footer style={{ marginTop: 56, paddingTop: 26, borderTop: `1px solid ${color.hairline}` }}>
          <div style={{ display: "flex", justifyContent: "center", gap: 6 }}>
            <Palette colors={colors} colorLinks />
          </div>
          <div
            style={{
              display: "flex",
              justifyContent: "center",
              alignItems: "center",
              gap: 20,
              marginTop: 20,
              flexWrap: "wrap",
            }}
          >
            <Link href={`/item/${item.id}`} style={barLink}>
              ← Back to card
            </Link>
            {external ? (
              <a href={external} target="_blank" rel="noreferrer" style={barLink}>
                Open original ↗
              </a>
            ) : null}
            <KindleButton target="item" id={item.id} />
          </div>
          <p
            className="meta"
            style={{
              color: color.inkFaint,
              textTransform: "none",
              letterSpacing: ".02em",
              margin: "22px 0 0",
              textAlign: "center",
            }}
          >
            Archived locally · link can&apos;t rot
          </p>
        </footer>
      </article>
    </main>
  );
}

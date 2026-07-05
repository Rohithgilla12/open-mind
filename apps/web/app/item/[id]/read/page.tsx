import { tokens } from "@openmind/ui";
import Link from "next/link";
import { notFound } from "next/navigation";
import { apiFetch } from "../../../../lib/api";
import { cardKind, domainOf, typeLabel } from "../../../../lib/cards";
import type { ItemDetail } from "../../../../lib/types";

const { color, font } = tokens;

/** "ARTICLE · domain · 4 JUL 2026" kicker. */
function metaLine(item: ItemDetail): string {
  const parts: string[] = [typeLabel[cardKind(item.cardType)]];
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
 * the surrounding chrome collapses to a slim top bar (back + open original).
 */
export default async function ReaderPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const res = await apiFetch(`/items/${id}`);
  if (!res.ok) notFound();
  const item = (await res.json()) as ItemDetail;

  const external = item.url && !item.url.startsWith("/assets/") ? item.url : null;
  const body = (item.body ?? "").trim();
  const paragraphs = body.split(/\n\n+/).map((p) => p.trim()).filter(Boolean);

  return (
    <main style={{ minHeight: "100vh", background: color.paper }}>
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

      <article style={{ maxWidth: 680, margin: "0 auto", padding: "56px 24px 120px" }}>
        <div className="meta" style={{ color: color.inkFaint }}>
          {metaLine(item)}
        </div>
        {item.title ? (
          <h1
            className="serif"
            style={{
              fontSize: 40,
              fontWeight: 600,
              lineHeight: 1.12,
              letterSpacing: "-.02em",
              color: color.ink,
              margin: "16px 0 0",
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
            {item.summary}
          </p>
        ) : null}

        {(item.title || item.summary) && paragraphs.length > 0 ? (
          <hr style={{ border: "none", borderTop: `1px solid ${color.hairline}`, margin: "34px 0" }} />
        ) : (
          <div style={{ height: 30 }} />
        )}

        {paragraphs.length > 0 ? (
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

        <div style={{ marginTop: 48, paddingTop: 20, borderTop: `1px solid ${color.hairline}` }}>
          <p
            className="meta"
            style={{ color: color.inkFaint, textTransform: "none", letterSpacing: ".02em", margin: 0 }}
          >
            Archived locally · link can&apos;t rot
          </p>
        </div>
      </article>
    </main>
  );
}

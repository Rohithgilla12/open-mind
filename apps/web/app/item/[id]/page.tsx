import { tokens } from "@openmind/ui";
import Link from "next/link";
import { notFound } from "next/navigation";
import type { CSSProperties, ReactNode } from "react";
import { Palette } from "../../../components/Palette";
import { RailLinks } from "../../../components/RailLinks";
import { Rule } from "../../../components/Rule";
import { apiFetch } from "../../../lib/api";
import { assetSrc } from "../../../lib/assets";
import { cardKind, domainOf, typeGradient, typeLabel } from "../../../lib/cards";
import { derivedPalette } from "../../../lib/palette";
import { relativeTime } from "../../../lib/relative-time";
import { renderInlineMarkdown } from "../../../lib/text";
import type { ItemDetail, Place } from "../../../lib/types";
import { KeepButton } from "../../../components/KeepButton";
import { KindleButton } from "../../../components/KindleButton";
import { PinButton } from "../../../components/PinButton";
import { DeleteButton } from "./DeleteButton";
import { PlacesSection } from "./PlacesSection";
import { TagEditor } from "./TagEditor";

const { color, font } = tokens;

const backLink: CSSProperties = {
  fontFamily: font.mono,
  fontSize: "0.78rem",
  color: color.cobalt,
  textDecoration: "none",
};

/**
 * Whether an item has long-form text worth opening in distraction-free reader
 * mode: a text-forward type with a non-trivial archived body.
 */
function readableBody(item: ItemDetail): boolean {
  const kind = cardKind(item.cardType);
  const textForward = kind === "article" || kind === "product" || kind === "book" || kind === "recipe" || kind === "note";
  return textForward && (item.body ?? "").trim().length > 120;
}

/** "ARTICLE · domain · 4 JUL 2026" — mono meta line above the title. */
function metaLine(item: ItemDetail): string {
  const kind = cardKind(item.cardType);
  const parts: string[] = [typeLabel[kind]];
  const domain = domainOf(item.url);
  if (domain) parts.push(domain);
  if (item.createdAt) {
    const d = new Date(item.createdAt);
    if (!Number.isNaN(d.getTime())) {
      parts.push(
        d.toLocaleDateString("en-GB", { day: "numeric", month: "short", year: "numeric" }),
      );
    }
  }
  return parts.join(" · ");
}

/**
 * Lead image painted as a background-image layered over the type gradient, so a
 * missing or broken (404) image reveals the gradient rather than a broken-image
 * glyph — the same fallback the grid cards use.
 */
function ReaderImage({ src, alt, gradient }: { src?: string; alt: string; gradient: string }) {
  return (
    <div
      style={{
        position: "relative",
        width: "100%",
        aspectRatio: "16 / 9",
        borderRadius: 11,
        overflow: "hidden",
        background: gradient,
      }}
    >
      {src ? (
        <div
          role="img"
          aria-label={alt}
          style={{
            position: "absolute",
            inset: 0,
            backgroundImage: `url(${src}), ${gradient}`,
            backgroundSize: "cover",
            backgroundPosition: "center",
            backgroundRepeat: "no-repeat",
          }}
        />
      ) : null}
    </div>
  );
}

function Title({ children }: { children: ReactNode }) {
  return (
    <h1
      className="serif"
      style={{
        fontSize: 32,
        fontWeight: 600,
        lineHeight: 1.15,
        letterSpacing: "-.02em",
        color: color.ink,
        margin: "14px 0 0",
        display: "-webkit-box",
        WebkitLineClamp: 3,
        WebkitBoxOrient: "vertical",
        overflow: "hidden",
      }}
    >
      {children}
    </h1>
  );
}

function SummaryLead({ children }: { children: string }) {
  return (
    <p
      className="serif"
      style={{
        fontSize: 16,
        lineHeight: 1.7,
        color: color.inkMuted,
        margin: "18px 0 0",
        maxWidth: "62ch",
      }}
    >
      {renderInlineMarkdown(children)}
    </p>
  );
}

function Body({ body }: { body: string }) {
  const paragraphs = body.split("\n\n").filter((p) => p.trim().length > 0);
  if (paragraphs.length === 0) return null;
  return (
    <div style={{ margin: "22px 0 0", maxWidth: "62ch" }}>
      {paragraphs.map((p, i) => (
        <p
          key={i}
          style={{
            fontFamily: font.sans,
            fontSize: 14,
            lineHeight: 1.75,
            color: color.ink,
            margin: "0 0 1.1rem",
            whiteSpace: "pre-wrap",
          }}
        >
          {p}
        </p>
      ))}
    </div>
  );
}

/** Dark editorial quote treatment — gold glyph, italic serif, matches the quote card. */
function QuoteReader({ text }: { text: string }) {
  return (
    <div style={{ margin: "20px 0 0" }}>
      <div className="serif" style={{ font: `600 46px/1 ${font.quote}`, color: color.gold, height: 26 }}>
        &ldquo;
      </div>
      <p
        className="serif"
        style={{
          fontStyle: "italic",
          fontSize: 22,
          lineHeight: 1.5,
          color: color.ink,
          margin: "10px 0 0",
          maxWidth: "56ch",
          whiteSpace: "pre-wrap",
        }}
      >
        {text}
      </p>
    </div>
  );
}

function OpenOriginal({ url }: { url: string }) {
  if (!url || url.startsWith("/assets/")) return null;
  return (
    <a href={url} target="_blank" rel="noreferrer" className="savebtn" style={{ textDecoration: "none" }}>
      Open original ↗
    </a>
  );
}

/** Right-hand rail: palette swatches, tags, and the archive assurance line. */
function Rail({ item, places }: { item: ItemDetail; places: Place[] }) {
  const tags = item.tags ?? [];
  const colors =
    item.palette && item.palette.length > 0
      ? item.palette
      : derivedPalette(`${item.title ?? ""} ${tags.join(" ")}`.trim() || item.cardType || "item");
  const divider = <Rule />;
  return (
    <aside
      style={{
        flex: "0 1 266px",
        minWidth: 220,
        background: color.panel,
        borderLeft: `1px solid ${color.hairline}`,
        padding: "26px 22px",
      }}
    >
      <div className="meta" style={{ color: color.inkFaintAlt }}>
        Palette
      </div>
      <div style={{ display: "flex", gap: 6, marginTop: 9, flexWrap: "wrap" }}>
        <Palette colors={colors} size={24} colorLinks />
      </div>
      <p
        style={{
          fontFamily: font.sans,
          fontSize: 11,
          lineHeight: 1.4,
          color: color.inkFaint,
          margin: "8px 0 0",
        }}
      >
        Tap a colour to find matches.
      </p>
      {tags.length > 0 ? (
        <>
          {divider}
          <div className="meta" style={{ color: color.inkFaintAlt }}>
            Tags
          </div>
          <div style={{ display: "flex", flexWrap: "wrap", gap: 5, marginTop: 9 }}>
            {tags.map((t) => (
              <span key={t} className="tag">
                {t}
              </span>
            ))}
          </div>
        </>
      ) : null}
      {item.feedId ? (
        <>
          {divider}
          <div className="meta" style={{ color: color.inkFaintAlt }}>
            Provenance
          </div>
          <p
            style={{
              fontFamily: font.sans,
              fontSize: 13,
              lineHeight: 1.5,
              color: color.inkMuted,
              margin: "9px 0 0",
            }}
          >
            From your feeds · {item.keptAt ? `kept ${relativeTime(item.keptAt)}` : "not kept"}
          </p>
          <div style={{ marginTop: 10 }}>
            <KeepButton itemId={item.id} kept={!!item.keptAt} />
          </div>
        </>
      ) : null}
      <PlacesSection itemId={item.id} places={places} />
      {divider}
      <TagEditor itemId={item.id} userTags={item.userTags ?? []} />
      {divider}
      <RailLinks itemId={item.id} />
      {divider}
      <p
        className="meta"
        style={{
          color: color.inkFaint,
          textTransform: "none",
          letterSpacing: ".02em",
          lineHeight: 1.5,
          margin: 0,
        }}
      >
        Archived locally · link can&apos;t rot
      </p>
    </aside>
  );
}

/** The type-aware reader body (left column content below the title). */
function ReaderContent({ item }: { item: ItemDetail }) {
  const kind = cardKind(item.cardType);
  const gradient = typeGradient[kind];

  if (item.status === "pending") {
    return (
      <p style={{ fontFamily: font.mono, fontSize: "0.82rem", color: color.cobalt, margin: "18px 0 0" }}>
        Still enriching…
      </p>
    );
  }

  if (item.status === "failed") {
    return (
      <div style={{ margin: "18px 0 0", display: "flex", flexDirection: "column", gap: 16 }}>
        <p style={{ fontFamily: font.sans, fontSize: 14, color: color.inkMuted, margin: 0 }}>
          Enrichment failed for this item.
        </p>
        <OpenOriginal url={item.url} />
      </div>
    );
  }

  if (kind === "quote") {
    return (
      <>
        <QuoteReader text={item.body || item.summary || item.title || ""} />
        <Actions url={item.url} />
      </>
    );
  }

  if (kind === "note") {
    return (
      <>
        <Body body={item.body || item.summary || item.title || ""} />
        <Actions url={item.url} />
      </>
    );
  }

  if (kind === "image") {
    return (
      <>
        <div style={{ margin: "18px 0 0" }}>
          <ReaderImage src={assetSrc(item.leadImageUrl)} alt={item.title ?? "saved image"} gradient={gradient} />
        </div>
        <Actions url={item.url} />
      </>
    );
  }

  // article / product / book / recipe / video / tweet
  const showHero = kind === "article" || kind === "product" || kind === "book" || kind === "recipe" || kind === "video";
  return (
    <>
      {showHero && item.leadImageUrl ? (
        <div style={{ margin: "18px 0 0" }}>
          <ReaderImage src={assetSrc(item.leadImageUrl)} alt={item.title ? `${item.title}` : "lead image"} gradient={gradient} />
        </div>
      ) : null}
      {item.summary ? <SummaryLead>{item.summary}</SummaryLead> : null}
      {item.body ? <Body body={item.body} /> : null}
      <Actions url={item.url} />
    </>
  );
}

function Actions({ url }: { url: string }) {
  if (!url || url.startsWith("/assets/")) return null;
  return (
    <div style={{ display: "flex", gap: 8, margin: "26px 0 0", flexWrap: "wrap" }}>
      <OpenOriginal url={url} />
    </div>
  );
}

export default async function ItemPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const res = await apiFetch(`/items/${id}`);
  if (!res.ok) notFound();
  const item = (await res.json()) as ItemDetail;

  const placesRes = await apiFetch(`/items/${id}/places`);
  const places: Place[] = placesRes.ok ? await placesRes.json() : [];

  return (
    <main
      style={{
        minHeight: "100vh",
        background: color.canvas,
        display: "flex",
        justifyContent: "center",
        alignItems: "flex-start",
        padding: "40px 24px",
      }}
    >
      <article
        style={{
          width: 960,
          maxWidth: "100%",
          background: color.cardSurface,
          borderRadius: 16,
          border: `1px solid ${color.hairline}`,
          overflow: "hidden",
          boxShadow: "0 40px 90px -20px rgba(0,0,0,.6)",
          display: "flex",
          flexWrap: "wrap",
        }}
      >
        <div style={{ flex: "1 1 460px", minWidth: 0, padding: "40px 48px" }}>
          <div
            style={{
              display: "flex",
              alignItems: "center",
              justifyContent: "space-between",
              gap: 12,
            }}
          >
            <Link href="/" style={backLink}>
              ← library
            </Link>
            <div style={{ display: "flex", alignItems: "center", gap: 16 }}>
              {readableBody(item) ? (
                <Link href={`/item/${item.id}/read`} style={{ ...backLink, color: color.ink }}>
                  Read ↗
                </Link>
              ) : null}
              <PinButton itemId={item.id} pinned={!!item.pinnedAt} />
              <KindleButton target="item" id={item.id} />
              <DeleteButton id={item.id} />
            </div>
          </div>
          <div className="meta" style={{ color: color.inkFaint, marginTop: 22 }}>
            {metaLine(item)}
            {item.pageCount != null && (
              <span>
                {" "}
                · PDF · {item.pageCount} {item.pageCount === 1 ? "page" : "pages"}
              </span>
            )}
          </div>
          {item.title ? <Title>{item.title}</Title> : null}
          <ReaderContent item={item} />
        </div>
        <Rail item={item} places={places} />
      </article>
    </main>
  );
}

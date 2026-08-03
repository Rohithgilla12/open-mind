import { tokens } from "@openmind/ui";
import Link from "next/link";
import { notFound } from "next/navigation";
import type { ReactNode } from "react";
import { Palette } from "../../../components/Palette";
import { RailLinks } from "../../../components/RailLinks";
import { Rule } from "../../../components/Rule";
import { apiFetch } from "../../../lib/api";
import { assetSrc } from "../../../lib/assets";
import { cardKind, domainOf, isTextForward, typeGradient, typeLabel, type CardKind } from "../../../lib/cards";
import { derivedPalette } from "../../../lib/palette";
import { readingMinutes } from "../../../lib/reading-time";
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

// How much archived body the detail page previews before handing off to the
// reader — enough to decide whether to commit, never the whole article.
const EXCERPT_MAX_PARAGRAPHS = 3;
const EXCERPT_MAX_CHARS = 700;

/**
 * Whether an item has long-form text worth opening in distraction-free reader
 * mode: a text-forward type with a non-trivial archived body.
 */
function readableBody(item: ItemDetail): boolean {
  const kind = cardKind(item.cardType);
  return isTextForward(kind) && (item.body ?? "").trim().length > 120;
}

/** External original URL, or null for uploads/notes whose url is an asset path. */
function externalUrl(item: ItemDetail): string | null {
  return item.url && !item.url.startsWith("/assets/") ? item.url : null;
}

/** "ARTICLE · 8 MIN · domain · 4 JUL 2026" — mono kicker above the title. */
function metaLine(item: ItemDetail): string {
  const kind = cardKind(item.cardType);
  const parts: string[] = [typeLabel[kind]];
  const minutes = readingMinutes(item.body);
  if (minutes) parts.push(`${minutes} min`);
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
        fontSize: "clamp(26px, 4.5vw, 34px)",
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

/**
 * Primary actions live directly under the title, as real buttons: Read (cobalt,
 * when there's an archived body worth reading) and Open original (outline).
 * Everything else is quiet chrome in the top bar.
 */
function ActionRow({ item }: { item: ItemDetail }) {
  const external = externalUrl(item);
  const readable = readableBody(item);
  const minutes = readingMinutes(item.body);
  if (!readable && !external) return null;
  return (
    <div style={{ display: "flex", gap: 10, margin: "22px 0 0", flexWrap: "wrap", alignItems: "center" }}>
      {readable ? (
        <Link href={`/item/${item.id}/read`} className="savebtn" style={{ textDecoration: "none", display: "inline-block" }}>
          Read{minutes ? ` · ${minutes} min` : ""}
        </Link>
      ) : null}
      {external ? (
        <a href={external} target="_blank" rel="noreferrer" className="ghostbtn">
          Open original ↗
        </a>
      ) : null}
    </div>
  );
}

function SummaryLead({ children }: { children: string }) {
  return (
    <p
      className="serif"
      style={{
        fontStyle: "italic",
        fontSize: 17,
        lineHeight: 1.65,
        color: color.inkMuted,
        margin: "20px 0 0",
        maxWidth: "60ch",
      }}
    >
      {renderInlineMarkdown(children)}
    </p>
  );
}

function BodyParagraphs({ paragraphs, serif }: { paragraphs: string[]; serif?: boolean }) {
  if (paragraphs.length === 0) return null;
  return (
    <div style={{ maxWidth: "62ch" }}>
      {paragraphs.map((p, i) => (
        <p
          key={i}
          className={serif ? "serif" : undefined}
          style={{
            fontFamily: serif ? undefined : font.sans,
            fontSize: serif ? 16 : 15,
            lineHeight: 1.8,
            color: color.ink,
            margin: "0 0 1.2rem",
            whiteSpace: "pre-wrap",
          }}
        >
          {p}
        </p>
      ))}
    </div>
  );
}

/**
 * Preview of a long archived body: the opening paragraphs fading into the card
 * surface, then a hand-off into the reader. The detail page shows enough to
 * decide; the reader owns the reading.
 */
function BodyPreview({ item }: { item: ItemDetail }) {
  const body = (item.body ?? "").trim();
  const paragraphs = body.split(/\n\n+/).map((p) => p.trim()).filter(Boolean);
  if (paragraphs.length === 0) return null;

  const shown: string[] = [];
  let chars = 0;
  for (const p of paragraphs) {
    shown.push(p);
    chars += p.length;
    if (shown.length >= EXCERPT_MAX_PARAGRAPHS || chars >= EXCERPT_MAX_CHARS) break;
  }
  const truncated = shown.length < paragraphs.length;
  const minutes = readingMinutes(body);

  if (!truncated || !readableBody(item)) {
    return (
      <div style={{ margin: "22px 0 0" }}>
        <BodyParagraphs paragraphs={paragraphs} />
      </div>
    );
  }

  return (
    <div style={{ margin: "22px 0 0" }}>
      <div className="body-fade">
        <BodyParagraphs paragraphs={shown} />
      </div>
      <Link
        href={`/item/${item.id}/read`}
        className="serif"
        style={{
          display: "inline-block",
          fontStyle: "italic",
          fontSize: 16,
          color: color.cobalt,
          textDecoration: "none",
          marginTop: 2,
        }}
      >
        Keep reading{minutes ? ` — ${minutes} min` : ""} →
      </Link>
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

/**
 * Right-hand rail: what the machine extracted (palette, tags, places), then
 * what you've woven (your tags, links), with the archive assurance pinned to
 * the bottom so it reads as the rail's colophon rather than another section.
 */
function Rail({ item, places }: { item: ItemDetail; places: Place[] }) {
  const tags = item.tags ?? [];
  const colors =
    item.palette && item.palette.length > 0
      ? item.palette
      : derivedPalette(`${item.title ?? ""} ${tags.join(" ")}`.trim() || item.cardType || "item");
  return (
    <aside className="item-rail">
      <div>
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
            <Rule />
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
        <Rule />
        <TagEditor itemId={item.id} userTags={item.userTags ?? []} />
        <PlacesSection itemId={item.id} places={places} />
        <Rule />
        <RailLinks itemId={item.id} />
        {item.feedId ? (
          <>
            <Rule />
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
      </div>
      <div style={{ marginTop: "auto", paddingTop: 22 }}>
        <Rule />
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
      </div>
    </aside>
  );
}

/**
 * Kinds that get a lead-image hero on the detail page's fallback branch (the
 * quote/note/image kinds render their own layout earlier and never consult
 * this). A Record over the whole CardKind union so adding a card type forces
 * a compile-time decision here instead of silently defaulting to no hero.
 */
const HERO_KINDS: Record<CardKind, boolean> = {
  article: true,
  quote: false,
  image: false,
  product: true,
  note: false,
  video: true,
  tweet: false,
  book: true,
  recipe: true,
  repo: true,
};

/** The type-aware content below the title and actions. */
function ReaderContent({ item }: { item: ItemDetail }) {
  const kind = cardKind(item.cardType);
  const gradient = typeGradient[kind];

  if (item.status === "pending") {
    return (
      <>
        <p style={{ fontFamily: font.mono, fontSize: "0.82rem", color: color.cobalt, margin: "20px 0 0" }}>
          Still enriching…
        </p>
        <ActionRow item={item} />
      </>
    );
  }

  if (item.status === "failed") {
    return (
      <>
        <p style={{ fontFamily: font.sans, fontSize: 14, color: color.inkMuted, margin: "20px 0 0" }}>
          Enrichment failed for this item.
        </p>
        <ActionRow item={item} />
      </>
    );
  }

  if (kind === "quote") {
    return (
      <>
        <QuoteReader text={item.body || item.summary || item.title || ""} />
        <ActionRow item={item} />
      </>
    );
  }

  if (kind === "note") {
    const body = (item.body || item.summary || item.title || "").trim();
    const paragraphs = body.split(/\n\n+/).map((p) => p.trim()).filter(Boolean);
    return (
      <>
        <div style={{ margin: "20px 0 0" }}>
          <BodyParagraphs paragraphs={paragraphs} serif />
        </div>
        <ActionRow item={item} />
      </>
    );
  }

  if (kind === "image") {
    return (
      <>
        <ActionRow item={item} />
        <div style={{ margin: "22px 0 0" }}>
          <ReaderImage src={assetSrc(item.leadImageUrl)} alt={item.title ?? "saved image"} gradient={gradient} />
        </div>
      </>
    );
  }

  // Kinds that arrive here: quote / note / image are handled by earlier
  // branches and pending/failed return before this point, so only
  // article / product / book / recipe / video / tweet / repo reach this
  // fallback. `HERO_KINDS` is a Record over the full CardKind union (not just
  // the ones that reach here) so the compiler forces a decision when card
  // type #11 arrives.
  const showHero = HERO_KINDS[kind];
  return (
    <>
      <ActionRow item={item} />
      {showHero && item.leadImageUrl ? (
        <div style={{ margin: "24px 0 0" }}>
          <ReaderImage src={assetSrc(item.leadImageUrl)} alt={item.title ? `${item.title}` : "lead image"} gradient={gradient} />
        </div>
      ) : null}
      {item.summary ? <SummaryLead>{item.summary}</SummaryLead> : null}
      {item.body ? <BodyPreview item={item} /> : null}
    </>
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
        padding: "clamp(16px, 4vw, 44px) clamp(12px, 3vw, 24px)",
      }}
    >
      <article
        style={{
          width: 980,
          maxWidth: "100%",
          background: color.cardSurface,
          borderRadius: 14,
          border: `1px solid ${color.hairline}`,
          overflow: "hidden",
          boxShadow: "0 2px 6px rgba(28,26,22,.06), 0 30px 70px -42px rgba(28,26,22,.55)",
        }}
      >
        {/* The screen-title mark: same terracotta hairline The Mind wears. */}
        <div
          aria-hidden
          style={{
            height: 2,
            background: `linear-gradient(90deg, ${color.terracotta}, ${color.terracotta} 38%, transparent)`,
          }}
        />
        <header className="item-chrome">
          <Link
            href="/"
            style={{ fontFamily: font.mono, fontSize: "0.78rem", color: color.cobalt, textDecoration: "none" }}
          >
            ← The Mind
          </Link>
          <div style={{ display: "flex", alignItems: "center", gap: 14, flexWrap: "wrap" }}>
            <PinButton itemId={item.id} pinned={!!item.pinnedAt} />
            <KindleButton target="item" id={item.id} />
            <span aria-hidden style={{ width: 1, height: 14, background: color.hairline }} />
            <DeleteButton id={item.id} />
          </div>
        </header>
        <div className="item-columns">
          <div className="item-main">
            <div className="meta" style={{ color: color.inkFaint }}>
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
        </div>
      </article>
    </main>
  );
}

import { tokens } from "@openmind/ui";
import type { CSSProperties, ReactNode } from "react";
import { assetSrc } from "../lib/assets";
import { cardKind, domainOf, typeAccent, typeGradient, typeLabel } from "../lib/cards";
import { derivedPalette } from "../lib/palette";
import { unionTags } from "../lib/tags";
import type { Item } from "../lib/types";
import { Palette } from "./Palette";

const { color, font } = tokens;

function clamp(lines: number): CSSProperties {
  return {
    display: "-webkit-box",
    WebkitLineClamp: lines,
    WebkitBoxOrient: "vertical",
    overflow: "hidden",
  };
}

function Enriching() {
  return (
    <p
      style={{
        fontFamily: font.mono,
        fontSize: "0.72rem",
        letterSpacing: ".02em",
        color: color.cobalt,
        margin: "10px 0 0",
      }}
    >
      enriching…
    </p>
  );
}

function Tags({ tags }: { tags?: string[] }) {
  if (!tags || tags.length === 0) return null;
  return (
    <div style={{ display: "flex", gap: 5, marginTop: 11, flexWrap: "wrap" }}>
      {tags.slice(0, 4).map((t) => (
        <span key={t} className="tag">
          {t}
        </span>
      ))}
    </div>
  );
}

function Footer({
  dots,
  meta,
  metaColor,
}: {
  dots: string[];
  meta: string;
  metaColor?: string;
}) {
  return (
    <div style={{ display: "flex", alignItems: "center", gap: 8, marginTop: 11 }}>
      <Palette colors={dots} />
      <span className="meta" style={{ marginLeft: "auto", ...(metaColor ? { color: metaColor } : {}) }}>
        {meta}
      </span>
    </div>
  );
}

/**
 * Lead image with a gradient underlay. `src` is painted as a CSS
 * `background-image` layered over the gradient (never an `<img>`), so a
 * missing image (no src) shows the gradient cleanly and a broken image
 * (404) falls back to it too — a failed background-image URL simply paints
 * nothing, unlike an `<img>`, which shows the browser's broken-image glyph.
 */
function LeadImage({
  src,
  alt,
  gradient,
  height,
  overlay,
  caption,
  children,
}: {
  src?: string;
  alt: string;
  gradient: string;
  height: number;
  overlay?: string;
  caption?: string;
  children?: ReactNode;
}) {
  return (
    <div style={{ position: "relative", height, background: gradient, overflow: "hidden" }}>
      {overlay ? <div style={{ position: "absolute", inset: 0, background: overlay }} /> : null}
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
      {caption ? (
        <span
          className="meta"
          style={{ position: "absolute", left: 12, bottom: 10, color: "rgba(255,255,255,.85)" }}
        >
          {caption}
        </span>
      ) : null}
      {children}
    </div>
  );
}

/**
 * Thin top accent rule for text-forward (imageless) cards: a subtle nod to
 * the card type's accent colour in place of the gradient hero slab.
 */
function TopAccent({ color: accent }: { color: string }) {
  return <div style={{ height: 3, background: accent }} />;
}

function serifTitle(size: number): CSSProperties {
  return {
    fontFamily: font.quote,
    fontSize: size,
    fontWeight: 600,
    lineHeight: 1.2,
    letterSpacing: "-.01em",
    color: color.ink,
    margin: 0,
  };
}

const summaryStyle: CSSProperties = {
  fontFamily: font.sans,
  fontSize: 12,
  lineHeight: 1.45,
  color: color.inkMuted,
  margin: "6px 0 0",
  ...clamp(4),
};

const specStyle: CSSProperties = {
  fontFamily: font.mono,
  fontSize: 10,
  letterSpacing: ".02em",
  color: color.inkFaintAlt,
  margin: "6px 0 0",
};

export function ItemCard({ item }: { item: Item }) {
  const kind = cardKind(item.cardType);
  const pending = item.status === "pending";
  const domain = domainOf(item.url);
  const img = assetSrc(item.leadImageUrl);
  // Real extracted palette when present; otherwise a deterministic placeholder
  // derived from the title + tags (see lib/palette).
  const tags = item.tags ?? [];
  const dots =
    item.palette && item.palette.length > 0
      ? item.palette
      : derivedPalette(`${item.title ?? ""} ${tags.join(" ")}`.trim() || kind);
  const gradient = typeGradient[kind];
  const accent = typeAccent[kind];
  const withDomain = (label: string) => (domain ? `${label} · ${domain}` : label);
  const imageAlt = item.title ?? "saved image";
  const videoAlt = item.title ? `${item.title} (video thumbnail)` : "video thumbnail";

  if (kind === "quote") {
    const text = item.summary ?? item.title ?? "";
    const attribution = item.summary && item.title ? item.title : null;
    return (
      <article className="card" style={{ background: color.ink }}>
        <div style={{ padding: "22px 18px 16px" }}>
          <div
            className="serif"
            style={{ fontSize: 40, fontWeight: 600, lineHeight: 1, color: color.gold, height: 20, overflow: "hidden" }}
          >
            “
          </div>
          <div
            className="serif"
            style={{ fontSize: 20, lineHeight: 1.32, color: color.paper, fontStyle: "italic", fontWeight: 400, ...clamp(6) }}
          >
            {text}
          </div>
          <div className="meta" style={{ color: color.inkFaintAlt, marginTop: 14 }}>
            {attribution ? `${attribution} — Quote` : "Quote"}
          </div>
          <div style={{ display: "flex", gap: 5, marginTop: 12 }}>
            <Palette colors={dots} />
          </div>
          {pending ? <Enriching /> : null}
        </div>
      </article>
    );
  }

  if (kind === "image") {
    return (
      <article className="card">
        <LeadImage
          src={img}
          alt={imageAlt}
          gradient={gradient}
          height={210}
          overlay="radial-gradient(circle at 70% 25%, rgba(255,255,255,.3), transparent 45%)"
          caption={item.title ?? undefined}
        />
        <div style={{ padding: "11px 13px", display: "flex", alignItems: "center", gap: 8 }}>
          <Palette colors={dots} />
          <span className="meta" style={{ marginLeft: "auto" }}>
            {withDomain("Image")}
          </span>
        </div>
        {pending ? <div style={{ padding: "0 13px 12px" }}><Enriching /></div> : null}
      </article>
    );
  }

  if (kind === "note") {
    const text = item.summary ?? item.title ?? "Untitled note";
    return (
      <article className="card" style={{ background: color.noteSurface }}>
        <div style={{ padding: "14px 15px" }}>
          <div className="meta" style={{ color: color.gold }}>
            Note
          </div>
          <div
            className="serif"
            style={{ fontSize: 15, lineHeight: 1.4, marginTop: 8, color: color.ink, ...clamp(8) }}
          >
            {text}
          </div>
          <Tags tags={unionTags(item.tags, item.userTags)} />
          <div style={{ display: "flex", gap: 5, marginTop: 12 }}>
            <Palette colors={dots} />
          </div>
          {pending ? <Enriching /> : null}
        </div>
      </article>
    );
  }

  if (kind === "tweet") {
    const name = item.title ?? domain ?? "Saved post";
    const text = item.summary ?? "";
    return (
      <article className="card">
        <div style={{ padding: "14px 15px" }}>
          <div style={{ display: "flex", alignItems: "center", gap: 9 }}>
            <div
              style={{
                width: 30,
                height: 30,
                borderRadius: "50%",
                background: `linear-gradient(135deg, ${color.cobalt}, ${color.green})`,
                flex: "none",
              }}
            />
            <div style={{ minWidth: 0 }}>
              <div style={{ fontFamily: font.sans, fontSize: 12.5, fontWeight: 600, color: color.ink }}>{name}</div>
              {domain ? (
                <div className="meta" style={{ textTransform: "none", letterSpacing: ".02em", color: color.inkFaint }}>
                  {domain}
                </div>
              ) : null}
            </div>
          </div>
          {text ? (
            <div style={{ fontFamily: font.sans, fontSize: 13.5, lineHeight: 1.5, marginTop: 10, color: color.ink, ...clamp(6) }}>
              {text}
            </div>
          ) : null}
          <Footer dots={dots} meta={withDomain("Post")} />
          {pending ? <Enriching /> : null}
        </div>
      </article>
    );
  }

  if (kind === "video") {
    return (
      <article className="card">
        <LeadImage src={img} alt={videoAlt} gradient={gradient} height={120}>
          <div
            style={{
              position: "absolute",
              inset: 0,
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
              pointerEvents: "none",
            }}
          >
            <div
              style={{
                width: 38,
                height: 38,
                borderRadius: "50%",
                background: color.paper,
                display: "flex",
                alignItems: "center",
                justifyContent: "center",
              }}
            >
              <div
                style={{
                  width: 0,
                  height: 0,
                  borderLeft: `11px solid ${color.ink}`,
                  borderTop: "7px solid transparent",
                  borderBottom: "7px solid transparent",
                  marginLeft: 3,
                }}
              />
            </div>
          </div>
        </LeadImage>
        <div style={{ padding: "12px 14px" }}>
          {item.title ? <h2 style={serifTitle(16)}>{item.title}</h2> : null}
          {item.summary ? <p style={specStyle}>{item.summary}</p> : null}
          <Footer dots={dots} meta={withDomain("Video")} />
          {pending ? <Enriching /> : null}
        </div>
      </article>
    );
  }

  if (kind === "product") {
    return (
      <article className="card">
        {img ? (
          <LeadImage src={img} alt={imageAlt} gradient={gradient} height={150} />
        ) : (
          <TopAccent color={accent} />
        )}
        <div style={{ padding: "13px 14px" }}>
          {item.title ? <h2 style={serifTitle(16)}>{item.title}</h2> : null}
          {item.summary ? <p style={{ ...specStyle, ...clamp(2) }}>{item.summary}</p> : null}
          <Tags tags={unionTags(item.tags, item.userTags)} />
          <Footer dots={dots} meta={withDomain("Product")} />
          {pending ? <Enriching /> : null}
        </div>
      </article>
    );
  }

  if (kind === "book") {
    return (
      <article className="card">
        {img ? (
          <LeadImage src={img} alt={imageAlt} gradient={gradient} height={180} />
        ) : (
          <TopAccent color={accent} />
        )}
        <div style={{ padding: "12px 14px" }}>
          {item.title ? <h2 style={serifTitle(15.5)}>{item.title}</h2> : null}
          <Tags tags={unionTags(item.tags, item.userTags)} />
          <Footer dots={dots} meta={withDomain("Book")} />
          {pending ? <Enriching /> : null}
        </div>
      </article>
    );
  }

  if (kind === "recipe") {
    return (
      <article className="card">
        {img ? (
          <LeadImage src={img} alt={imageAlt} gradient={gradient} height={96} />
        ) : (
          <TopAccent color={accent} />
        )}
        <div style={{ padding: "13px 14px" }}>
          {item.title ? <h2 style={serifTitle(16)}>{item.title}</h2> : null}
          {item.summary ? (
            <div style={{ fontFamily: font.mono, fontSize: 12, lineHeight: 1.7, color: color.inkMuted, marginTop: 9, ...clamp(6) }}>
              {item.summary}
            </div>
          ) : null}
          <Footer dots={dots} meta={withDomain("Recipe")} />
          {pending ? <Enriching /> : null}
        </div>
      </article>
    );
  }

  // article (and default for any unknown type)
  return (
    <article className="card">
      {img ? (
        <LeadImage src={img} alt={imageAlt} gradient={gradient} height={118} />
      ) : (
        <TopAccent color={accent} />
      )}
      <div style={{ padding: "13px 14px" }}>
        {item.title ? <h2 style={{ ...serifTitle(17), lineHeight: 1.2 }}>{item.title}</h2> : null}
        {item.summary ? <p style={summaryStyle}>{item.summary}</p> : null}
        <Tags tags={unionTags(item.tags, item.userTags)} />
        <Footer dots={dots} meta={withDomain(typeLabel[kind])} />
        {pending ? <Enriching /> : null}
      </div>
    </article>
  );
}

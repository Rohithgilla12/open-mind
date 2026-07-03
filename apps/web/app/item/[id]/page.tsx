import { tokens } from "@openmind/ui";
import Link from "next/link";
import { notFound } from "next/navigation";
import type { CSSProperties, ReactNode } from "react";
import { apiFetch } from "../../../lib/api";
import type { ItemDetail } from "../../../lib/types";
import { DeleteButton } from "./DeleteButton";

function domainOf(url: string): string | null {
  if (!url) return null;
  try {
    return new URL(url).hostname.replace(/^www\./, "");
  } catch {
    return null;
  }
}

const linkStyle: CSSProperties = {
  fontFamily: tokens.font.mono,
  fontSize: "0.78rem",
  color: tokens.color.cobalt,
  textDecoration: "none",
};

function SourceLink({ url }: { url: string }) {
  if (!url) return null;
  const domain = domainOf(url);
  return (
    <a href={url} target="_blank" rel="noreferrer" style={linkStyle}>
      {domain ? `${domain} ` : ""}Open original ↗
    </a>
  );
}

function QuoteBlock({ text }: { text: string }) {
  return (
    <p
      style={{
        fontFamily: tokens.font.quote,
        fontStyle: "italic",
        fontSize: "1.25rem",
        lineHeight: 1.6,
        color: tokens.color.ink,
        margin: 0,
        maxWidth: "65ch",
        whiteSpace: "pre-wrap",
      }}
    >
      {text}
    </p>
  );
}

function Body({ body }: { body: string }) {
  const paragraphs = body.split("\n\n").filter((p) => p.trim().length > 0);
  return (
    <div style={{ maxWidth: "65ch" }}>
      {paragraphs.map((p, i) => (
        <p
          key={i}
          style={{
            fontFamily: tokens.font.sans,
            fontSize: "1rem",
            lineHeight: 1.7,
            color: tokens.color.ink,
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

function DetailBody({ item }: { item: ItemDetail }) {
  if (item.status === "pending") {
    return (
      <p
        style={{
          fontFamily: tokens.font.mono,
          fontSize: "0.82rem",
          color: tokens.color.cobalt,
          margin: 0,
        }}
      >
        Still enriching…
      </p>
    );
  }

  if (item.status === "failed") {
    return (
      <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
        <p
          style={{
            fontFamily: tokens.font.sans,
            fontSize: "0.9rem",
            color: tokens.color.ink,
            opacity: 0.6,
            margin: 0,
          }}
        >
          Enrichment failed for this item.
        </p>
        <SourceLink url={item.url} />
      </div>
    );
  }

  if (item.cardType === "note") {
    return <QuoteBlock text={item.body || item.summary || item.title || ""} />;
  }

  if (item.cardType === "image") {
    return (
      <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
        {item.leadImageUrl ? (
          // eslint-disable-next-line @next/next/no-img-element
          <img
            src={item.leadImageUrl}
            alt={item.title ?? "saved image"}
            loading="lazy"
            style={{ maxWidth: "100%", height: "auto", borderRadius: 8 }}
          />
        ) : null}
        {item.title ? (
          <h1
            style={{
              fontFamily: tokens.font.sans,
              fontSize: "1.2rem",
              fontWeight: 600,
              color: tokens.color.ink,
              margin: 0,
            }}
          >
            {item.title}
          </h1>
        ) : null}
        <SourceLink url={item.url} />
      </div>
    );
  }

  if (item.cardType === "tweet" || item.cardType === "video") {
    return (
      <div style={{ display: "flex", flexDirection: "column", gap: 16 }}>
        {item.cardType === "video" && item.leadImageUrl ? (
          // eslint-disable-next-line @next/next/no-img-element
          <img
            src={item.leadImageUrl}
            alt={item.title ? `${item.title} (video thumbnail)` : "video thumbnail"}
            loading="lazy"
            style={{ maxWidth: "100%", height: "auto", borderRadius: 8 }}
          />
        ) : null}
        <QuoteBlock text={item.summary || item.body || item.title || ""} />
        <SourceLink url={item.url} />
      </div>
    );
  }

  // default: article / product / recipe / book / quote
  const domain = domainOf(item.url);
  const chips: ReactNode = item.tags && item.tags.length > 0 ? (
    <div style={{ display: "flex", flexWrap: "wrap", gap: 8, margin: "0.5rem 0 0" }}>
      {item.tags.map((tag) => (
        <span
          key={tag}
          style={{
            fontFamily: tokens.font.mono,
            fontSize: "0.7rem",
            textTransform: "lowercase",
            color: tokens.color.ink,
            opacity: 0.7,
            border: `1px solid ${tokens.color.line}`,
            borderRadius: 999,
            padding: "2px 8px",
          }}
        >
          {tag}
        </span>
      ))}
    </div>
  ) : null;

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 20 }}>
      {item.leadImageUrl ? (
        // eslint-disable-next-line @next/next/no-img-element
        <img
          src={item.leadImageUrl}
          alt={item.title ?? "lead image"}
          loading="lazy"
          style={{ maxWidth: "100%", height: "auto", borderRadius: 8 }}
        />
      ) : null}
      <div>
        {item.title ? (
          <h1
            style={{
              fontFamily: tokens.font.sans,
              fontSize: "1.6rem",
              fontWeight: 600,
              lineHeight: 1.3,
              color: tokens.color.ink,
              margin: "0 0 0.5rem",
            }}
          >
            {item.title}
          </h1>
        ) : null}
        {domain ? (
          <a href={item.url} target="_blank" rel="noreferrer" style={linkStyle}>
            {domain} · Open original ↗
          </a>
        ) : null}
      </div>
      {item.summary ? (
        <p
          style={{
            fontFamily: tokens.font.sans,
            fontSize: "0.95rem",
            lineHeight: 1.6,
            color: tokens.color.ink,
            margin: 0,
            maxWidth: "65ch",
            borderLeft: `2px solid ${tokens.color.line}`,
            paddingLeft: 16,
          }}
        >
          {item.summary}
        </p>
      ) : null}
      {chips}
      {item.body ? <Body body={item.body} /> : null}
    </div>
  );
}

export default async function ItemPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const res = await apiFetch(`/items/${id}`);
  if (!res.ok) notFound();
  const item = (await res.json()) as ItemDetail;

  return (
    <main style={{ maxWidth: 760, margin: "0 auto", padding: "2rem 1.5rem" }}>
      <div
        style={{
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
          marginBottom: "2rem",
        }}
      >
        <Link href="/" style={linkStyle}>
          ← library
        </Link>
        <DeleteButton id={item.id} />
      </div>
      <DetailBody item={item} />
    </main>
  );
}

import { tokens } from "@openmind/ui";
import type { CSSProperties } from "react";
import type { Item } from "../lib/types";

const cardStyle: CSSProperties = {
  backgroundColor: tokens.color.surface,
  border: `1px solid ${tokens.color.line}`,
  borderRadius: 10,
  padding: 14,
};

const titleStyle: CSSProperties = {
  fontFamily: tokens.font.sans,
  fontSize: "0.95rem",
  fontWeight: 600,
  color: tokens.color.ink,
  margin: "0 0 6px",
};

const domainStyle: CSSProperties = {
  fontFamily: tokens.font.mono,
  fontSize: "0.72rem",
  color: tokens.color.ink,
  opacity: 0.5,
  margin: "8px 0 0",
};

function clamp(lines: number): CSSProperties {
  return {
    display: "-webkit-box",
    WebkitLineClamp: lines,
    WebkitBoxOrient: "vertical",
    overflow: "hidden",
  };
}

function domainOf(url: string): string | null {
  if (!url) return null;
  try {
    return new URL(url).hostname.replace(/^www\./, "");
  } catch {
    return null;
  }
}

function Enriching() {
  return (
    <p
      style={{
        fontFamily: tokens.font.mono,
        fontSize: "0.72rem",
        color: tokens.color.cobalt,
        margin: "8px 0 0",
      }}
    >
      enriching…
    </p>
  );
}

export function ItemCard({ item }: { item: Item }) {
  const pending = item.status === "pending";
  const domain = domainOf(item.url);

  if (item.cardType === "note") {
    return (
      <article style={cardStyle}>
        <p
          style={{
            fontFamily: tokens.font.quote,
            fontStyle: "italic",
            fontSize: "1rem",
            color: tokens.color.ink,
            margin: 0,
            ...clamp(8),
          }}
        >
          {item.summary ?? item.title ?? "Untitled note"}
        </p>
        {pending ? <Enriching /> : null}
      </article>
    );
  }

  if (item.cardType === "tweet") {
    return (
      <article style={cardStyle}>
        <p
          style={{
            fontFamily: tokens.font.quote,
            fontStyle: "italic",
            fontSize: "0.95rem",
            color: tokens.color.ink,
            margin: 0,
            ...clamp(8),
          }}
        >
          {item.summary ?? item.title ?? ""}
        </p>
        {domain ? <p style={domainStyle}>{domain}</p> : null}
        {pending ? <Enriching /> : null}
      </article>
    );
  }

  // default: article / product / recipe / book / quote / video / image
  return (
    <article style={cardStyle}>
      {item.title ? <h2 style={titleStyle}>{item.title}</h2> : null}
      {item.summary ? (
        <p
          style={{
            fontFamily: tokens.font.sans,
            fontSize: "0.85rem",
            color: tokens.color.ink,
            opacity: 0.8,
            margin: 0,
            ...clamp(4),
          }}
        >
          {item.summary}
        </p>
      ) : null}
      {!item.title && !item.summary && domain ? (
        <h2 style={titleStyle}>{domain}</h2>
      ) : null}
      {domain ? <p style={domainStyle}>{domain}</p> : null}
      {pending ? <Enriching /> : null}
    </article>
  );
}

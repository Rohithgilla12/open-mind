"use client";

import { tokens } from "@openmind/ui";
import Link from "next/link";
import { type ReactNode, useCallback, useEffect, useState } from "react";
import { domainOf } from "../lib/cards";
import { relativeTime } from "../lib/relative-time";
import { renderInlineMarkdown } from "../lib/text";
import type { Feed, Item } from "../lib/types";

const { color, font } = tokens;

/** Feed chips shown before the "+N more" disclosure. */
const CHIP_LIMIT = 8;

/** Keep/Kept toggle for a single feed-river row. PATCHes `{kept}` through the items proxy. */
function KeepToggle({ itemId, kept, onChange }: { itemId: string; kept: boolean; onChange: (kept: boolean) => void }) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function toggle() {
    const next = !kept;
    setBusy(true);
    setError(null);
    try {
      const res = await fetch(`/api/items/${itemId}`, {
        method: "PATCH",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ kept: next }),
      });
      if (!res.ok) throw new Error(`kept update failed: ${res.status}`);
      onChange(next);
    } catch (err) {
      console.error("feed river keep toggle failed", { itemId, err });
      setError("Couldn't update. Try again.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <span style={{ display: "inline-flex", alignItems: "center", gap: 10 }}>
      {error ? (
        <span
          aria-live="polite"
          style={{ fontFamily: font.mono, fontSize: 10, letterSpacing: ".04em", color: color.danger }}
        >
          {error}
        </span>
      ) : null}
      <button
        type="button"
        className="feed-keep"
        onClick={toggle}
        disabled={busy}
        aria-pressed={kept}
        aria-label={kept ? "Unkeep this item" : "Keep this item"}
      >
        {kept ? "Kept" : "Keep"}
      </button>
    </span>
  );
}

function Row({ item, feedTitle, onKeptChange }: { item: Item; feedTitle: string; onKeptChange: (kept: boolean) => void }) {
  const domain = domainOf(item.url);
  // Source, time and domain all belong to the same secondary register — the
  // source used to sit above the title in terracotta, which spent the page's
  // one accent colour once per row.
  const provenance = [relativeTime(item.createdAt), domain, feedTitle].filter(Boolean).join(" · ");

  return (
    <li className="feed-row">
      <Link
        href={`/item/${item.id}`}
        className="serif feed-row-title"
        style={{
          display: "block",
          fontSize: 19,
          fontWeight: 600,
          lineHeight: 1.3,
          letterSpacing: "-.015em",
          color: color.ink,
          textDecoration: "none",
          textWrap: "pretty",
          maxWidth: "42ch",
        }}
      >
        {item.title || domain || item.url}
      </Link>
      {item.summary ? (
        <p
          className="feed-row-summary"
          style={{
            fontFamily: font.sans,
            fontSize: 13.5,
            lineHeight: 1.6,
            color: color.inkMuted,
            margin: "10px 0 0",
            maxWidth: "68ch",
            overflow: "hidden",
            display: "-webkit-box",
            WebkitLineClamp: 2,
            WebkitBoxOrient: "vertical",
          }}
        >
          {renderInlineMarkdown(item.summary)}
        </p>
      ) : null}
      <div className="feed-row-foot">
        <span
          className="meta"
          style={{ textTransform: "none", letterSpacing: ".05em", color: color.inkFaintAlt, minWidth: 0 }}
        >
          {provenance}
        </span>
        <KeepToggle itemId={item.id} kept={!!item.keptAt} onChange={onKeptChange} />
      </div>
    </li>
  );
}

/** The river's surface: ruled paper, house content padding. */
function RiverSurface({ children }: { children: ReactNode }) {
  return (
    <div style={{ position: "relative", flex: 1 }}>
      <div className="paper-texture" style={{ position: "absolute", inset: 0, pointerEvents: "none" }} />
      <div style={{ position: "relative", padding: "22px 28px 40px" }}>{children}</div>
    </div>
  );
}

/**
 * The Feed river: a reverse-chron list of everything your subscriptions
 * brought in. `feeds` arrives from the server (id→title map, doubling as the
 * per-feed filter chips); the items themselves are fetched client-side from
 * `/api/feed` because the filter re-queries without a navigation. Empty and
 * load-failure states follow the house pattern used elsewhere (see RelatedRail).
 */
export function FeedRiver({ feeds }: { feeds: Feed[] }) {
  const [items, setItems] = useState<Item[] | null>(null);
  const [activeFeedId, setActiveFeedId] = useState<string | undefined>(undefined);
  const [showAllChips, setShowAllChips] = useState(false);
  const [loadFailed, setLoadFailed] = useState(false);
  const [loadAttempt, setLoadAttempt] = useState(0);

  useEffect(() => {
    let cancelled = false;
    setLoadFailed(false);
    const params = new URLSearchParams();
    if (activeFeedId) params.set("feedId", activeFeedId);
    const qs = params.toString();
    fetch(`/api/feed${qs ? `?${qs}` : ""}`)
      .then(async (res) => {
        if (!res.ok) throw new Error(`failed to load feed: ${res.status}`);
        return (await res.json()) as Item[];
      })
      .then((data) => {
        if (!cancelled) setItems(data);
      })
      .catch((err) => {
        console.error("failed to load feed river", err);
        if (!cancelled) setLoadFailed(true);
      });
    return () => {
      cancelled = true;
    };
  }, [activeFeedId, loadAttempt]);

  const titleFor = useCallback(
    (feedId?: string | null) => {
      if (!feedId) return "Feed";
      return feeds.find((f) => f.id === feedId)?.title?.trim() || "Feed";
    },
    [feeds],
  );

  const setKept = useCallback((itemId: string, kept: boolean) => {
    setItems((prev) =>
      prev
        ? prev.map((it) => (it.id === itemId ? { ...it, keptAt: kept ? new Date().toISOString() : null } : it))
        : prev,
    );
  }, []);

  const activeChip = {
    background: color.ink,
    color: color.paper,
    borderColor: color.ink,
  } as const;

  // Chips wrap, so an unbounded subscription list would push the river below
  // the fold. Show a first rank and disclose the rest on request; a chip that
  // is currently active always stays visible so the filter can be cleared.
  const activeIsHidden = !!activeFeedId && feeds.findIndex((f) => f.id === activeFeedId) >= CHIP_LIMIT;
  const visibleChips = showAllChips || activeIsHidden ? feeds : feeds.slice(0, CHIP_LIMIT);
  const hiddenCount = feeds.length - visibleChips.length;

  const body = loadFailed ? (
    <button
      type="button"
      onClick={() => setLoadAttempt((n) => n + 1)}
      style={{
        display: "block",
        font: `500 11px/1 ${font.mono}`,
        letterSpacing: ".02em",
        color: color.inkFaint,
        background: "none",
        border: `1px solid ${color.hairline}`,
        borderRadius: 20,
        padding: "8px 14px",
        cursor: "pointer",
      }}
    >
      Couldn&apos;t load the feed — retry
    </button>
  ) : items === null ? (
    <p className="meta" style={{ textTransform: "none", letterSpacing: ".05em", color: color.inkFaintAlt }}>
      Loading…
    </p>
  ) : items.length === 0 && activeFeedId ? (
    <p className="meta" style={{ textTransform: "none", letterSpacing: ".05em", color: color.inkFaintAlt }}>
      Nothing from this feed yet.
    </p>
  ) : items.length === 0 ? (
    <div
      style={{
        maxWidth: 560,
        padding: "32px 28px",
        background: color.cardSurface,
        border: `1.5px dashed ${color.hairline}`,
        borderRadius: 11,
      }}
    >
      {/* Two different silences: no subscriptions at all, versus subscriptions
          that haven't published yet. The old copy told a reader with twelve
          feeds to go and subscribe to something. */}
      <div className="serif" style={{ fontSize: 19, fontWeight: 600, color: color.ink, marginBottom: 10 }}>
        {feeds.length > 0 ? "Nothing has come in yet" : "Nothing here yet"}
      </div>
      <p style={{ fontFamily: font.sans, fontSize: 14, lineHeight: 1.6, color: color.inkMuted, margin: 0 }}>
        {feeds.length > 0 ? (
          <>
            None of your subscriptions have published since you added them.{" "}
            <Link href="/feeds" style={{ color: color.cobalt, textDecoration: "none" }}>
              Add another
            </Link>
            .
          </>
        ) : (
          <>
            <Link href="/feeds" style={{ color: color.cobalt, textDecoration: "none" }}>
              Subscribe to something worth reading
            </Link>
            {" "}— new posts will show up here as they come in.
          </>
        )}
      </p>
    </div>
  ) : (
    <ul className="feed-river" style={{ listStyle: "none", margin: 0, maxWidth: 780 }}>
      {items.map((item) => (
        <Row
          key={item.id}
          item={item}
          feedTitle={titleFor(item.feedId)}
          onKeptChange={(kept) => setKept(item.id, kept)}
        />
      ))}
    </ul>
  );

  return (
    <>
      {feeds.length > 0 ? (
        <div className="feed-strip">
          {/* A filter set, not a tab set: there is no tabpanel and no arrow-key
              navigation, so the old role="tablist"/role="tab" pair promised a
              keyboard contract that was never implemented. Toggle buttons in a
              labelled group describe what these actually are. */}
          <div className="feed-strip-chips" role="group" aria-label="Filter by feed">
            <button
              type="button"
              onClick={() => setActiveFeedId(undefined)}
              className="chip"
              aria-pressed={!activeFeedId}
              style={!activeFeedId ? activeChip : undefined}
            >
              All
            </button>
            {visibleChips.map((feed) => {
              const label = feed.title?.trim() || domainOf(feed.siteUrl || feed.url) || "Feed";
              const isActive = feed.id === activeFeedId;
              return (
                <button
                  key={feed.id}
                  type="button"
                  onClick={() => setActiveFeedId(feed.id)}
                  className="chip"
                  aria-pressed={isActive}
                  // A long title is ellipsised on narrow screens, so the full
                  // one has to stay reachable somewhere.
                  title={label}
                  style={isActive ? activeChip : undefined}
                >
                  {label}
                </button>
              );
            })}
            {hiddenCount > 0 ? (
              <button
                type="button"
                onClick={() => setShowAllChips(true)}
                className="feed-strip-more"
                aria-expanded={false}
              >
                +{hiddenCount} more
              </button>
            ) : showAllChips && !activeIsHidden && feeds.length > CHIP_LIMIT ? (
              // Disclosure both ways — expanding used to be a one-way door that
              // left a long subscription list wrapped across the page for good.
              <button type="button" onClick={() => setShowAllChips(false)} className="feed-strip-more" aria-expanded>
                Fewer
              </button>
            ) : null}
          </div>
          {/* The sidebar has no Feeds entry, so before this the subscription
              list was reachable only from the river's empty state. */}
          <Link href="/feeds" className="meta meta-link" style={{ flex: "none", color: color.inkFaintAlt }}>
            Manage subscriptions
          </Link>
        </div>
      ) : null}

      <RiverSurface>{body}</RiverSurface>
    </>
  );
}

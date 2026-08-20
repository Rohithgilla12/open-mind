"use client";

import { useEffect, useRef, useState, type ReactNode } from "react";
import { tokens } from "@openmind/ui";
import { isFeedOnly } from "../lib/cards";
import { FeedDivider } from "./FeedDivider";
import { Grid } from "./Grid";
import { useLiveSearch } from "./LiveSearch";

const { color, font } = tokens;

/** Visually hidden, still read aloud. */
const srOnly = {
  position: "absolute",
  width: 1,
  height: 1,
  overflow: "hidden",
  clip: "rect(0 0 0 0)",
} as const;

/** How long results must sit still before they are announced. */
const ANNOUNCE_DELAY_MS = 500;

const count = (n: number) => n.toLocaleString("en-GB");

/**
 * The results area of the Mind: the server-rendered river until the reader
 * starts typing, then live matches in its place.
 *
 * `fallback` is the server tree, passed in as a prop so this component can hand
 * the area back without re-implementing the river, the paging, or the
 * search-result split.
 */
export function LiveResults({ fallback }: { fallback: ReactNode }) {
  const { active, answer, serverPending, indexed, indexDone, query } = useLiveSearch();
  const region = useRef<HTMLDivElement | null>(null);
  const wasActive = useRef(false);
  const [announcement, setAnnouncement] = useState("");

  // `answer` is null only until the very first local answer lands (a task after
  // the first keystroke). Rendering the live tree before then would flash a
  // truthful-looking "0 matches"; holding the fallback until there is something
  // to show means the river gives way to the matches in one transition.
  const showLive = active && answer !== null;
  const items = answer?.items ?? [];
  const library = items.filter((i) => !isFeedOnly(i));
  const feed = items.filter(isFeedOnly);
  const ranked = answer?.phase === "server";

  // A reader deep in the river who starts typing would otherwise be left
  // staring at whitespace where the river used to be. Only pulls up when the
  // results have actually scrolled off the top — never fights a reader who can
  // already see them.
  //
  // Keyed on `active` (the keystroke) rather than on the results arriving: a
  // scroll during a running view transition would drag the cards away from the
  // positions the browser snapshotted, so it has to happen before the first
  // morph, not inside it.
  useEffect(() => {
    if (active && !wasActive.current) {
      const rect = region.current?.getBoundingClientRect();
      if (rect && rect.top < 0) region.current?.scrollIntoView({ block: "start" });
    }
    wasActive.current = active;
  }, [active]);

  // Debounced: announcing a count on every keystroke turns a screen reader
  // into a metronome.
  const summary = showLive ? `${count(items.length)} matches for ${query.trim()}` : "";
  useEffect(() => {
    if (!summary) {
      setAnnouncement("");
      return;
    }
    const timer = setTimeout(() => setAnnouncement(summary), ANNOUNCE_DELAY_MS);
    return () => clearTimeout(timer);
  }, [summary]);

  if (!showLive) return <div ref={region}>{fallback}</div>;

  return (
    <div ref={region}>
      <p aria-live="polite" style={srOnly}>
        {announcement}
      </p>

      {/* .meta's own #A39C8B is the colour of static page chrome; this line is
          live information about the reader's own search, so it takes the muted
          ink instead — 9.5px uppercase mono needs the contrast to be read, not
          just noticed. */}
      <div
        className="meta"
        style={{
          display: "flex",
          alignItems: "center",
          gap: 7,
          margin: "0 0 18px",
          color: color.inkMuted,
        }}
      >
        {ranked ? (
          <span
            aria-hidden
            style={{
              width: 6,
              height: 6,
              borderRadius: "50%",
              background: color.cobalt,
              flex: "none",
            }}
          />
        ) : null}
        <span>
          {count(items.length)} {items.length === 1 ? "match" : "matches"}
          {ranked
            ? " · ranked by meaning"
            : indexDone
              ? " · from memory"
              : ` · from memory · ${count(indexed)} saves indexed`}
        </span>
      </div>

      {library.length > 0 ? (
        <Grid items={library} morph />
      ) : (
        <p
          style={{
            fontFamily: font.quote,
            fontStyle: "italic",
            fontSize: "1.25rem",
            lineHeight: 1.5,
            color: color.inkMuted,
            margin: "1.5rem 0 0",
            maxWidth: "48ch",
          }}
        >
          {feed.length > 0 ? (
            <>Nothing in your Mind matches “{query.trim()}” — these came through your feeds.</>
          ) : serverPending || !ranked ? (
            <>No words match “{query.trim()}” — looking for meaning…</>
          ) : (
            <>Nothing matches “{query.trim()}” — not by word, and not by meaning.</>
          )}
        </p>
      )}

      {feed.length > 0 ? (
        <>
          <FeedDivider count={feed.length} />
          <Grid items={feed} morph />
        </>
      ) : null}
    </div>
  );
}

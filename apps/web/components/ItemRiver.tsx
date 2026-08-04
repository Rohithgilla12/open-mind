"use client";

import { useCallback, useRef, useState } from "react";
import { Grid } from "./Grid";
import { LoadMore } from "./LoadMore";
import { appendPage, initialPagedState } from "../lib/pages";
import type { Item, ItemPage } from "../lib/types";

/**
 * The Mind's paged river. Page one arrives from the server render, so first
 * paint is unchanged; later pages are fetched client-side and appended.
 *
 * Each page renders as its own <Grid>, i.e. its own .mind-col block. Appending
 * into one shared block would make the browser rebalance all columns, moving
 * cards the reader has already passed (measured 8 of 12 on a 12-card page).
 */
export function ItemRiver({
  initialItems,
  initialCursor,
  colorActive,
}: {
  initialItems: Item[];
  initialCursor?: string;
  colorActive?: boolean;
}) {
  // `router.refresh()` (fired by QuickAdd/ImageDrop after a save) re-renders
  // the server tree but preserves this client component's state, so a
  // useState initialiser alone would never see the fresh page 1 — a save
  // would clear the input yet never show up in the grid. Re-seeding whenever
  // the server hands down a new `initialItems` array (identity, not length —
  // a same-length add+delete must still be caught) restores the pre-branch
  // behaviour of rendering straight from server props.
  const [seed, setSeed] = useState(initialItems);
  const [state, setState] = useState(() => initialPagedState(initialItems, initialCursor));
  // Bumped only on the same path as the re-seed above (never on every
  // render). A "Load more" in flight when a re-seed happens carries a cursor
  // and windowing from the pre-refresh list; without this, its functional
  // setState update would apply against the freshly re-seeded `prev`,
  // splicing a stale page onto the new list and clobbering the new cursor
  // with an old one. loadMore snapshots this before its await and skips its
  // writes if a re-seed moved it on — same shape as FeedRiver's requestIdRef.
  const requestIdRef = useRef(0);
  const [loading, setLoading] = useState(false);
  const [failed, setFailed] = useState(false);
  const [announcement, setAnnouncement] = useState("");
  if (seed !== initialItems) {
    setSeed(initialItems);
    setState(initialPagedState(initialItems, initialCursor));
    requestIdRef.current += 1;
    // A retry banner or in-flight spinner from before the save is no longer
    // relevant to the freshly re-seeded page 1 (mirrors FeedRiver clearing
    // loadingMore/moreFailed on a filter change).
    setLoading(false);
    setFailed(false);
    setAnnouncement("");
  }

  const loadMore = useCallback(async () => {
    if (loading || !state.cursor) return;
    // Snapshotted so a response can be told apart from a re-seed that
    // happened while it was in flight: if a save triggers a refresh before
    // this fetch resolves, requestIdRef will have moved on and every write
    // below is skipped instead of corrupting the freshly re-seeded state.
    const requestId = requestIdRef.current;
    setLoading(true);
    setFailed(false);
    try {
      const res = await fetch(`/api/items?cursor=${encodeURIComponent(state.cursor)}`);
      if (!res.ok) throw new Error(`failed to load more items: ${res.status}`);
      const page = (await res.json()) as ItemPage;
      if (requestId !== requestIdRef.current) return;
      setState((prev) => appendPage(prev, page));
      setAnnouncement(`${page.items.length} more saves loaded`);
    } catch (err) {
      if (requestId !== requestIdRef.current) return;
      console.error("failed to load more items", err);
      setFailed(true);
    } finally {
      if (requestId === requestIdRef.current) setLoading(false);
    }
  }, [loading, state.cursor]);

  return (
    <>
      {state.pages.map((page, i) => (
        // Index keys are safe here: pages are appended, or replaced wholesale by
        // a re-seed — never reordered or spliced. Cards themselves are keyed by
        // id inside Grid, so a re-seed reconciles on identity, not position.
        <Grid key={i} items={page} colorActive={colorActive} />
      ))}
      {state.cursor ? (
        <LoadMore
          onLoad={loadMore}
          loading={loading}
          error={failed}
          label="Load more saves"
          announcement={announcement}
        />
      ) : null}
    </>
  );
}

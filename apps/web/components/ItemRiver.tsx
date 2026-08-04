"use client";

import { useCallback, useState } from "react";
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
  const [state, setState] = useState(() => initialPagedState(initialItems, initialCursor));
  const [loading, setLoading] = useState(false);
  const [failed, setFailed] = useState(false);
  const [announcement, setAnnouncement] = useState("");

  const loadMore = useCallback(async () => {
    if (loading || !state.cursor) return;
    setLoading(true);
    setFailed(false);
    try {
      const res = await fetch(`/api/items?cursor=${encodeURIComponent(state.cursor)}`);
      if (!res.ok) throw new Error(`failed to load more items: ${res.status}`);
      const page = (await res.json()) as ItemPage;
      setState((prev) => appendPage(prev, page));
      setAnnouncement(`${page.items.length} more saves loaded`);
    } catch (err) {
      console.error("failed to load more items", err);
      setFailed(true);
    } finally {
      setLoading(false);
    }
  }, [loading, state.cursor]);

  return (
    <>
      {state.pages.map((page, i) => (
        // Index keys are safe here: pages are only ever appended, never
        // reordered or spliced.
        <Grid key={i} items={page} colorActive={colorActive} />
      ))}
      <p
        aria-live="polite"
        style={{ position: "absolute", width: 1, height: 1, overflow: "hidden", clip: "rect(0 0 0 0)" }}
      >
        {announcement}
      </p>
      {state.cursor ? (
        <LoadMore onLoad={loadMore} loading={loading} error={failed} label="Load more saves" />
      ) : null}
    </>
  );
}

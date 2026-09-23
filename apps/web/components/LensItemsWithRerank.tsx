"use client";

import { useEffect, useState, useTransition } from "react";
import { tokens } from "@openmind/ui";
import { Grid } from "./Grid";
import type { Item, SearchResponse } from "../lib/types";

const { color, font } = tokens;

const STORAGE_KEY = "om_lens_rerank_beta";

type Props = {
  lensId: string;
  /** Lens rule free-text; re-rank needs a query. Toggle is hidden when empty. */
  query: string;
  initialItems: Item[];
};

/**
 * Lens live-view grid with an opt-in "sort by relevance (beta)" toggle.
 * When on, refetches `/api/lenses/{id}/items?rerank=true` (explicit load only —
 * not search-as-you-type). Preference sticks in localStorage.
 */
export function LensItemsWithRerank({ lensId, query, initialItems }: Props) {
  const canRerank = query.trim().length > 0;
  const [on, setOn] = useState(false);
  const [hydrated, setHydrated] = useState(false);
  const [items, setItems] = useState(initialItems);
  const [err, setErr] = useState<string | null>(null);
  const [pending, startTransition] = useTransition();

  useEffect(() => {
    if (!canRerank) {
      setHydrated(true);
      return;
    }
    try {
      setOn(localStorage.getItem(STORAGE_KEY) === "1");
    } catch {
      /* private mode */
    }
    setHydrated(true);
  }, [canRerank]);

  useEffect(() => {
    if (!hydrated || !canRerank) return;
    if (!on) {
      setItems(initialItems);
      return;
    }
    let cancelled = false;
    startTransition(async () => {
      setErr(null);
      try {
        const res = await fetch(`/api/lenses/${lensId}/items?rerank=true`, { cache: "no-store" });
        if (!res.ok) {
          if (!cancelled) setErr("could not refresh");
          return;
        }
        const body = (await res.json()) as SearchResponse;
        if (!cancelled) setItems((body.results ?? []).map((r) => r.item));
      } catch {
        if (!cancelled) setErr("could not refresh");
      }
    });
    return () => {
      cancelled = true;
    };
  }, [hydrated, canRerank, on, lensId, initialItems]);

  function toggle() {
    const next = !on;
    setOn(next);
    try {
      localStorage.setItem(STORAGE_KEY, next ? "1" : "0");
    } catch {
      /* ignore */
    }
  }

  return (
    <>
      {canRerank ? (
        <div
          style={{
            display: "flex",
            alignItems: "center",
            gap: 10,
            marginBottom: 14,
            flexWrap: "wrap",
          }}
        >
          <label
            style={{
              display: "inline-flex",
              alignItems: "center",
              gap: 8,
              cursor: "pointer",
              fontFamily: font.mono,
              fontSize: 11,
              letterSpacing: ".04em",
              color: color.inkMuted,
              userSelect: "none",
            }}
          >
            <input
              type="checkbox"
              checked={on}
              onChange={toggle}
              disabled={pending}
              style={{ accentColor: color.cobalt }}
            />
            sort by relevance (beta)
          </label>
          {pending ? (
            <span className="meta" style={{ color: color.inkFaintAlt, textTransform: "none" }}>
              sorting…
            </span>
          ) : null}
          {err ? (
            <span className="meta" style={{ color: color.terracotta, textTransform: "none" }}>
              {err}
            </span>
          ) : null}
        </div>
      ) : null}
      <Grid items={items} />
    </>
  );
}

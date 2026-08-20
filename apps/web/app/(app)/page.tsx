import { tokens } from "@openmind/ui";
import { apiFetch } from "../../lib/api";
import type { Item, ItemPage, SearchResponse, UnderstoodQuery } from "../../lib/types";
import { FeedDivider } from "../../components/FeedDivider";
import { Grid } from "../../components/Grid";
import { ItemRiver } from "../../components/ItemRiver";
import { LiveResults } from "../../components/LiveResults";
import { LiveSearchProvider } from "../../components/LiveSearch";
import { QuickAdd } from "../../components/QuickAdd";
import { ImageDrop } from "../../components/ImageDrop";
import { Topbar } from "../../components/Topbar";
import { FilterStrip } from "../../components/FilterStrip";
import { SearchContext } from "../../components/SearchContext";
import { isFeedOnly } from "../../lib/cards";

async function getRecents(): Promise<ItemPage> {
  try {
    const res = await apiFetch("/items");
    if (!res.ok) return { items: [] };
    return ((await res.json()) as ItemPage) ?? { items: [] };
  } catch {
    // API/enrichment may be down; render an empty state rather than failing.
    return { items: [] };
  }
}

interface SearchOutcome {
  items: Item[];
  understood?: UnderstoodQuery;
}

// Runs a search. When q is present we set parse=true so the AI provider can
// split it into text + colour + type + domain filters (it degrades to a plain
// text search under the noop provider). Explicit type/colour/domain filters
// fuse in too and are applied server-side.
async function getSearch(opts: {
  q?: string;
  color?: string;
  type?: string;
  domains?: string;
}): Promise<SearchOutcome> {
  const params = new URLSearchParams();
  if (opts.q) {
    params.set("q", opts.q);
    params.set("parse", "true");
  }
  if (opts.color) params.set("color", opts.color);
  if (opts.type && opts.type !== "all") params.set("types", opts.type);
  if (opts.domains) {
    for (const d of opts.domains.split(",").map((s) => s.trim()).filter(Boolean)) {
      params.append("domains", d);
    }
  }
  try {
    const res = await apiFetch(`/search?${params.toString()}`);
    if (!res.ok) return { items: [] };
    const body = (await res.json()) as SearchResponse;
    // score is intentionally unused for now: results are already rank-ordered by the API.
    return { items: (body.results ?? []).map((r) => r.item), understood: body.understood };
  } catch {
    return { items: [] };
  }
}

export default async function Page({
  searchParams,
}: {
  searchParams: Promise<{ q?: string; type?: string; color?: string; domains?: string }>;
}) {
  const { q, type, color, domains } = await searchParams;
  const active = type ?? "all";
  const searching = Boolean(q || color || (type && type !== "all") || domains);

  // getRecents is skipped while searching: search fuses across the whole
  // library and feed river and the API caps fused results at 50, so it is
  // never paginated — recency paging only applies to the plain /items list.
  const recents = searching ? null : await getRecents();
  const { items, understood } = searching
    ? await getSearch({ q, color, type, domains })
    : { items: recents!.items, understood: undefined };
  // Search spans the feed river, but the library always leads: results from
  // the API arrive library-first, and unkept feed matches render below a
  // divider. Recents (/items) never include unkept feed items, so the divider
  // only ever appears while searching.
  const libraryItems = items.filter((i) => !isFeedOnly(i));
  const feedItems = items.filter(isFeedOnly);

  return (
    <LiveSearchProvider committedQ={q} seed={libraryItems}>
      <Topbar count={items.length} hasMore={Boolean(recents?.nextCursor)} />
      <FilterStrip active={active} q={q} color={color} domains={domains} />
      <SearchContext
        q={q}
        understood={understood}
        colorParam={color}
        typeParam={type}
        domainsParam={domains}
      />

      <div style={{ position: "relative", flex: 1 }}>
        <div
          className="paper-texture"
          style={{ position: "absolute", inset: 0, pointerEvents: "none" }}
        />
        <div style={{ position: "relative", padding: "22px 28px 40px" }}>
          <div
            id="capture"
            style={{
              display: "flex",
              gap: 16,
              alignItems: "stretch",
              flexWrap: "wrap",
              marginBottom: 22,
              scrollMarginTop: 20,
            }}
          >
            <div style={{ flex: "2 1 320px", minWidth: 260 }}>
              <QuickAdd />
            </div>
            <div style={{ flex: "1 1 220px", minWidth: 220 }}>
              <ImageDrop />
            </div>
          </div>
          {/* The server tree is handed to LiveResults as the fallback: it owns
              the results area only while the reader is typing something the
              server has not answered yet, and hands it straight back
              otherwise. Both trees pass `morph`, so a card that survives a
              live filter glides out of the river into its new column rather
              than being replaced by a copy of itself. */}
          <LiveResults
            fallback={
              <>
                {searching ? (
                  libraryItems.length > 0 || feedItems.length === 0 ? (
                    <Grid items={libraryItems} colorActive={Boolean(color)} morph />
                  ) : (
                    <p
                      style={{
                        fontFamily: tokens.font.quote,
                        fontStyle: "italic",
                        fontSize: "1.25rem",
                        color: tokens.color.inkMuted,
                        marginTop: "2rem",
                      }}
                    >
                      Nothing in your Mind matches — these came through your feeds.
                    </p>
                  )
                ) : (
                  <ItemRiver
                    initialItems={libraryItems}
                    initialCursor={recents?.nextCursor}
                    colorActive={Boolean(color)}
                    morph
                  />
                )}
                {feedItems.length > 0 && (
                  <>
                    <FeedDivider count={feedItems.length} />
                    <Grid items={feedItems} colorActive={Boolean(color)} morph />
                  </>
                )}
              </>
            }
          />
        </div>
      </div>
    </LiveSearchProvider>
  );
}

import { tokens } from "@openmind/ui";
import { apiFetch } from "../lib/api";
import type { Item, ItemPage, SearchResponse, UnderstoodQuery } from "../lib/types";
import { Grid } from "../components/Grid";
import { ItemRiver } from "../components/ItemRiver";
import { QuickAdd } from "../components/QuickAdd";
import { ImageDrop } from "../components/ImageDrop";
import { Shell } from "../components/Shell";
import { Topbar } from "../components/Topbar";
import { FilterStrip } from "../components/FilterStrip";
import { SearchContext } from "../components/SearchContext";

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

// An unkept feed-river item: searchable, but not part of the Mind until kept,
// so search shows it below the divider rather than among library matches.
function isFeedOnly(item: Item): boolean {
  return Boolean(item.feedId) && !item.keptAt;
}

function FeedDivider({ count }: { count: number }) {
  return (
    <div
      role="separator"
      aria-label="Matches from your feeds"
      style={{ display: "flex", alignItems: "center", gap: 12, margin: "30px 0 20px" }}
    >
      <span aria-hidden style={{ flex: 1, height: 1, background: tokens.color.hairline }} />
      <span className="meta">From your feeds · {count} — not yet in your Mind</span>
      <span aria-hidden style={{ flex: 1, height: 1, background: tokens.color.hairline }} />
    </div>
  );
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
    <Shell>
      <Topbar count={items.length} q={q} hasMore={Boolean(recents?.nextCursor)} />
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
          {searching ? (
            libraryItems.length > 0 || feedItems.length === 0 ? (
              <Grid items={libraryItems} colorActive={Boolean(color)} />
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
            />
          )}
          {feedItems.length > 0 && (
            <>
              <FeedDivider count={feedItems.length} />
              <Grid items={feedItems} colorActive={Boolean(color)} />
            </>
          )}
        </div>
      </div>
    </Shell>
  );
}

import { apiFetch } from "../lib/api";
import { cardKind } from "../lib/cards";
import type { Item, SearchResponse, UnderstoodQuery } from "../lib/types";
import { Grid } from "../components/Grid";
import { QuickAdd } from "../components/QuickAdd";
import { ImageDrop } from "../components/ImageDrop";
import { Shell } from "../components/Shell";
import { Topbar } from "../components/Topbar";
import { FilterStrip } from "../components/FilterStrip";
import { SearchContext } from "../components/SearchContext";

async function getRecents(): Promise<Item[]> {
  try {
    const res = await apiFetch("/items");
    if (!res.ok) return [];
    return ((await res.json()) as Item[]) ?? [];
  } catch {
    // API/enrichment may be down; render an empty state rather than failing.
    return [];
  }
}

interface SearchOutcome {
  items: Item[];
  understood?: UnderstoodQuery;
}

// Runs a search. When q is present we set parse=true so the AI provider can
// split it into text + colour + type filters (it degrades to a plain text
// search under the noop provider). An explicit colour filter fuses in too.
async function getSearch(q?: string, color?: string): Promise<SearchOutcome> {
  const params = new URLSearchParams();
  if (q) {
    params.set("q", q);
    params.set("parse", "true");
  }
  if (color) params.set("color", color);
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
  searchParams: Promise<{ q?: string; type?: string; color?: string }>;
}) {
  const { q, type, color } = await searchParams;
  const active = type ?? "all";
  const searching = Boolean(q || color);

  const { items: fetched, understood } = searching
    ? await getSearch(q, color)
    : { items: await getRecents(), understood: undefined };
  const items =
    active === "all" ? fetched : fetched.filter((i) => cardKind(i.cardType) === active);

  return (
    <Shell>
      <Topbar count={items.length} q={q} />
      <FilterStrip active={active} q={q} color={color} />
      <SearchContext q={q} understood={understood} colorParam={color} />

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
          <Grid items={items} />
        </div>
      </div>
    </Shell>
  );
}

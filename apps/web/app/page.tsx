import { apiFetch } from "../lib/api";
import { cardKind } from "../lib/cards";
import type { Item, SearchResult } from "../lib/types";
import { Grid } from "../components/Grid";
import { QuickAdd } from "../components/QuickAdd";
import { ImageDrop } from "../components/ImageDrop";
import { Shell } from "../components/Shell";
import { Topbar } from "../components/Topbar";
import { FilterStrip } from "../components/FilterStrip";

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

async function getSearch(q: string): Promise<Item[]> {
  try {
    const res = await apiFetch(`/search?q=${encodeURIComponent(q)}`);
    if (!res.ok) return [];
    const results = ((await res.json()) as SearchResult[]) ?? [];
    // score is intentionally unused for now: results are already rank-ordered by the API.
    return results.map((r) => r.item);
  } catch {
    return [];
  }
}

export default async function Page({
  searchParams,
}: {
  searchParams: Promise<{ q?: string; type?: string }>;
}) {
  const { q, type } = await searchParams;
  const active = type ?? "all";

  const fetched = q ? await getSearch(q) : await getRecents();
  const items =
    active === "all" ? fetched : fetched.filter((i) => cardKind(i.cardType) === active);

  return (
    <Shell>
      <Topbar count={items.length} q={q} />
      <FilterStrip active={active} q={q} />

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

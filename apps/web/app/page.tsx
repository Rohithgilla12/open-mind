import { tokens } from "@openmind/ui";
import { apiFetch } from "../lib/api";
import type { Item, SearchResult } from "../lib/types";
import { Grid } from "../components/Grid";
import { QuickAdd } from "../components/QuickAdd";
import { SearchBox } from "../components/SearchBox";

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
    return results.map((r) => r.item);
  } catch {
    return [];
  }
}

export default async function Page({
  searchParams,
}: {
  searchParams: Promise<{ q?: string }>;
}) {
  const { q } = await searchParams;
  const items = q ? await getSearch(q) : await getRecents();

  return (
    <main style={{ maxWidth: 1200, margin: "0 auto", padding: "2rem 1.5rem" }}>
      <h1
        style={{
          fontFamily: tokens.font.sans,
          fontSize: "1.4rem",
          fontWeight: 600,
          color: tokens.color.ink,
          margin: "0 0 1.25rem",
        }}
      >
        Openmind
      </h1>
      <QuickAdd />
      <SearchBox initial={q} />
      <Grid items={items} />
    </main>
  );
}

import { describe, expect, it } from "vitest";
import { indexItems, normalise, queryLocal, queryTerms } from "./search-index";
import type { Item } from "./types";

/** A minimal item; every field the index reads is overridable. */
function item(over: Partial<Item> & { id: string }): Item {
  return {
    url: "https://example.com/a",
    status: "enriched",
    createdAt: "2026-01-01T00:00:00Z",
    ...over,
  } as Item;
}

describe("normalise", () => {
  const cases: [string, string, string][] = [
    ["lowercases", "Kyoto", "kyoto"],
    ["strips diacritics", "Kyōto Café", "kyoto cafe"],
    ["spaces out punctuation", "Kyōto—in Autumn!", "kyoto in autumn"],
    ["collapses runs", "a   b\n\tc", "a b c"],
    ["keeps digits", "Blade Runner 2049", "blade runner 2049"],
  ];
  for (const [name, input, want] of cases) {
    it(name, () => expect(normalise(input)).toBe(want));
  }

  it("is empty for undefined", () => expect(normalise(undefined)).toBe(""));
});

describe("queryTerms", () => {
  it("splits on whitespace and punctuation", () =>
    expect(queryTerms("  Kyoto, autumn ")).toEqual(["kyoto", "autumn"]));

  it("is empty for a blank query", () => expect(queryTerms("   ")).toEqual([]));
});

describe("queryLocal", () => {
  const kyoto = item({
    id: "aaaa",
    title: "Kyōto in Autumn",
    summary: "Temples and maples.",
    tags: ["travel", "japan"],
    url: "https://simonwillison.net/kyoto",
  });
  const autumnRecipe = item({
    id: "bbbb",
    title: "Roast squash",
    summary: "An autumn recipe worth keeping.",
    tags: ["cooking"],
  });
  const unrelated = item({ id: "cccc", title: "Postgres indexes", tags: ["database"] });
  const index = indexItems([kyoto, autumnRecipe, unrelated]);

  it("returns nothing for a blank query", () => {
    expect(queryLocal(index, "  ")).toEqual([]);
  });

  it("matches a prefix of a title word", () => {
    expect(queryLocal(index, "aut").map((i) => i.id)).toEqual(["aaaa", "bbbb"]);
  });

  it("matches across diacritics", () => {
    expect(queryLocal(index, "kyoto").map((i) => i.id)).toEqual(["aaaa"]);
  });

  it("ANDs terms — every term must match somewhere", () => {
    expect(queryLocal(index, "kyoto autumn").map((i) => i.id)).toEqual(["aaaa"]);
    expect(queryLocal(index, "kyoto postgres")).toEqual([]);
  });

  it("ranks a title word above a summary mention", () => {
    // "Kyōto in Autumn" (title) outranks "An autumn recipe" (summary).
    expect(queryLocal(index, "autumn").map((i) => i.id)).toEqual(["aaaa", "bbbb"]);
  });

  it("ranks a word prefix above a mid-word substring", () => {
    const midword = item({ id: "dddd", title: "Kyoto" });
    const wordStart = item({ id: "eeee", title: "Oto the dog" });
    const ranked = queryLocal(indexItems([midword, wordStart]), "oto");
    expect(ranked.map((i) => i.id)).toEqual(["eeee", "dddd"]);
  });

  it("matches a tag exactly", () => {
    expect(queryLocal(index, "japan").map((i) => i.id)).toEqual(["aaaa"]);
  });

  it("matches a domain", () => {
    expect(queryLocal(index, "simonwillison").map((i) => i.id)).toEqual(["aaaa"]);
  });

  it("honours the limit", () => {
    const many = Array.from({ length: 10 }, (_, i) =>
      item({ id: `id-${i}`, title: `Note ${i}` }),
    );
    expect(queryLocal(indexItems(many), "note", 3)).toHaveLength(3);
  });

  it("breaks ties by newest first, then id descending", () => {
    const older = item({ id: "a1", title: "Note", createdAt: "2026-01-01T00:00:00Z" });
    const newer = item({ id: "a2", title: "Note", createdAt: "2026-06-01T00:00:00Z" });
    const sameDay = item({ id: "a3", title: "Note", createdAt: "2026-06-01T00:00:00Z" });
    const ranked = queryLocal(indexItems([older, newer, sameDay]), "note");
    expect(ranked.map((i) => i.id)).toEqual(["a3", "a2", "a1"]);
  });
});

describe("queryLocal colour terms", () => {
  const cobalt = item({ id: "col", title: "A print", palette: ["#1B3FD1", "#F4F0E6"] });
  const nearCobalt = item({ id: "near", title: "Another print", palette: ["#1B54D1"] });
  const terracotta = item({ id: "terra", title: "A pot", palette: ["#C24A2E"] });
  const noPalette = item({ id: "bare", title: "A thought" });
  const index = indexItems([cobalt, nearCobalt, terracotta, noPalette]);

  it("matches a named colour against the extracted palette", () => {
    const ranked = queryLocal(index, "cobalt").map((i) => i.id);
    // Exact palette hit first, the neighbouring blue second, the pot excluded.
    expect(ranked).toEqual(["col", "near"]);
  });

  it("matches a hex term", () => {
    expect(queryLocal(index, "#C24A2E").map((i) => i.id)).toEqual(["terra"]);
  });

  it("never matches an item with no extracted palette", () => {
    // "A thought" has no palette; cards show derivedPalette() dots for it, but
    // those are decoration and must not answer a colour search.
    expect(queryLocal(index, "cobalt").map((i) => i.id)).not.toContain("bare");
  });

  it("does not read a hex-shaped word as a colour", () => {
    // "facade" is spelled entirely in hex digits. Before colourTerm it was
    // treated as #facade and matched items by palette, so a plain text search
    // silently became a palette search.
    const pale = item({ id: "pal", title: "A thing", palette: ["#FACADE"] });
    const worded = item({ id: "txt", title: "The facade of the building" });
    const ranked = queryLocal(indexItems([pale, worded]), "facade").map((i) => i.id);
    expect(ranked).toEqual(["txt"]);
  });

  it("still matches a colour word found in text", () => {
    const named = item({ id: "word", title: "The Cobalt Notebook" });
    expect(queryLocal(indexItems([named]), "cobalt").map((i) => i.id)).toEqual(["word"]);
  });

  it("ANDs a colour term with a word term", () => {
    expect(queryLocal(index, "cobalt print").map((i) => i.id)).toEqual(["col", "near"]);
    expect(queryLocal(index, "cobalt pot")).toEqual([]);
  });
});

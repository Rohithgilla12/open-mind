import { filterCachedItems, isInfiniteCache, mapCachedItems, trimToFirstPage } from "../paged-cache";

type Row = { id: string; pinnedAt?: string | null };

const pin = (r: Row): Row => (r.id === "b" ? { ...r, pinnedAt: "2026-08-04T00:00:00Z" } : r);

describe("isInfiniteCache", () => {
  it("recognises a TanStack infinite cache", () => {
    expect(isInfiniteCache({ pages: [], pageParams: [] })).toBe(true);
  });

  it("rejects the flat shapes", () => {
    expect(isInfiniteCache([{ id: "a" }])).toBe(false);
    expect(isInfiniteCache({ items: [{ id: "a" }] })).toBe(false);
    expect(isInfiniteCache(undefined)).toBe(false);
  });
});

describe("mapCachedItems", () => {
  it("patches an item that lives on a later page", () => {
    const cache: { pages: { items: Row[]; nextCursor?: string }[]; pageParams: unknown[] } = {
      pages: [{ items: [{ id: "a" }] }, { items: [{ id: "z" }, { id: "b" }] }],
      pageParams: [undefined, "cur1"],
    };
    const got = mapCachedItems<Row>(cache, pin) as typeof cache;
    expect(got.pages[1].items[1].pinnedAt).toBe("2026-08-04T00:00:00Z");
    expect(got.pages[0].items[0].pinnedAt).toBeUndefined();
    expect(got.pages[1].nextCursor).toBeUndefined();
    expect(got.pageParams).toEqual([undefined, "cur1"]);
  });

  it("does not mutate the cache it patches", () => {
    const cache: { pages: { items: Row[] }[]; pageParams: unknown[] } = {
      pages: [{ items: [{ id: "b" }] }],
      pageParams: [undefined],
    };
    mapCachedItems<Row>(cache, pin);
    expect(cache.pages[0].items[0].pinnedAt).toBeUndefined();
  });

  it("still patches a flat array cache (desk)", () => {
    const got = mapCachedItems<Row>([{ id: "a" }, { id: "b" }], pin) as Row[];
    expect(got[1].pinnedAt).toBe("2026-08-04T00:00:00Z");
  });

  it("still patches an { items } cache (search)", () => {
    const got = mapCachedItems<Row>({ items: [{ id: "b" }], understood: { text: "x" } }, pin) as {
      items: Row[];
      understood: { text: string };
    };
    expect(got.items[0].pinnedAt).toBe("2026-08-04T00:00:00Z");
    expect(got.understood).toEqual({ text: "x" });
  });

  it("leaves an unset cache alone", () => {
    expect(mapCachedItems<Row>(undefined, pin)).toBeUndefined();
  });
});

describe("filterCachedItems", () => {
  const keep = (r: Row) => r.id !== "b";

  it("removes an item that lives on a later page", () => {
    const cache: { pages: { items: Row[]; nextCursor?: string }[]; pageParams: unknown[] } = {
      pages: [{ items: [{ id: "a" }] }, { items: [{ id: "z" }, { id: "b" }] }],
      pageParams: [undefined, "cur1"],
    };
    const got = filterCachedItems<Row>(cache, keep) as typeof cache;
    expect(got.pages[1].items).toEqual([{ id: "z" }]);
    expect(got.pages[0].items).toEqual([{ id: "a" }]);
    expect(got.pageParams).toEqual([undefined, "cur1"]);
  });

  it("does not mutate the cache it filters", () => {
    const cache: { pages: { items: Row[] }[]; pageParams: unknown[] } = {
      pages: [{ items: [{ id: "a" }, { id: "b" }] }],
      pageParams: [undefined],
    };
    filterCachedItems<Row>(cache, keep);
    expect(cache.pages[0].items).toEqual([{ id: "a" }, { id: "b" }]);
  });

  it("still filters a flat array cache (desk)", () => {
    const got = filterCachedItems<Row>([{ id: "a" }, { id: "b" }], keep) as Row[];
    expect(got).toEqual([{ id: "a" }]);
  });

  it("still filters an { items } cache (search)", () => {
    const got = filterCachedItems<Row>(
      { items: [{ id: "a" }, { id: "b" }], understood: { text: "x" } },
      keep,
    ) as { items: Row[]; understood: { text: string } };
    expect(got.items).toEqual([{ id: "a" }]);
    expect(got.understood).toEqual({ text: "x" });
  });

  it("leaves an unset cache alone", () => {
    expect(filterCachedItems<Row>(undefined, keep)).toBeUndefined();
  });
});

describe("trimToFirstPage", () => {
  it("drops every page after the first so one refetch is one request", () => {
    const cache = {
      pages: [{ items: [{ id: "a" }], nextCursor: "cur1" }, { items: [{ id: "b" }] }],
      pageParams: [undefined, "cur1"],
    };
    const got = trimToFirstPage(cache) as typeof cache;
    expect(got.pages).toHaveLength(1);
    expect(got.pageParams).toEqual([undefined]);
  });

  it("leaves non-infinite caches untouched", () => {
    const flat = [{ id: "a" }];
    expect(trimToFirstPage(flat)).toBe(flat);
    expect(trimToFirstPage(undefined)).toBeUndefined();
  });
});

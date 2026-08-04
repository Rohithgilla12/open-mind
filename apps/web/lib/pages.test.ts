import { describe, expect, it } from "vitest";
import { appendPage, initialPagedState, mapPagedItems, type PagedState } from "./pages";

type Row = { id: string; kept?: boolean };

describe("paged state", () => {
  it("starts as a single page carrying the first cursor", () => {
    const s = initialPagedState<Row>([{ id: "a" }, { id: "b" }], "cur1");
    expect(s.pages).toEqual([[{ id: "a" }, { id: "b" }]]);
    expect(s.cursor).toBe("cur1");
  });

  it("keeps each page as its own array so the Mind can render one block per page", () => {
    let s = initialPagedState<Row>([{ id: "a" }], "cur1");
    s = appendPage(s, { items: [{ id: "b" }], nextCursor: "cur2" });
    expect(s.pages).toHaveLength(2);
    expect(s.pages[0]).toEqual([{ id: "a" }]);
    expect(s.pages[1]).toEqual([{ id: "b" }]);
    expect(s.cursor).toBe("cur2");
  });

  it("clears the cursor when a page arrives without one", () => {
    let s = initialPagedState<Row>([{ id: "a" }], "cur1");
    s = appendPage(s, { items: [{ id: "b" }] });
    expect(s.cursor).toBeUndefined();
  });

  it("drops an empty final page rather than rendering an empty block", () => {
    const s = appendPage(initialPagedState<Row>([{ id: "a" }], "cur1"), { items: [] });
    expect(s.pages).toHaveLength(1);
    expect(s.cursor).toBeUndefined();
  });

  it("maps items across every page", () => {
    let s = initialPagedState<Row>([{ id: "a" }], "cur1");
    s = appendPage(s, { items: [{ id: "b" }] });
    const mapped = mapPagedItems(s, (r) => (r.id === "b" ? { ...r, kept: true } : r));
    expect(mapped.pages[1][0].kept).toBe(true);
    expect(mapped.pages[0][0].kept).toBeUndefined();
  });

  it("does not mutate the state it maps", () => {
    const s: PagedState<Row> = initialPagedState<Row>([{ id: "a" }], undefined);
    mapPagedItems(s, (r) => ({ ...r, kept: true }));
    expect(s.pages[0][0].kept).toBeUndefined();
  });
});

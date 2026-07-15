import { describe, expect, it } from "vitest";
import type { Item } from "./api";
import { mergeHomeLists } from "./home-lists";

function item(id: string, title = id): Item {
  return { id, url: `https://example.com/${id}`, title, status: "enriched" };
}

describe("mergeHomeLists", () => {
  it("caps desk at 5 and recent at 8 by default", () => {
    const desk = Array.from({ length: 7 }, (_, i) => item(`d${i}`));
    const recent = Array.from({ length: 12 }, (_, i) => item(`r${i}`));
    const merged = mergeHomeLists(desk, recent);
    expect(merged.desk).toHaveLength(5);
    expect(merged.recent).toHaveLength(8);
    expect(merged.desk.map((i) => i.id)).toEqual(["d0", "d1", "d2", "d3", "d4"]);
  });

  it("excludes desk ids from recent", () => {
    const desk = [item("a"), item("b")];
    const recent = [item("a"), item("c"), item("b"), item("d")];
    const merged = mergeHomeLists(desk, recent);
    expect(merged.desk.map((i) => i.id)).toEqual(["a", "b"]);
    expect(merged.recent.map((i) => i.id)).toEqual(["c", "d"]);
  });

  it("honours custom caps", () => {
    const desk = [item("a"), item("b"), item("c")];
    const recent = [item("d"), item("e"), item("f")];
    const merged = mergeHomeLists(desk, recent, { deskCap: 2, recentCap: 1 });
    expect(merged.desk.map((i) => i.id)).toEqual(["a", "b"]);
    expect(merged.recent.map((i) => i.id)).toEqual(["d"]);
  });
});

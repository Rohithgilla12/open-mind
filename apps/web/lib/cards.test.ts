import { describe, expect, it } from "vitest";
import {
  cardKind,
  chipLabel,
  fallbackTitle,
  isFeedOnly,
  isTextForward,
  typeAccent,
  typeFilters,
  typeGradient,
  typeLabel,
} from "./cards";
import type { Item } from "./types";

describe("typeFilters", () => {
  const kinds = Object.keys(typeLabel);

  it("leads with All, which carries no type filter", () => {
    expect(typeFilters[0]).toEqual({ label: "All", type: "all" });
  });

  // The guard the chip list previously lacked: `repo` had to be added by hand,
  // and `Record<CardKind, …>` alone can't catch a kind missing from the order.
  it("offers a chip for every card kind", () => {
    const chipped = typeFilters.filter((f) => f.type !== "all").map((f) => f.type);
    expect([...chipped].sort()).toEqual([...kinds].sort());
  });

  // Pins the curated order and the plural wording as shipped, so deriving the
  // chips from the enum can't quietly reorder or rename the strip.
  it("renders the curated order", () => {
    expect(typeFilters.map((f) => f.label)).toEqual([
      "All",
      "Articles",
      "Images",
      "Quotes",
      "Products",
      "Video",
      "Posts",
      "Recipes",
      "Notes",
      "Books",
      "Repos",
    ]);
  });

  it("lists no kind twice", () => {
    const types = typeFilters.map((f) => f.type);
    expect(new Set(types).size).toBe(types.length);
  });

  it("gives every kind a gradient, accent, and both labels", () => {
    for (const kind of kinds) {
      expect(typeGradient[kind as keyof typeof typeGradient]).toBeTruthy();
      expect(typeAccent[kind as keyof typeof typeAccent]).toBeTruthy();
      expect(typeLabel[kind as keyof typeof typeLabel]).toBeTruthy();
      expect(chipLabel[kind as keyof typeof chipLabel]).toBeTruthy();
    }
  });
});

describe("repo card type", () => {
  it("normalises repo to itself", () => {
    expect(cardKind("repo")).toBe("repo");
  });
  it("labels it Repo", () => {
    expect(typeLabel.repo).toBe("Repo");
  });
  it("has a gradient", () => {
    expect(typeGradient.repo).toContain("linear-gradient");
  });
  it("still falls back to article for unknown types", () => {
    expect(cardKind("gizmo")).toBe("article");
  });
});

describe("fallbackTitle", () => {
  it("de-slugifies the last path segment", () => {
    expect(fallbackTitle("https://www.etsy.com/codeascraft/kafka-app-there-is-a-skill")).toBe(
      "kafka app there is a skill",
    );
  });
  it("ignores the query string that bot-protection redirects tack on", () => {
    expect(fallbackTitle("https://themodesse.com/products/loop-up-top?utm_source=facebook&fbclid=x")).toBe(
      "loop up top",
    );
  });
  it("ignores a trailing slash", () => {
    expect(fallbackTitle("https://example.com/some-post/")).toBe("some post");
  });
  it("drops a file extension", () => {
    expect(fallbackTitle("https://example.com/docs/getting-started.html")).toBe("getting started");
  });
  it("treats underscores as word breaks", () => {
    expect(fallbackTitle("https://example.com/a_b_c")).toBe("a b c");
  });
  it("falls back to the hostname when there is no path", () => {
    expect(fallbackTitle("https://www.etsy.com/")).toBe("etsy.com");
  });
  it("returns null for uploads, which have no meaningful url", () => {
    expect(fallbackTitle("/assets/abc123")).toBeNull();
  });
  it("returns null for an unparseable url", () => {
    expect(fallbackTitle("not a url")).toBeNull();
  });
  it("returns null when there is no url at all", () => {
    expect(fallbackTitle(undefined)).toBeNull();
  });
  it("returns null when a segment de-slugifies to nothing", () => {
    expect(fallbackTitle("https://example.com/---")).toBeNull();
  });
});

describe("isTextForward", () => {
  it("treats repo as text-forward (README-style body reads like an article)", () => {
    expect(isTextForward("repo")).toBe(true);
  });
  it("excludes non-text-forward kinds, e.g. image", () => {
    expect(isTextForward("image")).toBe(false);
  });
});

describe("isFeedOnly", () => {
  const base = { id: "a", url: "https://example.com", status: "enriched", createdAt: "2026-01-01T00:00:00Z" };
  const cases: [string, Partial<Item>, boolean][] = [
    ["a plain save is not feed-only", {}, false],
    ["an unkept feed item is feed-only", { feedId: "f1" }, true],
    ["a kept feed item is not", { feedId: "f1", keptAt: "2026-02-01T00:00:00Z" }, false],
  ];
  for (const [name, over, want] of cases) {
    it(name, () => expect(isFeedOnly({ ...base, ...over } as Item)).toBe(want));
  }
});

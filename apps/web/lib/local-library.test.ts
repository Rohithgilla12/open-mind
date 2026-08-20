import { afterEach, describe, expect, it, vi } from "vitest";
import { createLocalLibrary, type LibraryProgress } from "./local-library";
import type { Item, ItemPage } from "./types";

function item(id: string, title: string): Item {
  return {
    id,
    url: `https://example.com/${id}`,
    title,
    status: "enriched",
    createdAt: "2026-01-01T00:00:00Z",
  } as Item;
}

/** Serve the given pages in order, then fail if asked for more. */
function stubPages(pages: ItemPage[]) {
  let call = 0;
  const fetchMock = vi.fn(async () => {
    const page = pages[call++];
    if (!page) throw new Error("unexpected extra page request");
    return { ok: true, json: async () => page } as unknown as Response;
  });
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

/** Resolve once the crawl reports done. */
function crawled(): { onProgress: (p: LibraryProgress) => void; done: Promise<LibraryProgress> } {
  let resolve: (p: LibraryProgress) => void;
  const done = new Promise<LibraryProgress>((r) => (resolve = r));
  return {
    onProgress: (p) => {
      if (p.done) resolve(p);
    },
    done,
  };
}

afterEach(() => vi.unstubAllGlobals());

describe("createLocalLibrary", () => {
  it("answers from the seed before any crawl", () => {
    const lib = createLocalLibrary(() => {});
    lib.seed([item("a", "Kyoto in Autumn"), item("b", "Roast squash")]);
    expect(lib.query("kyoto").map((i) => i.id)).toEqual(["a"]);
    expect(lib.progress()).toEqual({ indexed: 2, done: false });
  });

  it("pages until there is no cursor, then reports done", async () => {
    stubPages([
      { items: [item("a", "One")], nextCursor: "c1" },
      { items: [item("b", "Two")] },
    ]);
    const { onProgress, done } = crawled();
    const lib = createLocalLibrary(onProgress);
    lib.crawl();
    expect(await done).toEqual({ indexed: 2, done: true });
    expect(lib.query("two").map((i) => i.id)).toEqual(["b"]);
  });

  it("does not index the same item twice", async () => {
    stubPages([{ items: [item("a", "One"), item("a", "One")] }]);
    const { onProgress, done } = crawled();
    const lib = createLocalLibrary(onProgress);
    lib.seed([item("a", "One")]);
    lib.crawl();
    expect((await done).indexed).toBe(1);
  });

  it("crawls once however often it is called", async () => {
    const fetchMock = stubPages([{ items: [item("a", "One")] }]);
    const { onProgress, done } = crawled();
    const lib = createLocalLibrary(onProgress);
    lib.crawl();
    lib.crawl();
    await done;
    lib.crawl();
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("stops when the cursor does not advance", async () => {
    // add() dedupes by id, so a repeated cursor adds nothing and the item cap
    // never trips — without the guard this re-fetched the same page forever.
    const fetchMock = vi.fn(async () => ({
      ok: true,
      json: async () => ({ items: [item("a", "One")], nextCursor: "same" }),
    } as unknown as Response));
    vi.stubGlobal("fetch", fetchMock);
    const { onProgress, done } = crawled();
    const lib = createLocalLibrary(onProgress);
    lib.crawl();
    expect(await done).toEqual({ indexed: 1, done: true });
    await new Promise((r) => setTimeout(r, 200));
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it("stop() halts an in-flight crawl and refuses to start another", async () => {
    const fetchMock = vi.fn(async () => ({
      ok: true,
      json: async () => ({ items: [item(`a${fetchMock.mock.calls.length}`, "One")], nextCursor: `c${fetchMock.mock.calls.length}` }),
    } as unknown as Response));
    vi.stubGlobal("fetch", fetchMock);
    const lib = createLocalLibrary(() => {});
    lib.crawl();
    await new Promise((r) => setTimeout(r, 20));
    lib.stop();
    const callsAtStop = fetchMock.mock.calls.length;
    await new Promise((r) => setTimeout(r, 250));
    expect(fetchMock.mock.calls.length).toBe(callsAtStop);
    lib.crawl();
    await new Promise((r) => setTimeout(r, 120));
    expect(fetchMock.mock.calls.length).toBe(callsAtStop);
  });

  it("keeps answering from what it has when a page fails", async () => {
    let call = 0;
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => {
        call += 1;
        if (call === 1) {
          return {
            ok: true,
            json: async () => ({ items: [item("a", "Kyoto")], nextCursor: "c1" }),
          } as unknown as Response;
        }
        return { ok: false, status: 500 } as unknown as Response;
      }),
    );
    const { onProgress, done } = crawled();
    const lib = createLocalLibrary(onProgress);
    lib.crawl();
    // Reports done so the interface stops promising more of the library.
    expect(await done).toEqual({ indexed: 1, done: true });
    expect(lib.query("kyoto").map((i) => i.id)).toEqual(["a"]);
  });
});

/**
 * The reader's library, held locally and searched without a network round trip.
 *
 * Pages in the library through the existing /api/items proxy, indexes each page
 * with lib/search-index.ts, and answers queries from memory. The Mind's search
 * pill drives it; the server's hybrid ranking replaces its answer when that
 * lands (see components/LiveSearch.tsx).
 *
 * This runs on the main thread, deliberately, after a Web Worker turned out not
 * to survive the bundler: Turbopack resolves
 * `new Worker(new URL("./x.worker.ts", import.meta.url))` to a static asset URL
 * and serves the raw TypeScript as `video/mp2t`, which a module worker refuses
 * to execute — so a worker would have meant no local search at all in
 * production. Main-thread is affordable because the costs are small and
 * bounded: indexing one 200-item page measures in single-digit milliseconds and
 * yields to the browser afterwards, and scoring a query is a linear scan of a
 * few thousand prepared strings — far inside a frame. If a library ever grows
 * enough to make that visible, the seam to move is this file, not the index.
 */
import { indexItems, LOCAL_RESULT_LIMIT, queryLocal, type Indexed } from "./search-index";
import type { Item, ItemPage } from "./types";

/** The API clamps ?limit to 200 (maxListLimit in apps/api/internal/api/server.go). */
const PAGE_SIZE = 200;

/**
 * Ceiling on what is held in memory. At roughly a kilobyte of JSON per item
 * this is a few megabytes — far more library than anyone has today, and a bound
 * worth having before an unbounded crawl finds the account that does.
 */
const MAX_INDEXED = 5000;

/**
 * Breather between pages. Long enough that a crawl never competes with the
 * reader's own requests or chains into a long task, short enough that a large
 * library is indexed within a few seconds of the first keystroke.
 */
const PAGE_PAUSE_MS = 60;

export interface LibraryProgress {
  indexed: number;
  /** True once the whole library is in, or the crawl gave up. */
  done: boolean;
}

export interface LocalLibrary {
  /** Index items already on screen, so the first keystroke has something to match. */
  seed: (items: Item[]) => void;
  /** Page in the rest of the library. Safe to call repeatedly; only the first call crawls. */
  crawl: () => void;
  /** Best local matches for a query, most relevant first. */
  query: (q: string) => Item[];
  progress: () => LibraryProgress;
  /**
   * Abort an in-flight crawl and refuse to start another. The library is held
   * in a ref by its owner, so without this the fetch loop keeps paging after
   * the component that started it has gone.
   */
  stop: () => void;
}

const sleep = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms));

/**
 * `onProgress` fires after every page — both to report the count and to let the
 * caller re-run the active query, so results deepen as the library arrives
 * rather than waiting for the crawl to finish.
 */
export function createLocalLibrary(onProgress: (p: LibraryProgress) => void): LocalLibrary {
  const index: Indexed[] = [];
  const seen = new Set<string>();
  let crawling = false;
  let done = false;
  let stopped = false;
  let inFlight: AbortController | null = null;

  function add(items: Item[]): void {
    const room = MAX_INDEXED - index.length;
    if (room <= 0) return;
    const fresh: Item[] = [];
    for (const item of items) {
      if (!item.id || seen.has(item.id)) continue;
      seen.add(item.id);
      fresh.push(item);
      if (fresh.length === room) break;
    }
    index.push(...indexItems(fresh));
  }

  const progress = (): LibraryProgress => ({ indexed: index.length, done });

  async function run(): Promise<void> {
    let cursor: string | undefined;
    try {
      for (;;) {
        if (stopped) return;
        const params = new URLSearchParams({ limit: String(PAGE_SIZE) });
        if (cursor) params.set("cursor", cursor);
        inFlight = new AbortController();
        const res = await fetch(`/api/items?${params.toString()}`, { signal: inFlight.signal });
        if (!res.ok) throw new Error(`items page failed: ${res.status}`);
        const page = (await res.json()) as ItemPage;
        if (stopped) return;
        add(page.items ?? []);
        const next = page.nextCursor;
        // A cursor that fails to advance has to end the crawl, not just the
        // page: `add` dedupes by id, so a repeated cursor contributes nothing,
        // `index.length` never reaches MAX_INDEXED, and the item cap never
        // stops it — the loop would re-fetch the same page every pause,
        // forever.
        if (!next || next === cursor || index.length >= MAX_INDEXED) {
          done = true;
          onProgress(progress());
          return;
        }
        cursor = next;
        onProgress(progress());
        await sleep(PAGE_PAUSE_MS);
      }
    } catch {
      // Cancellation is not a failure and has nothing to report: the owner is
      // going away.
      if (stopped) return;
      // A failed crawl is not a failed search either: whatever was indexed
      // still answers, and the server search is untouched. Stop and report
      // done so the interface stops promising more of the library.
      done = true;
      onProgress(progress());
    } finally {
      crawling = false;
      inFlight = null;
    }
  }

  return {
    seed(items) {
      add(items);
      onProgress(progress());
    },
    crawl() {
      if (crawling || done || stopped) return;
      crawling = true;
      void run();
    },
    query(q) {
      return queryLocal(index, q, LOCAL_RESULT_LIMIT);
    },
    progress,
    stop() {
      stopped = true;
      inFlight?.abort();
    },
  };
}

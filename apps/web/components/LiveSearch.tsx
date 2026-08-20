"use client";

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { useRouter } from "next/navigation";
import { createLocalLibrary, type LocalLibrary } from "../lib/local-library";
import { serverSearchParams } from "../lib/search-request";
import { morph } from "../lib/view-transition";
import type { Item, SearchResponse } from "../lib/types";

/**
 * Live search for the Mind.
 *
 * Every keystroke is answered from the local index (lib/local-library.ts) with
 * no network at all, and a debounced /search call replaces that answer with the
 * server's hybrid FTS + vector ranking when it lands. The reader sees matches
 * immediately and a quiet re-order a few hundred milliseconds later; both
 * changes are applied inside a view transition, so the second one reads as the
 * same saves rearranging rather than a new list arriving.
 *
 * The URL is only touched on Enter. Typing is not navigation.
 */

/** How long to wait after the last keystroke before asking the server. */
const SERVER_DEBOUNCE_MS = 220;

type Phase = "local" | "server";

interface Answer {
  /** Which query this answers; a stale id is discarded. */
  id: number;
  items: Item[];
  phase: Phase;
}

interface LiveSearchValue {
  query: string;
  setQuery: (q: string) => void;
  /** Promote the live query to a real ?q= URL (Enter). */
  commit: () => void;
  /** Escape: drop the live query, or leave a committed search if there is nothing to drop. */
  clear: () => void;
  /** True while the live overlay owns the results area. */
  active: boolean;
  answer: Answer | null;
  serverPending: boolean;
  indexed: number;
  indexDone: boolean;
  /** Called on search intent (focus, ⌘K) to build the index and start the crawl. */
  warm: () => void;
}

const Ctx = createContext<LiveSearchValue | null>(null);

export function useLiveSearch(): LiveSearchValue {
  const ctx = useContext(Ctx);
  if (!ctx) throw new Error("useLiveSearch must be used inside <LiveSearchProvider>");
  return ctx;
}

export function LiveSearchProvider({
  committedQ,
  seed,
  children,
}: {
  /** The ?q= the server rendered, if any. */
  committedQ?: string;
  /** Items already on screen, indexed immediately so the first keystroke lands. */
  seed: Item[];
  children: ReactNode;
}) {
  const router = useRouter();
  const [query, setQueryState] = useState(committedQ ?? "");
  const [answer, setAnswer] = useState<Answer | null>(null);
  const [serverPending, setServerPending] = useState(false);
  const [indexed, setIndexed] = useState(0);
  const [indexDone, setIndexDone] = useState(false);

  // The committed query is server state; when a navigation changes it (Enter,
  // back/forward, a cleared search) the input follows it and any live answer
  // for the old query is dropped. Derived during render rather than in an
  // effect so the two never disagree for a frame — same shape as ItemRiver's
  // re-seed.
  const [seenCommitted, setSeenCommitted] = useState(committedQ);
  if (seenCommitted !== committedQ) {
    setSeenCommitted(committedQ);
    setQueryState(committedQ ?? "");
    setAnswer(null);
    setServerPending(false);
  }

  const libraryRef = useRef<LocalLibrary | null>(null);
  const queryIdRef = useRef(0);
  const answerRef = useRef<Answer | null>(null);
  const abortRef = useRef<AbortController | null>(null);
  const seedRef = useRef(seed);
  seedRef.current = seed;
  // Read by the crawl's progress callback, which outlives any one render.
  const queryRef = useRef(query);
  queryRef.current = query;
  const activeRef = useRef(false);

  const trimmed = query.trim();
  const committedTrimmed = (committedQ ?? "").trim();
  // When the live query equals the committed one, the server-rendered tree is
  // already the better answer for it — the overlay stands down.
  const active = trimmed.length > 0 && trimmed !== committedTrimmed;

  activeRef.current = active;

  /** Publish an answer, ignoring stale ids and never letting local outrank server. */
  const apply = useCallback((next: Answer) => {
    if (next.id !== queryIdRef.current) return;
    const current = answerRef.current;
    if (next.phase === "local" && current?.id === next.id && current.phase === "server") return;
    // A server answer that found nothing never replaces local matches. The
    // server's text search matches whole lexemes (websearch_to_tsquery in
    // apps/api/internal/store/queries/search.sql), so a query that is still
    // mid-word — which is every query while someone is typing — legitimately
    // returns nothing, and on the noop AI provider there is no vector half to
    // rescue it. Letting that through would make results appear instantly and
    // then vanish a third of a second later, on every keystroke inside a word.
    // The server wins on ranking and reach, never on emptiness.
    if (
      next.phase === "server" &&
      next.items.length === 0 &&
      current?.id === next.id &&
      current.items.length > 0
    ) {
      return;
    }
    answerRef.current = next;
    morph(() => setAnswer(next));
  }, []);

  /**
   * Build the index and start paging the library in. Tied to search intent
   * (focus, ⌘K, a keystroke) rather than to page load, so a reader who never
   * searches never pays for it.
   */
  const warm = useCallback(() => {
    if (libraryRef.current) return;
    const library = createLocalLibrary((p) => {
      setIndexed(p.indexed);
      setIndexDone(p.done);
      // A page just landed: re-answer whatever is being typed, so results
      // deepen as the library arrives.
      //
      // Only ever a RE-answer. The first answer for a query id belongs to the
      // effect below, which publishes it in the same task as the keystroke; if
      // this fired first (it would, on the synchronous seed inside `warm`) the
      // effect's identical answer would land as a plain update while the seed's
      // view transition was still capturing, painting the new grid a frame
      // before the transition animated towards it.
      const q = queryRef.current.trim();
      if (activeRef.current && q && answerRef.current?.id === queryIdRef.current) {
        apply({ id: queryIdRef.current, items: library.query(q), phase: "local" });
      }
    });
    libraryRef.current = library;
    library.seed(seedRef.current);
    library.crawl();
  }, [apply]);

  // The crawl outlives this component otherwise: the library sits in a ref and
  // its fetch loop would keep paging after the reader has navigated away. The
  // ref is cleared as well as stopped because React runs an extra
  // cleanup/setup cycle in development, and a non-null ref holding a stopped
  // library would make `warm` a no-op for the rest of the session.
  useEffect(
    () => () => {
      libraryRef.current?.stop();
      libraryRef.current = null;
    },
    [],
  );

  useEffect(() => {
    if (!active) {
      abortRef.current?.abort();
      abortRef.current = null;
      // Retire the id so an answer already in flight cannot land after the
      // reader has stepped out of the search.
      queryIdRef.current += 1;
      answerRef.current = null;
      setServerPending(false);
      return;
    }

    const id = queryIdRef.current + 1;
    queryIdRef.current = id;
    warm();
    const local = libraryRef.current;
    if (local) apply({ id, items: local.query(trimmed), phase: "local" });

    const timer = setTimeout(async () => {
      abortRef.current?.abort();
      const ac = new AbortController();
      abortRef.current = ac;
      setServerPending(true);
      try {
        const res = await fetch(`/api/search?${serverSearchParams(trimmed).toString()}`, {
          signal: ac.signal,
        });
        if (!res.ok) throw new Error(`search failed: ${res.status}`);
        const body = (await res.json()) as SearchResponse;
        apply({ id, items: (body.results ?? []).map((r) => r.item), phase: "server" });
      } catch {
        // Aborted, offline, or the API is down — the local answer stands, which
        // is the whole point of having one.
      } finally {
        if (queryIdRef.current === id) setServerPending(false);
      }
    }, SERVER_DEBOUNCE_MS);

    return () => clearTimeout(timer);
  }, [active, trimmed, warm, apply]);

  const setQuery = useCallback((q: string) => setQueryState(q), []);

  const commit = useCallback(() => {
    const q = query.trim();
    router.push(q ? `/?q=${encodeURIComponent(q)}` : "/");
  }, [query, router]);

  const clear = useCallback(() => {
    if (trimmed && trimmed !== committedTrimmed) {
      setQueryState(committedQ ?? "");
      setAnswer(null);
      return;
    }
    if (committedTrimmed) router.push("/");
  }, [trimmed, committedTrimmed, committedQ, router]);

  return (
    <Ctx.Provider
      value={{
        query,
        setQuery,
        commit,
        clear,
        active,
        answer,
        serverPending,
        indexed,
        indexDone,
        warm,
      }}
    >
      {children}
    </Ctx.Provider>
  );
}
